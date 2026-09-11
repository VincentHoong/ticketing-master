package simulation

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"ticketing-master/repository"
	reservationRepo "ticketing-master/repository/reservations"
	"ticketing-master/service"
)

type Outcome string

const (
	OutcomeReserved      Outcome = "reserved"
	OutcomeCapacityFull  Outcome = "capacity_full"
	OutcomeQuotaExceeded Outcome = "quota_exceeded"
	OutcomeTimedOut      Outcome = "timed_out"
	OutcomeError         Outcome = "error"
)

const snapshotTimeout = 5 * time.Second

const (
	DefaultWorkers      = 200
	DefaultPollInterval = 50 * time.Millisecond
	DefaultUserTimeout  = 60 * time.Second
	MaxPollInterval     = 1 * time.Second
	// Measured: a single Redis saturates near 50k polls/s on this hardware, and past that
	// the poll storm queues ahead of the reserve path and inflates every latency in the
	// run. Below it, wall clock grows with the interval instead. 20k users at 400ms sits
	// on that knee - 35s and a 3.8ms reserve p50, against 142s and 1257ms at 50ms.
	pollOpsBudget = 50000
)

type Config struct {
	EventId       string
	UserIds       []string
	Quantity      uint32
	MaxConcurrent uint64
	Workers       int
	PollInterval  time.Duration
	UserTimeout   time.Duration
	Transport     Transport
	BaseURL       string
	Tokens        map[string]string
}

type Snapshot struct {
	EventId          string           `json:"eventId"`
	TotalUsers       int              `json:"totalUsers"`
	Started          int64            `json:"started"`
	Enqueued         int64            `json:"enqueued"`
	Admitted         int64            `json:"admitted"`
	Reserved         int64            `json:"reserved"`
	RejectedCapacity int64            `json:"rejectedCapacity"`
	RejectedQuota    int64            `json:"rejectedQuota"`
	TimedOut         int64            `json:"timedOut"`
	Cancelled        int64            `json:"cancelled"`
	Reaped           int64            `json:"reaped"`
	Errors           int64            `json:"errors"`
	InQueue          int64            `json:"inQueue"`
	WhitelistSize    int64            `json:"whitelistSize"`
	SeatsSold        int64            `json:"seatsSold"`
	Capacity         uint32           `json:"capacity"`
	CapacityKnown    bool             `json:"capacityKnown"`
	MaxConcurrent    uint64           `json:"maxConcurrent"`
	Oversold         bool             `json:"oversold"`
	ElapsedMs        int64            `json:"elapsedMs"`
	Done             bool             `json:"done"`
	ReserveP50Ms     float64          `json:"reserveP50Ms"`
	ReserveP95Ms     float64          `json:"reserveP95Ms"`
	ReserveP99Ms     float64          `json:"reserveP99Ms"`
	QueueWaitP50Ms   float64          `json:"queueWaitP50Ms"`
	QueueWaitP95Ms   float64          `json:"queueWaitP95Ms"`
	PollIntervalMs   int64            `json:"pollIntervalMs"`
	Workers          int              `json:"workers"`
	Transport        Transport        `json:"transport"`
	PgAcquired       int32            `json:"pgAcquired"`
	PgIdle           int32            `json:"pgIdle"`
	PgMax            int32            `json:"pgMax"`
	RedisTotalConns  uint32           `json:"redisTotalConns"`
	RedisIdleConns   uint32           `json:"redisIdleConns"`
	RedisTimeouts    uint32           `json:"redisTimeouts"`
	ErrorCounts      map[string]int64 `json:"errorCounts"`
	Duration         time.Duration    `json:"-"`
}

type Runner struct {
	services     *service.Services
	repositories *repository.Repositories
	cfg          Config
	client       client

	started          atomic.Int64
	enqueued         atomic.Int64
	admitted         atomic.Int64
	reserved         atomic.Int64
	rejectedCapacity atomic.Int64
	rejectedQuota    atomic.Int64
	timedOut         atomic.Int64
	cancelled        atomic.Int64
	reaped           atomic.Int64
	errors           atomic.Int64

	reserveSem chan struct{}

	mu            sync.Mutex
	reserveSample []time.Duration
	waitSample    []time.Duration
	errorCounts   map[string]int64

	startedAt time.Time
	done      atomic.Bool
}

func NewRunner(services *service.Services, repositories *repository.Repositories, cfg Config) *Runner {
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.Workers > len(cfg.UserIds) {
		cfg.Workers = len(cfg.UserIds)
	}
	if cfg.PollInterval <= 0 {
		// Every waiting user polls independently, so a fixed interval turns into
		// users/interval ops per second against Redis. Scale it so the simulator does
		// not become the bottleneck it is supposed to be measuring.
		// Each waiting user polls independently, so offered load is users/interval ops
		// per second. Pick the interval from a Redis op budget rather than a fixed value,
		// so small runs stay fast and large ones do not measure the simulator's own
		// client pool instead of the system under test.
		cfg.PollInterval = time.Duration(len(cfg.UserIds)*1000/pollOpsBudget) * time.Millisecond
		if cfg.PollInterval < DefaultPollInterval {
			cfg.PollInterval = DefaultPollInterval
		}
		if cfg.PollInterval > MaxPollInterval {
			cfg.PollInterval = MaxPollInterval
		}
	}
	if cfg.UserTimeout <= 0 {
		cfg.UserTimeout = DefaultUserTimeout
	}
	if cfg.Quantity == 0 {
		cfg.Quantity = 1
	}
	if cfg.Transport == "" {
		cfg.Transport = TransportInProcess
	}

	var c client
	if cfg.Transport == TransportHTTP {
		c = newHTTPClient(cfg.BaseURL, cfg.EventId, cfg.Tokens, cfg.Workers)
	} else {
		c = &inProcessClient{services: services, repositories: repositories, eventId: cfg.EventId}
	}

	return &Runner{
		services:      services,
		repositories:  repositories,
		cfg:           cfg,
		client:        c,
		reserveSem:    make(chan struct{}, cfg.Workers),
		reserveSample: make([]time.Duration, 0, len(cfg.UserIds)),
		waitSample:    make([]time.Duration, 0, len(cfg.UserIds)),
		errorCounts:   map[string]int64{},
		startedAt:     time.Now(),
	}
}

func (r *Runner) Run(ctx context.Context) error {
	r.startedAt = time.Now()
	defer r.done.Store(true)

	if r.cfg.MaxConcurrent > 0 {
		if err := r.repositories.VirtualQueueRepository.SetEventMaxConcurrent(ctx, r.cfg.EventId, r.cfg.MaxConcurrent); err != nil {
			return err
		}
		defer func() {
			clearCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r.repositories.VirtualQueueRepository.ClearEventMaxConcurrent(clearCtx, r.cfg.EventId)
		}()
	}

	var wg sync.WaitGroup

	for _, userId := range r.cfg.UserIds {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(userId string) {
			defer wg.Done()
			r.runUser(ctx, userId)
		}(userId)
	}

	wg.Wait()
	return ctx.Err()
}

func (r *Runner) runUser(ctx context.Context, userId string) {
	r.started.Add(1)

	userCtx, cancel := context.WithTimeout(ctx, r.cfg.UserTimeout)
	defer cancel()

	enqueuedAt := time.Now()
	whitelistTTL, err := r.client.Enqueue(userCtx, userId)
	if err != nil {
		r.classify("enqueue", err)
		return
	}
	r.enqueued.Add(1)

	if whitelistTTL <= 0 {
		if !r.waitForAdmission(userCtx, userId) {
			return
		}
	}
	r.admitted.Add(1)
	r.recordWait(time.Since(enqueuedAt))

	select {
	case <-userCtx.Done():
		r.classify("reserve-queue", userCtx.Err())
		return
	case r.reserveSem <- struct{}{}:
	}
	defer func() { <-r.reserveSem }()

	reserveStart := time.Now()
	err = r.client.Reserve(userCtx, userId, r.cfg.Quantity)
	r.recordReserve(time.Since(reserveStart))

	switch {
	case err == nil:
		r.reserved.Add(1)
	case errors.Is(err, reservationRepo.ErrInsufficientCapacity):
		r.rejectedCapacity.Add(1)
	case errors.Is(err, reservationRepo.ErrExceedMaxReserveQuantity):
		r.rejectedQuota.Add(1)
	default:
		r.classify("reserve", err)
	}
}

func (r *Runner) waitForAdmission(ctx context.Context, userId string) bool {
	for {
		jitter := time.Duration(rand.Int63n(int64(r.cfg.PollInterval)))
		timer := time.NewTimer(r.cfg.PollInterval + jitter)

		select {
		case <-ctx.Done():
			timer.Stop()
			r.classify("poll", ctx.Err())
			return false
		case <-timer.C:
		}

		// One round trip: asking "am I admitted yet?" also refreshes the heartbeat,
		// because a client that is asking is by definition still alive. Two separate
		// calls doubled Redis load and starved the run at high user counts.
		status, err := r.client.Poll(ctx, userId)
		if err != nil {
			r.classify("poll", err)
			return false
		}
		if status.WhitelistTTL > 0 {
			return true
		}
		if !status.Alive {
			r.reaped.Add(1)
			return false
		}

		// Sold out: there is nothing left to be admitted for, so stop waiting for a turn
		// that only delivers a rejection.
		if status.SoldOut {
			r.rejectedCapacity.Add(1)
			r.releaseSlot(ctx, userId)
			return false
		}
	}
}

func (r *Runner) releaseSlot(ctx context.Context, userId string) {
	if err := r.client.ReleaseSlot(ctx, userId); err != nil && ctx.Err() == nil {
		log.Printf("simulation: release slot %s: %v", userId, err)
	}
}

func (r *Runner) classify(stage string, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		r.timedOut.Add(1)
	case errors.Is(err, context.Canceled):
		r.cancelled.Add(1)
	default:
		r.recordError(stage, err)
	}
}

func (r *Runner) recordError(stage string, err error) {
	r.errors.Add(1)
	r.mu.Lock()
	r.errorCounts[stage+": "+err.Error()]++
	r.mu.Unlock()
}

func (r *Runner) recordReserve(d time.Duration) {
	r.mu.Lock()
	r.reserveSample = append(r.reserveSample, d)
	r.mu.Unlock()
}

func (r *Runner) recordWait(d time.Duration) {
	r.mu.Lock()
	r.waitSample = append(r.waitSample, d)
	r.mu.Unlock()
}

func (r *Runner) Snapshot(parent context.Context) Snapshot {
	ctx, cancel := context.WithTimeout(parent, snapshotTimeout)
	defer cancel()

	s := Snapshot{
		EventId:          r.cfg.EventId,
		TotalUsers:       len(r.cfg.UserIds),
		Started:          r.started.Load(),
		Enqueued:         r.enqueued.Load(),
		Admitted:         r.admitted.Load(),
		Reserved:         r.reserved.Load(),
		RejectedCapacity: r.rejectedCapacity.Load(),
		RejectedQuota:    r.rejectedQuota.Load(),
		TimedOut:         r.timedOut.Load(),
		Cancelled:        r.cancelled.Load(),
		Reaped:           r.reaped.Load(),
		Errors:           r.errors.Load(),
		MaxConcurrent:    r.cfg.MaxConcurrent,
		Workers:          r.cfg.Workers,
		PollIntervalMs:   r.cfg.PollInterval.Milliseconds(),
		Transport:        r.cfg.Transport,
		ElapsedMs:        time.Since(r.startedAt).Milliseconds(),
		Duration:         time.Since(r.startedAt),
		Done:             r.done.Load(),
	}

	if inQueue, err := r.repositories.VirtualQueueRepository.GetTotalVirtualQueue(ctx, r.cfg.EventId); err == nil {
		s.InQueue = inQueue
	}
	if whitelisted, err := r.repositories.VirtualQueueRepository.GetTotalEventWhitelist(ctx, r.cfg.EventId); err == nil {
		s.WhitelistSize = whitelisted
	}
	if event, err := r.repositories.EventRepository.GetEvent(ctx, r.cfg.EventId); err == nil {
		s.Capacity = event.Capacity
		s.CapacityKnown = true
	}
	if sold, err := r.repositories.ReservationRepository.GetTotalReserved(ctx, r.cfg.EventId); err == nil {
		s.SeatsSold = sold
	}
	s.Oversold = s.Capacity > 0 && s.SeatsSold > int64(s.Capacity)

	pgStat := r.repositories.DbPool.Stat()
	s.PgAcquired = pgStat.AcquiredConns()
	s.PgIdle = pgStat.IdleConns()
	s.PgMax = pgStat.MaxConns()

	redisStat := r.repositories.Rdb.PoolStats()
	s.RedisTotalConns = redisStat.TotalConns
	s.RedisIdleConns = redisStat.IdleConns
	s.RedisTimeouts = redisStat.Timeouts

	r.mu.Lock()
	reserve := append([]time.Duration(nil), r.reserveSample...)
	wait := append([]time.Duration(nil), r.waitSample...)
	s.ErrorCounts = make(map[string]int64, len(r.errorCounts))
	for k, v := range r.errorCounts {
		s.ErrorCounts[k] = v
	}
	r.mu.Unlock()

	s.ReserveP50Ms = percentileMs(reserve, 0.50)
	s.ReserveP95Ms = percentileMs(reserve, 0.95)
	s.ReserveP99Ms = percentileMs(reserve, 0.99)
	s.QueueWaitP50Ms = percentileMs(wait, 0.50)
	s.QueueWaitP95Ms = percentileMs(wait, 0.95)

	return s
}

func percentileMs(samples []time.Duration, q float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })

	idx := int(float64(len(samples)-1) * q)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(samples) {
		idx = len(samples) - 1
	}

	return float64(samples[idx].Microseconds()) / 1000
}

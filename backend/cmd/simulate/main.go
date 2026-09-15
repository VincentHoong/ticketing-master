package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"ticketing-master/config"
	"ticketing-master/logging"
	"ticketing-master/repository"
	"ticketing-master/service"
	"ticketing-master/simulation"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("simulate: %v", err)
	}
}

func run() error {
	var (
		eventId       = flag.String("event", "", "event id to contend for (required)")
		users         = flag.Int("users", 500, "number of virtual users")
		quantity      = flag.Uint("quantity", 1, "seats each user attempts to reserve")
		maxConcurrent = flag.Uint64("cap", 0, "admission cap for this run (0 = leave event/global setting alone)")
		workers       = flag.Int("workers", simulation.DefaultWorkers, "max users acting concurrently")
		poll          = flag.Duration("poll", simulation.DefaultPollInterval, "how often a queued user checks for admission")
		userTimeout   = flag.Duration("user-timeout", simulation.DefaultUserTimeout, "give up on a single user after this long")
		capacity      = flag.Uint("capacity", 0, "reset the event to this capacity before running (0 = leave as-is)")
		reset         = flag.Bool("reset", true, "clear reservations and redis state before running")
	)
	flag.Parse()

	if *eventId == "" {
		flag.Usage()
		return fmt.Errorf("-event is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()
	repositories, err := repository.NewRepositories(ctx, cfg, logging.New(cfg.LogLevel))
	if err != nil {
		return err
	}
	defer repositories.Close()

	services := service.NewServices(repositories)

	if *capacity > 0 {
		if _, err := repositories.DbPool.Exec(ctx,
			`UPDATE events SET capacity = $1 WHERE id = $2`, *capacity, *eventId); err != nil {
			return err
		}
		if err := repositories.EventRepository.InvalidateCache(ctx, *eventId); err != nil {
			return err
		}
	}

	if *reset {
		if _, err := repositories.DbPool.Exec(ctx, `DELETE FROM reservations WHERE event_id = $1`, *eventId); err != nil {
			return err
		}
		if err := repositories.Rdb.FlushDB(ctx).Err(); err != nil {
			return err
		}
	}

	userIds, err := mintUsers(ctx, repositories, *users)
	if err != nil {
		return err
	}
	if len(userIds) < *users {
		log.Printf("warning: wanted %d users, got %d", *users, len(userIds))
	}

	runner := simulation.NewRunner(services, repositories, simulation.Config{
		EventId:       *eventId,
		UserIds:       userIds,
		Quantity:      uint32(*quantity),
		MaxConcurrent: *maxConcurrent,
		Workers:       *workers,
		PollInterval:  *poll,
		UserTimeout:   *userTimeout,
	})

	fmt.Printf("running %d users against event %s (workers=%d cap=%s poll=%s)\n\n",
		len(userIds), *eventId, *workers, capLabel(*maxConcurrent, cfg.MaxConcurrentQueue), *poll)

	start := time.Now()
	if err := runner.Run(ctx); err != nil {
		return err
	}
	elapsed := time.Since(start)

	printReport(runner.Snapshot(ctx), elapsed)
	return nil
}

func capLabel(runCap uint64, globalCap uint64) string {
	if runCap > 0 {
		return fmt.Sprintf("%d", runCap)
	}
	return fmt.Sprintf("%d (global)", globalCap)
}

func mintUsers(ctx context.Context, repositories *repository.Repositories, count int) ([]string, error) {
	rows, err := repositories.DbPool.Query(ctx, `
		INSERT INTO users (name, email, password_hash)
		SELECT
			'Sim User ' || g,
			'sim+' || gen_random_uuid() || '@example.com',
			'x'
		FROM generate_series(1, $1) g
		RETURNING id
	`, count)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	userIds := make([]string, 0, count)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		userIds = append(userIds, id)
	}

	return userIds, rows.Err()
}

func printReport(s simulation.Snapshot, elapsed time.Duration) {
	verdict := "PASS"
	capacity := fmt.Sprintf("%d", s.Capacity)
	if !s.CapacityKnown {
		verdict = "UNKNOWN (could not read capacity)"
		capacity = "?"
	} else if s.Oversold {
		verdict = "OVERSOLD"
	}

	fmt.Printf("seats sold      %s / %s   %s\n", fmt.Sprintf("%d", s.SeatsSold), capacity, verdict)
	fmt.Printf("wall clock      %s\n", elapsed.Round(time.Millisecond))
	if elapsed > 0 {
		fmt.Printf("throughput      %.0f users/s\n", float64(s.TotalUsers)/elapsed.Seconds())
	}
	fmt.Println()

	fmt.Println("funnel")
	fmt.Printf("  enqueued          %d\n", s.Enqueued)
	fmt.Printf("  admitted          %d\n", s.Admitted)
	fmt.Printf("  reserved          %d\n", s.Reserved)
	fmt.Printf("  rejected capacity %d\n", s.RejectedCapacity)
	fmt.Printf("  rejected quota    %d\n", s.RejectedQuota)
	fmt.Printf("  timed out         %d\n", s.TimedOut)
	fmt.Printf("  cancelled         %d\n", s.Cancelled)
	fmt.Printf("  reaped (no ping)  %d\n", s.Reaped)
	fmt.Printf("  errors            %d\n", s.Errors)
	fmt.Printf("  accounted         %d / %d\n",
		s.Reserved+s.RejectedCapacity+s.RejectedQuota+s.TimedOut+s.Cancelled+s.Reaped+s.Errors, s.TotalUsers)
	fmt.Println()

	if len(s.ErrorCounts) > 0 {
		fmt.Println("error breakdown")
		for msg, n := range s.ErrorCounts {
			fmt.Printf("  %4d  %s\n", n, msg)
		}
		fmt.Println()
	}

	fmt.Println("reserve latency, successful (db contention)")
	fmt.Printf("  p50 %.2fms   p95 %.2fms   p99 %.2fms\n", s.ReserveP50Ms, s.ReserveP95Ms, s.ReserveP99Ms)
	fmt.Println("reserve latency, rejected (capacity/quota)")
	fmt.Printf("  p50 %.2fms   p95 %.2fms   p99 %.2fms\n", s.RejectedP50Ms, s.RejectedP95Ms, s.RejectedP99Ms)
	fmt.Printf("queue wait (quantised by %dms poll)\n", s.PollIntervalMs)
	fmt.Printf("  p50 %.2fms   p95 %.2fms\n", s.QueueWaitP50Ms, s.QueueWaitP95Ms)
	fmt.Println()

	fmt.Println("leftover state")
	fmt.Printf("  still queued      %d\n", s.InQueue)
	fmt.Printf("  still whitelisted %d\n", s.WhitelistSize)
	fmt.Println()

	fmt.Println("pools")
	fmt.Printf("  postgres  %d/%d acquired, %d idle\n", s.PgAcquired, s.PgMax, s.PgIdle)
	fmt.Printf("  redis     %d conns, %d idle, %d timeouts\n", s.RedisTotalConns, s.RedisIdleConns, s.RedisTimeouts)
	fmt.Println()

	fmt.Println("note: workers call the service layer in-process; the HTTP layer is bypassed.")

	if s.Oversold {
		os.Exit(1)
	}
}

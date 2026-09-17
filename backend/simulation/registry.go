package simulation

import (
	"context"
	"errors"
	"sync"
	"time"

	"ticketing-master/repository"
	"ticketing-master/service"

	"github.com/google/uuid"
)

var ErrRunNotFound = errors.New("simulation run not found")
var ErrRunInProgress = errors.New("a simulation is already running for this event")

const runRetention = 10 * time.Minute

type Run struct {
	Id        string
	EventId   string
	StartedAt time.Time
	Runner    *Runner

	cancel context.CancelFunc
}

type Registry struct {
	services     *service.Services
	repositories *repository.Repositories

	mu      sync.Mutex
	runs    map[string]*Run
	byEvent map[string]string
}

func NewRegistry(services *service.Services, repositories *repository.Repositories) *Registry {
	return &Registry{
		services:     services,
		repositories: repositories,
		runs:         map[string]*Run{},
		byEvent:      map[string]string{},
	}
}

func (reg *Registry) Start(cfg Config) (*Run, error) {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	if runId, ok := reg.byEvent[cfg.EventId]; ok {
		if existing, ok := reg.runs[runId]; ok && !existing.Runner.done.Load() {
			return nil, ErrRunInProgress
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	run := &Run{
		Id:        uuid.NewString(),
		EventId:   cfg.EventId,
		StartedAt: time.Now(),
		Runner:    NewRunner(reg.services, reg.repositories, cfg),
		cancel:    cancel,
	}

	reg.runs[run.Id] = run
	reg.byEvent[cfg.EventId] = run.Id

	go func() {
		defer cancel()
		_ = run.Runner.Run(ctx)
		time.AfterFunc(runRetention, func() { reg.forget(run.Id, cfg.EventId) })
	}()

	return run, nil
}

// HasActiveRun reports whether an event is mid-run. Callers that mutate state before
// starting must check this first: Start's own guard rejects the duplicate, but only
// after the caller has already reset the database and Redis out from under the live run.
func (reg *Registry) HasActiveRun(eventId string) bool {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	runId, ok := reg.byEvent[eventId]
	if !ok {
		return false
	}
	existing, ok := reg.runs[runId]

	return ok && !existing.Runner.done.Load()
}

// HasAnyActiveRun reports whether any event is mid-run. For callers like a global reset
// that are not scoped to one event, checking every tracked run is the only way to catch
// a live run before mutating the tables and Redis keys it depends on.
func (reg *Registry) HasAnyActiveRun() bool {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	for _, run := range reg.runs {
		if !run.Runner.done.Load() {
			return true
		}
	}

	return false
}

func (reg *Registry) Get(runId string) (*Run, error) {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	run, ok := reg.runs[runId]
	if !ok {
		return nil, ErrRunNotFound
	}

	return run, nil
}

func (reg *Registry) Cancel(runId string) error {
	run, err := reg.Get(runId)
	if err != nil {
		return err
	}
	run.cancel()

	return nil
}

func (reg *Registry) forget(runId string, eventId string) {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	delete(reg.runs, runId)
	if reg.byEvent[eventId] == runId {
		delete(reg.byEvent, eventId)
	}
}

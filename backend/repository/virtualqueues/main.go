package virtualqueues

import (
	"context"
	"log/slog"
	"sync"
	"ticketing-master/config"
	"ticketing-master/logging"
	"time"

	"github.com/redis/go-redis/v9"
)

const promoteTickTimeout = 10 * time.Second

type VirtualQueueRepository struct {
	maxConcurrentQueue    uint64
	promoteInterval       time.Duration
	RemoteCacheRepository *RemoteCacheVirtualQueueRepository
	logger                *slog.Logger
	stopPromoterChan      chan bool
	stopPromoterOnce      sync.Once
}

type IVirtualQueueRepository interface {
	Close()
	DefaultWhitelistTTL() time.Duration
	MaxConcurrentQueue() uint64
	Ping(ctx context.Context, eventId string, userId string) (time.Duration, bool, error)
	Enqueue(ctx context.Context, eventId string, userId string) (bool, error)
	Dequeue(ctx context.Context, eventId string, userId string) (bool, bool, error)
	GetTotalVirtualQueue(ctx context.Context, eventId string) (int64, error)
	GetTotalEventWhitelist(ctx context.Context, eventId string) (int64, error)
	GetWhitelistedUserTTL(ctx context.Context, eventId string, userId string) (time.Duration, error)
	DeleteWhitelistedUserTTL(ctx context.Context, eventId string, userId string) (bool, error)
	TryWhitelistEventQueue(ctx context.Context, eventId string, queueCount uint64) (bool, error)
	PromoteActiveEvents(ctx context.Context) error
	SetEventMaxConcurrent(ctx context.Context, eventId string, maxConcurrent uint64) error
	ClearEventMaxConcurrent(ctx context.Context, eventId string) error
	GetEventMaxConcurrent(ctx context.Context, eventId string) (uint64, error)
}

func NewVirtualQueueRepository(rdb *redis.Client, cfg *config.Config, logger *slog.Logger) IVirtualQueueRepository {
	r := &VirtualQueueRepository{
		maxConcurrentQueue:    cfg.MaxConcurrentQueue,
		promoteInterval:       cfg.PromoteInterval,
		RemoteCacheRepository: newRemoteCacheVirtualQueueRepository(rdb, cfg.WhitelistTTL, cfg.HeartbeatTTL, cfg.QueueTTL),
		logger:                logger,
		stopPromoterChan:      make(chan bool),
	}

	r.startPromoterTicker()

	return r
}

func (r *VirtualQueueRepository) startPromoterTicker() {
	ticker := time.NewTicker(r.promoteInterval)
	r.logger.Info("starting virtual queue promoter ticker")

	go func() {
		defer ticker.Stop()
		defer close(r.stopPromoterChan)
		for {
			select {
			case <-r.stopPromoterChan:
				return
			case <-ticker.C:
				tickCtx, tickCancel := context.WithTimeout(logging.WithContext(context.Background(), r.logger), promoteTickTimeout)
				if err := r.PromoteActiveEvents(tickCtx); err != nil {
					r.logger.Error("virtual queue promoter", "error", err)
				}
				tickCancel()
			}
		}
	}()
}

func (r *VirtualQueueRepository) Close() {
	r.stopPromoterOnce.Do(func() {
		r.logger.Info("stopping virtual queue promoter ticker")
		r.stopPromoterChan <- true
	})
}

func (r *VirtualQueueRepository) PromoteActiveEvents(ctx context.Context) error {
	eventIds, err := r.RemoteCacheRepository.GetActiveEvents(ctx)
	if err != nil {
		return err
	}

	logger := logging.FromContext(ctx)
	for _, eventId := range eventIds {
		queued, err := r.GetTotalVirtualQueue(ctx, eventId)
		if err != nil {
			logger.Error("virtual queue promoter: queue size", "event_id", eventId, "error", err)
			continue
		}
		if queued == 0 {
			if _, err := r.RemoteCacheRepository.UntrackIdleEvent(ctx, eventId); err != nil {
				logger.Error("virtual queue promoter: untrack event", "event_id", eventId, "error", err)
			}
			continue
		}

		whitelisted, err := r.GetTotalEventWhitelist(ctx, eventId)
		if err != nil {
			logger.Error("virtual queue promoter: whitelist size", "event_id", eventId, "error", err)
			continue
		}

		maxConcurrent, err := r.RemoteCacheRepository.GetEventMaxConcurrent(ctx, eventId, r.maxConcurrentQueue)
		if err != nil {
			logger.Error("virtual queue promoter: cap", "event_id", eventId, "error", err)
			continue
		}

		free := int64(maxConcurrent) - whitelisted
		if free <= 0 {
			continue
		}
		if free > queued {
			free = queued
		}

		if _, err := r.TryWhitelistEventQueue(ctx, eventId, uint64(free)); err != nil {
			logger.Error("virtual queue promoter: admit", "event_id", eventId, "error", err)
		}
	}

	return nil
}

func (r *VirtualQueueRepository) DefaultWhitelistTTL() time.Duration {
	return r.RemoteCacheRepository.WhitelistTTL
}

func (r *VirtualQueueRepository) MaxConcurrentQueue() uint64 {
	return r.maxConcurrentQueue
}

func (r *VirtualQueueRepository) Ping(ctx context.Context, eventId string, userId string) (time.Duration, bool, error) {
	return r.RemoteCacheRepository.Ping(ctx, eventId, userId)
}

func (r *VirtualQueueRepository) Enqueue(ctx context.Context, eventId string, userId string) (bool, error) {
	return r.RemoteCacheRepository.Enqueue(ctx, eventId, userId)
}

func (r *VirtualQueueRepository) Dequeue(ctx context.Context, eventId string, userId string) (bool, bool, error) {
	return r.RemoteCacheRepository.Dequeue(ctx, eventId, userId)
}

func (r *VirtualQueueRepository) GetTotalVirtualQueue(ctx context.Context, eventId string) (int64, error) {
	return r.RemoteCacheRepository.GetTotalVirtualQueue(ctx, eventId)
}

func (r *VirtualQueueRepository) GetTotalEventWhitelist(ctx context.Context, eventId string) (int64, error) {
	return r.RemoteCacheRepository.GetTotalEventWhitelist(ctx, eventId)
}

func (r *VirtualQueueRepository) GetWhitelistedUserTTL(ctx context.Context, eventId string, userId string) (time.Duration, error) {
	return r.RemoteCacheRepository.GetWhitelistedUserTTL(ctx, eventId, userId)
}

func (r *VirtualQueueRepository) DeleteWhitelistedUserTTL(ctx context.Context, eventId string, userId string) (bool, error) {
	return r.RemoteCacheRepository.DeleteWhitelistedUserTTL(ctx, eventId, userId)
}

func (r *VirtualQueueRepository) TryWhitelistEventQueue(ctx context.Context, eventId string, queueCount uint64) (bool, error) {
	return r.RemoteCacheRepository.TryWhitelistEventQueue(ctx, eventId, queueCount, r.maxConcurrentQueue)
}

func (r *VirtualQueueRepository) SetEventMaxConcurrent(ctx context.Context, eventId string, maxConcurrent uint64) error {
	return r.RemoteCacheRepository.SetEventMaxConcurrent(ctx, eventId, maxConcurrent)
}

func (r *VirtualQueueRepository) ClearEventMaxConcurrent(ctx context.Context, eventId string) error {
	return r.RemoteCacheRepository.ClearEventMaxConcurrent(ctx, eventId)
}

func (r *VirtualQueueRepository) GetEventMaxConcurrent(ctx context.Context, eventId string) (uint64, error) {
	return r.RemoteCacheRepository.GetEventMaxConcurrent(ctx, eventId, r.maxConcurrentQueue)
}

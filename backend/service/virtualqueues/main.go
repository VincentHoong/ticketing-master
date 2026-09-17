package virtualqueues

import (
	"context"
	"log/slog"
	"ticketing-master/repository"
	"time"
)

type VirtualQueueService struct {
	Repositories *repository.Repositories
	logger       *slog.Logger
}

type QueueStatus struct {
	WhitelistTTL time.Duration
	Alive        bool
	SoldOut      bool
}

type IVirtualQueueService interface {
	Ping(ctx context.Context, eventId string, userId string) (QueueStatus, error)
	Enqueue(ctx context.Context, eventId string, userId string) (time.Duration, error)
	Dequeue(ctx context.Context, eventId string, userId string) error
	GetTotalVirtualQueue(ctx context.Context, eventId string) (int64, error)
}

func NewVirtualQueueService(repositories *repository.Repositories, logger *slog.Logger) IVirtualQueueService {
	return &VirtualQueueService{
		Repositories: repositories,
		logger:       logger,
	}
}

func (s *VirtualQueueService) Ping(ctx context.Context, eventId string, userId string) (QueueStatus, error) {
	whitelistTTL, alive, err := s.Repositories.VirtualQueueRepository.Ping(ctx, eventId, userId)
	if err != nil {
		return QueueStatus{}, err
	}

	return QueueStatus{
		WhitelistTTL: whitelistTTL,
		Alive:        alive,
		SoldOut:      s.Repositories.ReservationRepository.IsBlockEvent(ctx, eventId),
	}, nil
}

func (s *VirtualQueueService) Enqueue(ctx context.Context, eventId string, userId string) (time.Duration, error) {
	whitelistTTL, err := s.Repositories.VirtualQueueRepository.GetWhitelistedUserTTL(ctx, eventId, userId)
	if err != nil {
		return 0, err
	}
	if whitelistTTL > 0 {
		return whitelistTTL, nil
	}

	if _, err := s.Repositories.VirtualQueueRepository.Enqueue(ctx, eventId, userId); err != nil {
		return 0, err
	}

	if _, err := s.Repositories.VirtualQueueRepository.TryWhitelistEventQueue(ctx, eventId, 1); err != nil {
		return 0, err
	}

	whitelistTTL, err = s.Repositories.VirtualQueueRepository.GetWhitelistedUserTTL(ctx, eventId, userId)
	if err != nil {
		return 0, err
	}
	if whitelistTTL <= 0 {
		return 0, nil
	}

	return whitelistTTL, nil
}

func (s *VirtualQueueService) Dequeue(ctx context.Context, eventId string, userId string) error {
	_, releasedSlot, err := s.Repositories.VirtualQueueRepository.Dequeue(ctx, eventId, userId)

	if err != nil {
		return err
	}

	if releasedSlot {
		if _, err := s.Repositories.VirtualQueueRepository.TryWhitelistEventQueue(ctx, eventId, 1); err != nil {
			s.logger.Error("failed to whitelist next queued user", "error", err, "eventId", eventId)
		}
	}

	return nil
}

func (s *VirtualQueueService) GetTotalVirtualQueue(ctx context.Context, eventId string) (int64, error) {
	return s.Repositories.VirtualQueueRepository.GetTotalVirtualQueue(ctx, eventId)
}

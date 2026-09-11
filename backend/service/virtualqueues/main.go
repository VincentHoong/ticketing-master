package virtualqueues

import (
	"context"
	"ticketing-master/repository"
	"time"
)

type VirtualQueueService struct {
	Repositories *repository.Repositories
}

type IVirtualQueueService interface {
	Ping(ctx context.Context, eventId string, userId string) (bool, error)
	Enqueue(ctx context.Context, eventId string, userId string) (time.Duration, error)
	Dequeue(ctx context.Context, eventId string, userId string) error
	GetTotalVirtualQueue(ctx context.Context, eventId string) (int64, error)
}

func NewVirtualQueueService(repositories *repository.Repositories) IVirtualQueueService {
	return &VirtualQueueService{
		Repositories: repositories,
	}
}

func (s *VirtualQueueService) Ping(ctx context.Context, eventId string, userId string) (bool, error) {
	return s.Repositories.VirtualQueueRepository.Ping(ctx, eventId, userId)
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
	affected, err := s.Repositories.VirtualQueueRepository.Dequeue(ctx, eventId, userId)

	if err != nil {
		return err
	}

	if affected {
		s.Repositories.VirtualQueueRepository.TryWhitelistEventQueue(ctx, eventId, 1)
	}

	return nil
}

func (s *VirtualQueueService) GetTotalVirtualQueue(ctx context.Context, eventId string) (int64, error) {
	return s.Repositories.VirtualQueueRepository.GetTotalVirtualQueue(ctx, eventId)
}

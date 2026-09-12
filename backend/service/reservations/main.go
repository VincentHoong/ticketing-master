package reservations

import (
	"context"
	"errors"
	"ticketing-master/logging"
	"ticketing-master/repository"
	"ticketing-master/repository/reservations"
	"ticketing-master/repository/virtualqueues"
)

var ErrQueueInProgress = errors.New("queue in progress")
var ErrIdempotencyKeyReused = errors.New("idempotency key is being reused")

type IReservationService interface {
	GetReservation(ctx context.Context, id string) (*reservations.ReservationItem, error)
	ConfirmReservation(ctx context.Context, request *ConfirmReservationRequest) error
	ReleaseReservation(ctx context.Context, request *ReleaseReservationRequest) error
	ReserveEvent(ctx context.Context, request *ReserveEventRequest) (*reservations.ReservationItem, error)
}

type ReservationService struct {
	Repositories *repository.Repositories
}

func NewReservationService(repositories *repository.Repositories) IReservationService {
	return &ReservationService{Repositories: repositories}
}

type ReserveEventRequest struct {
	EventId        string  `json:"eventId"`
	UserId         string  `json:"userId"`
	Quantity       uint32  `json:"quantity"`
	IdempotencyKey *string `json:"idempotencyKey,omitempty"`
}

type ConfirmReservationRequest struct {
	UserId        string `json:"userId"`
	ReservationId string `json:"reservationId"`
}

type ReleaseReservationRequest struct {
	UserId        string `json:"userId"`
	ReservationId string `json:"reservationId"`
}

func (s *ReservationService) ReserveEvent(ctx context.Context, request *ReserveEventRequest) (*reservations.ReservationItem, error) {
	isBlockEvent := s.Repositories.ReservationRepository.IsBlockEvent(ctx, request.EventId)
	if isBlockEvent {
		s.releaseQueueSlot(ctx, request.EventId, request.UserId)
		return nil, reservations.ErrInsufficientCapacity
	}

	if request.IdempotencyKey != nil {
		cached, _ := s.Repositories.ReservationRepository.GetReservationByIdempotencyKey(ctx, request.EventId, request.UserId, *request.IdempotencyKey)
		if cached != nil {
			if cached.Quantity != request.Quantity {
				return nil, ErrIdempotencyKeyReused
			}
			return cached, nil
		}
	}
	whitelistTTL, err := s.Repositories.VirtualQueueRepository.GetWhitelistedUserTTL(ctx, request.EventId, request.UserId)
	if err != nil {
		return nil, err
	}
	if whitelistTTL <= 0 {
		return nil, ErrQueueInProgress
	}

	event, err := s.Repositories.EventRepository.GetEvent(ctx, request.EventId)
	if err != nil {
		return nil, err
	}

	reservationItem, isCapped, err := s.Repositories.ReservationRepository.ReserveEvent(
		ctx,
		request.EventId,
		request.UserId,
		request.Quantity,
		request.IdempotencyKey,
		event.MaxReservePerUser,
	)
	if isCapped {
		s.Repositories.ReservationRepository.BlockEvent(ctx, request.EventId)
	}
	if err != nil {
		if errors.Is(err, reservations.ErrInsufficientCapacity) ||
			errors.Is(err, reservations.ErrExceedMaxReserveQuantity) {
			s.releaseQueueSlot(ctx, request.EventId, request.UserId)
		}
		return nil, err
	}
	if reservationItem.IdempotencyKey != nil {
		err := s.Repositories.ReservationRepository.SetReservationByIdempotencyKey(ctx, request.EventId, request.UserId, *reservationItem.IdempotencyKey, reservationItem)
		if err != nil {
			logging.FromContext(ctx).Error("failed to cache reservation", "error", err)
		}
	}

	s.releaseQueueSlot(ctx, request.EventId, request.UserId)

	return reservationItem, nil
}

func (s *ReservationService) releaseQueueSlot(ctx context.Context, eventId string, userId string) {
	logger := logging.FromContext(ctx)
	if _, err := s.Repositories.VirtualQueueRepository.DeleteWhitelistedUserTTL(ctx, eventId, userId); err != nil &&
		!errors.Is(err, virtualqueues.ErrMissingWhitelistKey) {
		logger.Error("failed to release whitelist slot", "user_id", userId, "event_id", eventId, "error", err)
	}
	if _, err := s.Repositories.VirtualQueueRepository.TryWhitelistEventQueue(ctx, eventId, 1); err != nil {
		logger.Error("failed to admit next user", "event_id", eventId, "error", err)
	}
}

func (s *ReservationService) ConfirmReservation(ctx context.Context, request *ConfirmReservationRequest) error {
	err := s.Repositories.ReservationRepository.ConfirmReservation(ctx, request.UserId, request.ReservationId)
	if errors.Is(err, reservations.ErrReservationNotFound) {
		if res, e := s.Repositories.ReservationRepository.GetReservation(ctx, request.ReservationId); e == nil &&
			res.UserId == request.UserId && res.Status == reservations.StatusConfirmed {
			return nil
		}
	}
	return err
}

func (s *ReservationService) ReleaseReservation(ctx context.Context, request *ReleaseReservationRequest) error {
	return s.Repositories.ReservationRepository.ReleaseReservation(ctx, request.UserId, request.ReservationId)
}

func (s *ReservationService) GetReservation(ctx context.Context, id string) (*reservations.ReservationItem, error) {
	return s.Repositories.ReservationRepository.GetReservation(ctx, id)
}

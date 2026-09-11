package reservations

import (
	"context"
	"errors"
	"log"
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
	if request.IdempotencyKey != nil {
		reservationItem, _ := s.Repositories.ReservationRepository.GetReservationByIdempotencyKey(ctx, request.EventId, request.UserId, *request.IdempotencyKey)
		if reservationItem != nil {
			if reservationItem.Quantity != request.Quantity {
				return nil, ErrIdempotencyKeyReused
			}
			return reservationItem, nil
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

	reservationItem, err := s.Repositories.ReservationRepository.ReserveEvent(
		ctx,
		request.EventId,
		request.UserId,
		request.Quantity,
		request.IdempotencyKey,
		event.MaxReservePerUser,
	)
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
			log.Printf("failed to cache reservation %v", err)
		}
	}

	s.releaseQueueSlot(ctx, request.EventId, request.UserId)

	return reservationItem, nil
}

func (s *ReservationService) releaseQueueSlot(ctx context.Context, eventId string, userId string) {
	if _, err := s.Repositories.VirtualQueueRepository.DeleteWhitelistedUserTTL(ctx, eventId, userId); err != nil &&
		!errors.Is(err, virtualqueues.ErrMissingWhitelistKey) {
		log.Printf("failed to release whitelist slot for user %s on event %s: %v", userId, eventId, err)
	}
	if _, err := s.Repositories.VirtualQueueRepository.TryWhitelistEventQueue(ctx, eventId, 1); err != nil {
		log.Printf("failed to admit next user for event %s: %v", eventId, err)
	}
}

func (s *ReservationService) ConfirmReservation(ctx context.Context, request *ConfirmReservationRequest) error {
	return s.Repositories.ReservationRepository.ConfirmReservation(ctx, request.UserId, request.ReservationId)
}

func (s *ReservationService) ReleaseReservation(ctx context.Context, request *ReleaseReservationRequest) error {
	return s.Repositories.ReservationRepository.ReleaseReservation(ctx, request.UserId, request.ReservationId)
}

func (s *ReservationService) GetReservation(ctx context.Context, id string) (*reservations.ReservationItem, error) {
	return s.Repositories.ReservationRepository.GetReservation(ctx, id)
}

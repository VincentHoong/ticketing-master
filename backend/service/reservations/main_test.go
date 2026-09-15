package reservations

import (
	"context"
	"errors"
	"testing"
	"ticketing-master/repository"
	"ticketing-master/repository/events"
	"ticketing-master/repository/reservations"
	"ticketing-master/repository/virtualqueues"
	"time"
)

type fakeReservationRepository struct {
	reservations.IReservationRepository

	isBlocked            bool
	reserveEventItem     *reservations.ReservationItem
	reserveEventIsCapped bool
	reserveEventErr      error

	blockEventCalls int
}

func (f *fakeReservationRepository) IsBlockEvent(ctx context.Context, eventId string) bool {
	return f.isBlocked
}

func (f *fakeReservationRepository) GetReservationByIdempotencyKey(ctx context.Context, eventId string, userId string, idempotencyKey string) (*reservations.ReservationItem, error) {
	return nil, nil
}

func (f *fakeReservationRepository) ReserveEvent(ctx context.Context, eventId string, userId string, quantity uint32, idempotencyKey *string, maxReservePerUser uint32) (*reservations.ReservationItem, bool, error) {
	return f.reserveEventItem, f.reserveEventIsCapped, f.reserveEventErr
}

func (f *fakeReservationRepository) BlockEvent(ctx context.Context, eventId string) bool {
	f.blockEventCalls++
	return false
}

func (f *fakeReservationRepository) SetReservationByIdempotencyKey(ctx context.Context, eventId string, userId string, idempotencyKey string, reservationItem *reservations.ReservationItem) error {
	return nil
}

type fakeVirtualQueueRepository struct {
	virtualqueues.IVirtualQueueRepository

	whitelistTTL time.Duration
}

func (f *fakeVirtualQueueRepository) GetWhitelistedUserTTL(ctx context.Context, eventId string, userId string) (time.Duration, error) {
	return f.whitelistTTL, nil
}

func (f *fakeVirtualQueueRepository) DeleteWhitelistedUserTTL(ctx context.Context, eventId string, userId string) (bool, error) {
	return true, nil
}

func (f *fakeVirtualQueueRepository) TryWhitelistEventQueue(ctx context.Context, eventId string, queueCount uint64) (bool, error) {
	return true, nil
}

type fakeEventRepository struct {
	events.IEventRepository
}

func (f *fakeEventRepository) GetEvent(ctx context.Context, id string) (*events.EventItem, error) {
	return &events.EventItem{Id: id, Capacity: 10, MaxReservePerUser: 4}, nil
}

func TestReserveEvent_BlocksEventWhenCappedEvenOnError(t *testing.T) {
	ctx := context.Background()

	reservationRepo := &fakeReservationRepository{
		reserveEventItem:     nil,
		reserveEventIsCapped: true,
		reserveEventErr:      reservations.ErrInsufficientCapacity,
	}

	svc := &ReservationService{
		Repositories: &repository.Repositories{
			ReservationRepository:  reservationRepo,
			VirtualQueueRepository: &fakeVirtualQueueRepository{whitelistTTL: time.Minute},
			EventRepository:        &fakeEventRepository{},
		},
	}

	_, err := svc.ReserveEvent(ctx, &ReserveEventRequest{EventId: "event-1", UserId: "user-1", Quantity: 1})

	if !errors.Is(err, reservations.ErrInsufficientCapacity) {
		t.Fatalf("expected ErrInsufficientCapacity, got %v", err)
	}
	if reservationRepo.blockEventCalls != 1 {
		t.Fatalf("expected BlockEvent to be called once even though ReserveEvent errored, got %d calls", reservationRepo.blockEventCalls)
	}
}

func TestReserveEvent_DoesNotBlockWhenNotCapped(t *testing.T) {
	ctx := context.Background()

	reservationRepo := &fakeReservationRepository{
		reserveEventItem:     &reservations.ReservationItem{Id: "res-1"},
		reserveEventIsCapped: false,
		reserveEventErr:      nil,
	}

	svc := &ReservationService{
		Repositories: &repository.Repositories{
			ReservationRepository:  reservationRepo,
			VirtualQueueRepository: &fakeVirtualQueueRepository{whitelistTTL: time.Minute},
			EventRepository:        &fakeEventRepository{},
		},
	}

	_, err := svc.ReserveEvent(ctx, &ReserveEventRequest{EventId: "event-1", UserId: "user-1", Quantity: 1})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if reservationRepo.blockEventCalls != 0 {
		t.Fatalf("expected BlockEvent not to be called, got %d calls", reservationRepo.blockEventCalls)
	}
}

func TestReserveEvent_ShortCircuitsWhenAlreadyBlocked(t *testing.T) {
	ctx := context.Background()

	reservationRepo := &fakeReservationRepository{isBlocked: true}

	svc := &ReservationService{
		Repositories: &repository.Repositories{
			ReservationRepository:  reservationRepo,
			VirtualQueueRepository: &fakeVirtualQueueRepository{whitelistTTL: time.Minute},
			EventRepository:        &fakeEventRepository{},
		},
	}

	_, err := svc.ReserveEvent(ctx, &ReserveEventRequest{EventId: "event-1", UserId: "user-1", Quantity: 1})

	if !errors.Is(err, reservations.ErrInsufficientCapacity) {
		t.Fatalf("expected ErrInsufficientCapacity, got %v", err)
	}
}

//go:build integration

package integrationtest

import (
	"errors"
	"log/slog"
	"testing"

	"ticketing-master/repository/reservations"
	svcreservations "ticketing-master/service/reservations"
	svcvirtualqueues "ticketing-master/service/virtualqueues"
)

func TestService_HappyPath_EnqueueReserveConfirm(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	reservationSvc := svcreservations.NewReservationService(repos)
	queueSvc := svcvirtualqueues.NewVirtualQueueService(repos, slog.Default())

	userId := createTestUser(t, repos)
	event := createTestEvent(t, repos, 5, 5)

	ttl, err := queueSvc.Enqueue(ctx, event.Id, userId)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("expected immediate admission with room available, got ttl=%v", ttl)
	}

	item, err := reservationSvc.ReserveEvent(ctx, &svcreservations.ReserveEventRequest{
		EventId:  event.Id,
		UserId:   userId,
		Quantity: 1,
	})
	if err != nil {
		t.Fatalf("reserve event: %v", err)
	}

	if err := reservationSvc.ConfirmReservation(ctx, &svcreservations.ConfirmReservationRequest{
		UserId:        userId,
		ReservationId: item.Id,
	}); err != nil {
		t.Fatalf("confirm reservation: %v", err)
	}
}

func TestService_Unhappy_ReserveWithoutEnqueue(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	reservationSvc := svcreservations.NewReservationService(repos)

	userId := createTestUser(t, repos)
	event := createTestEvent(t, repos, 5, 5)

	_, err := reservationSvc.ReserveEvent(ctx, &svcreservations.ReserveEventRequest{
		EventId:  event.Id,
		UserId:   userId,
		Quantity: 1,
	})
	if !errors.Is(err, svcreservations.ErrQueueInProgress) {
		t.Fatalf("expected ErrQueueInProgress when reserving without enqueueing first, got %v", err)
	}
}

func TestService_Unhappy_ReserveOnBlockedEventReleasesQueueSlot(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	reservationSvc := svcreservations.NewReservationService(repos)
	queueSvc := svcvirtualqueues.NewVirtualQueueService(repos, slog.Default())

	event := createTestEvent(t, repos, 1, 1)

	fillerUser := createTestUser(t, repos)
	if _, err := queueSvc.Enqueue(ctx, event.Id, fillerUser); err != nil {
		t.Fatalf("enqueue filler: %v", err)
	}
	if _, err := reservationSvc.ReserveEvent(ctx, &svcreservations.ReserveEventRequest{
		EventId:  event.Id,
		UserId:   fillerUser,
		Quantity: 1,
	}); err != nil {
		t.Fatalf("filler reserve: %v", err)
	}
	if !repos.ReservationRepository.IsBlockEvent(ctx, event.Id) {
		t.Fatalf("expected event to be blocked after capacity filled")
	}

	blockedUser := createTestUser(t, repos)
	ttl, err := queueSvc.Enqueue(ctx, event.Id, blockedUser)
	if err != nil {
		t.Fatalf("enqueue blocked user: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("expected blockedUser to be admitted to the whitelist despite the sold-out event")
	}

	_, err = reservationSvc.ReserveEvent(ctx, &svcreservations.ReserveEventRequest{
		EventId:  event.Id,
		UserId:   blockedUser,
		Quantity: 1,
	})
	if !errors.Is(err, reservations.ErrInsufficientCapacity) {
		t.Fatalf("expected ErrInsufficientCapacity, got %v", err)
	}

	remainingTTL, err := repos.VirtualQueueRepository.GetWhitelistedUserTTL(ctx, event.Id, blockedUser)
	if err == nil && remainingTTL > 0 {
		t.Fatalf("expected blockedUser's whitelist slot to be released after being bounced, still has ttl=%v", remainingTTL)
	}
}

func TestService_Unhappy_IdempotencyKeyReuseWithDifferentQuantity(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	reservationSvc := svcreservations.NewReservationService(repos)
	queueSvc := svcvirtualqueues.NewVirtualQueueService(repos, slog.Default())

	userId := createTestUser(t, repos)
	event := createTestEvent(t, repos, 10, 10)

	if _, err := queueSvc.Enqueue(ctx, event.Id, userId); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	key := "idem-key-1"
	first, err := reservationSvc.ReserveEvent(ctx, &svcreservations.ReserveEventRequest{
		EventId:        event.Id,
		UserId:         userId,
		Quantity:       2,
		IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}

	if _, err := reservationSvc.ReserveEvent(ctx, &svcreservations.ReserveEventRequest{
		EventId:        event.Id,
		UserId:         userId,
		Quantity:       3,
		IdempotencyKey: &key,
	}); !errors.Is(err, svcreservations.ErrIdempotencyKeyReused) {
		t.Fatalf("expected ErrIdempotencyKeyReused, got %v", err)
	}

	second, err := reservationSvc.ReserveEvent(ctx, &svcreservations.ReserveEventRequest{
		EventId:        event.Id,
		UserId:         userId,
		Quantity:       2,
		IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("same key/quantity reserve: %v", err)
	}
	if second.Id != first.Id {
		t.Fatalf("expected the cached reservation to be returned, got a different id")
	}
}

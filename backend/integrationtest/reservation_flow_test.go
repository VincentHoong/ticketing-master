//go:build integration

package integrationtest

import (
	"errors"
	"testing"

	"ticketing-master/repository/reservations"
)

func TestReservation_HappyPath_ReserveThenConfirm(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	userId := createTestUser(t, repos)
	event := createTestEvent(t, repos, 10, 5)

	item, isCapped, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, userId, 2, nil, event.MaxReservePerUser)
	if err != nil {
		t.Fatalf("reserve event: %v", err)
	}
	if isCapped {
		t.Fatalf("expected not capped with capacity=10, quantity=2")
	}
	if item.Status != reservations.StatusHeld {
		t.Fatalf("expected status held, got %s", item.Status)
	}

	if err := repos.ReservationRepository.ConfirmReservation(ctx, userId, item.Id); err != nil {
		t.Fatalf("confirm reservation: %v", err)
	}

	confirmed, err := repos.ReservationRepository.GetReservation(ctx, item.Id)
	if err != nil {
		t.Fatalf("get reservation: %v", err)
	}
	if confirmed.Status != reservations.StatusConfirmed {
		t.Fatalf("expected status confirmed, got %s", confirmed.Status)
	}
}

func TestReservation_HappyPath_ReserveThenRelease(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	userId := createTestUser(t, repos)
	event := createTestEvent(t, repos, 5, 5)

	item, _, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, userId, 5, nil, event.MaxReservePerUser)
	if err != nil {
		t.Fatalf("reserve event: %v", err)
	}

	otherUser := createTestUser(t, repos)
	if _, _, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, otherUser, 1, nil, event.MaxReservePerUser); !errors.Is(err, reservations.ErrInsufficientCapacity) {
		t.Fatalf("expected ErrInsufficientCapacity while event is fully held, got %v", err)
	}

	if err := repos.ReservationRepository.ReleaseReservation(ctx, userId, item.Id); err != nil {
		t.Fatalf("release reservation: %v", err)
	}

	if _, _, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, otherUser, 5, nil, event.MaxReservePerUser); err != nil {
		t.Fatalf("expected reserve to succeed after release, got %v", err)
	}
}

func TestReservation_Unhappy_InsufficientCapacityBlocksEvent(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	userId := createTestUser(t, repos)
	event := createTestEvent(t, repos, 3, 5)

	_, isCapped, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, userId, 3, nil, event.MaxReservePerUser)
	if err != nil {
		t.Fatalf("reserve event: %v", err)
	}
	if !isCapped {
		t.Fatalf("expected isCapped=true when reservation fills capacity")
	}
	repos.ReservationRepository.BlockEvent(ctx, event.Id)

	if !repos.ReservationRepository.IsBlockEvent(ctx, event.Id) {
		t.Fatalf("expected event to be blocked after capping")
	}

	otherUser := createTestUser(t, repos)
	if _, _, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, otherUser, 1, nil, event.MaxReservePerUser); !errors.Is(err, reservations.ErrInsufficientCapacity) {
		t.Fatalf("expected ErrInsufficientCapacity, got %v", err)
	}
}

func TestReservation_Unhappy_ExceedsMaxReservePerUser(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	userId := createTestUser(t, repos)
	event := createTestEvent(t, repos, 100, 2)

	if _, _, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, userId, 3, nil, event.MaxReservePerUser); !errors.Is(err, reservations.ErrExceedMaxReserveQuantity) {
		t.Fatalf("expected ErrExceedMaxReserveQuantity, got %v", err)
	}
}

func TestReservation_Unhappy_ConfirmAlreadyReleased(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	userId := createTestUser(t, repos)
	event := createTestEvent(t, repos, 5, 5)

	item, _, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, userId, 1, nil, event.MaxReservePerUser)
	if err != nil {
		t.Fatalf("reserve event: %v", err)
	}
	if err := repos.ReservationRepository.ReleaseReservation(ctx, userId, item.Id); err != nil {
		t.Fatalf("release reservation: %v", err)
	}

	if err := repos.ReservationRepository.ConfirmReservation(ctx, userId, item.Id); !errors.Is(err, reservations.ErrReservationNotFound) {
		t.Fatalf("expected ErrReservationNotFound for an already-released reservation, got %v", err)
	}
}

func TestReservation_Unhappy_ConfirmWrongUser(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	userId := createTestUser(t, repos)
	otherUser := createTestUser(t, repos)
	event := createTestEvent(t, repos, 5, 5)

	item, _, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, userId, 1, nil, event.MaxReservePerUser)
	if err != nil {
		t.Fatalf("reserve event: %v", err)
	}

	if err := repos.ReservationRepository.ConfirmReservation(ctx, otherUser, item.Id); !errors.Is(err, reservations.ErrReservationNotFound) {
		t.Fatalf("expected ErrReservationNotFound for wrong user, got %v", err)
	}
}

func TestReservation_SweepClearsExpiredHoldsAndUnblocks(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	userId := createTestUser(t, repos)
	event := createTestEvent(t, repos, 1, 1)

	item, isCapped, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, userId, 1, nil, event.MaxReservePerUser)
	if err != nil {
		t.Fatalf("reserve event: %v", err)
	}
	if !isCapped {
		t.Fatalf("expected isCapped=true")
	}
	repos.ReservationRepository.BlockEvent(ctx, event.Id)

	if _, err := repos.DbPool.Exec(ctx, `UPDATE reservations SET expires_at = now() - interval '1 minute' WHERE id = $1`, item.Id); err != nil {
		t.Fatalf("force-expire reservation: %v", err)
	}

	if err := repos.ReservationRepository.RefreshEventStatus(ctx); err != nil {
		t.Fatalf("refresh event status: %v", err)
	}

	if repos.ReservationRepository.IsBlockEvent(ctx, event.Id) {
		t.Fatalf("expected event to be unblocked after sweep clears the expired hold")
	}

	expired, err := repos.ReservationRepository.GetReservation(ctx, item.Id)
	if err != nil {
		t.Fatalf("get reservation: %v", err)
	}
	if expired.Status != reservations.StatusExpired {
		t.Fatalf("expected status expired, got %s", expired.Status)
	}
}

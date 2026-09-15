//go:build integration

package integrationtest

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ticketing-master/config"
	"ticketing-master/repository/reservations"
	svcreservations "ticketing-master/service/reservations"
	svcvirtualqueues "ticketing-master/service/virtualqueues"
)

func TestRace_ReservationCapacityEnforcedUnderConcurrency(t *testing.T) {
	const capacity = 5
	const contenders = 20

	repos := newTestRepositories(t, func(cfg *config.Config) {
		cfg.MaxConcurrentQueue = contenders
	})
	resetState(t, repos)
	ctx := newTestCtx(t)

	reservationSvc := svcreservations.NewReservationService(repos)
	queueSvc := svcvirtualqueues.NewVirtualQueueService(repos)

	event := createTestEvent(t, repos, capacity, 1)

	userIds := make([]string, contenders)
	for i := range userIds {
		userIds[i] = createTestUser(t, repos)
		if _, err := queueSvc.Enqueue(ctx, event.Id, userIds[i]); err != nil {
			t.Fatalf("enqueue user %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	var succeeded, failed int64
	for _, userId := range userIds {
		wg.Add(1)
		go func(userId string) {
			defer wg.Done()
			_, err := reservationSvc.ReserveEvent(ctx, &svcreservations.ReserveEventRequest{
				EventId:  event.Id,
				UserId:   userId,
				Quantity: 1,
			})
			switch {
			case err == nil:
				atomic.AddInt64(&succeeded, 1)
			case errors.Is(err, reservations.ErrInsufficientCapacity):
				atomic.AddInt64(&failed, 1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(userId)
	}
	wg.Wait()

	if succeeded != capacity {
		t.Fatalf("expected exactly %d successful reservations, got %d (failed=%d)", capacity, succeeded, failed)
	}

	total, err := repos.ReservationRepository.GetTotalReserved(ctx, event.Id)
	if err != nil {
		t.Fatalf("get total reserved: %v", err)
	}
	if total != capacity {
		t.Fatalf("expected total reserved to equal capacity=%d, got %d (would indicate overselling)", capacity, total)
	}
}

func TestRace_WhitelistAdmissionNeverExceedsMaxConcurrent(t *testing.T) {
	const maxConcurrent = 3
	const contenders = 20

	repos := newTestRepositories(t, func(cfg *config.Config) {
		cfg.MaxConcurrentQueue = maxConcurrent
		cfg.PromoteInterval = 20 * time.Millisecond
	})
	resetState(t, repos)
	ctx := newTestCtx(t)

	event := createTestEvent(t, repos, 100, 100)

	userIds := make([]string, contenders)
	for i := range userIds {
		userIds[i] = createTestUser(t, repos)
	}

	var wg sync.WaitGroup
	for _, userId := range userIds {
		wg.Add(1)
		go func(userId string) {
			defer wg.Done()
			if _, err := repos.VirtualQueueRepository.Enqueue(ctx, event.Id, userId); err != nil {
				t.Errorf("enqueue: %v", err)
				return
			}
			if _, err := repos.VirtualQueueRepository.TryWhitelistEventQueue(ctx, event.Id, 1); err != nil {
				t.Errorf("try whitelist: %v", err)
			}
		}(userId)
	}
	wg.Wait()

	whitelisted, err := repos.VirtualQueueRepository.GetTotalEventWhitelist(ctx, event.Id)
	if err != nil {
		t.Fatalf("get total whitelist: %v", err)
	}
	if whitelisted > maxConcurrent {
		t.Fatalf("expected at most %d whitelisted concurrently, got %d (would indicate an over-admission race)", maxConcurrent, whitelisted)
	}
}

func TestRace_ConcurrentReserveAndReleaseNoDoubleBooking(t *testing.T) {
	const rounds = 30

	repos := newTestRepositories(t, func(cfg *config.Config) {
		cfg.MaxConcurrentQueue = 2
	})
	resetState(t, repos)
	ctx := newTestCtx(t)

	reservationSvc := svcreservations.NewReservationService(repos)
	queueSvc := svcvirtualqueues.NewVirtualQueueService(repos)

	event := createTestEvent(t, repos, 1, 1)

	for i := 0; i < rounds; i++ {
		userA := createTestUser(t, repos)
		userB := createTestUser(t, repos)

		if _, err := queueSvc.Enqueue(ctx, event.Id, userA); err != nil {
			t.Fatalf("round %d: enqueue A: %v", i, err)
		}
		if _, err := queueSvc.Enqueue(ctx, event.Id, userB); err != nil {
			t.Fatalf("round %d: enqueue B: %v", i, err)
		}

		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, userId := range []string{userA, userB} {
			wg.Add(1)
			go func(userId string) {
				defer wg.Done()
				_, err := reservationSvc.ReserveEvent(ctx, &svcreservations.ReserveEventRequest{
					EventId:  event.Id,
					UserId:   userId,
					Quantity: 1,
				})
				results <- err
			}(userId)
		}
		wg.Wait()
		close(results)

		succeeded := 0
		for err := range results {
			if err == nil {
				succeeded++
			} else if !errors.Is(err, reservations.ErrInsufficientCapacity) {
				t.Fatalf("round %d: unexpected error: %v", i, err)
			}
		}
		if succeeded != 1 {
			t.Fatalf("round %d: expected exactly 1 success for a capacity-1 event, got %d", i, succeeded)
		}

		total, err := repos.ReservationRepository.GetTotalReserved(ctx, event.Id)
		if err != nil {
			t.Fatalf("round %d: get total reserved: %v", i, err)
		}
		if total != 1 {
			t.Fatalf("round %d: expected total reserved=1, got %d (would indicate double-booking)", i, total)
		}

		var reservationId, ownerId string
		err = repos.DbPool.QueryRow(ctx, `
			SELECT id, user_id FROM reservations
			WHERE event_id = $1 AND status = $2 LIMIT 1
		`, event.Id, reservations.StatusHeld).Scan(&reservationId, &ownerId)
		if err != nil {
			t.Fatalf("round %d: find held reservation: %v", i, err)
		}

		if err := repos.ReservationRepository.ReleaseReservation(ctx, ownerId, reservationId); err != nil {
			t.Fatalf("round %d: release reservation: %v", i, err)
		}
	}
}

// TestRace_ConcurrentIdempotentReserveReturnsSameReservation exercises the repository
// directly (bypassing the service layer's pre-check, which is what actually races) to
// prove the unique-constraint fallback: two callers sharing an idempotency key must both
// get back the *same* reservation, not one success and one raw constraint error.
func TestRace_ConcurrentIdempotentReserveReturnsSameReservation(t *testing.T) {
	const contenders = 20

	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	event := createTestEvent(t, repos, 100, 100)
	userId := createTestUser(t, repos)
	key := "shared-key"

	var wg sync.WaitGroup
	results := make(chan struct {
		item *reservations.ReservationItem
		err  error
	}, contenders)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item, _, err := repos.ReservationRepository.ReserveEvent(ctx, event.Id, userId, 1, &key, event.MaxReservePerUser)
			results <- struct {
				item *reservations.ReservationItem
				err  error
			}{item, err}
		}()
	}
	wg.Wait()
	close(results)

	var firstId string
	for r := range results {
		if r.err != nil {
			t.Fatalf("expected every concurrent idempotent request to succeed, got: %v", r.err)
		}
		if firstId == "" {
			firstId = r.item.Id
		} else if r.item.Id != firstId {
			t.Fatalf("expected all concurrent requests to resolve to the same reservation id, got %q and %q", firstId, r.item.Id)
		}
	}

	total, err := repos.ReservationRepository.GetTotalReserved(ctx, event.Id)
	if err != nil {
		t.Fatalf("get total reserved: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected exactly 1 reservation despite %d concurrent identical requests, got total=%d", contenders, total)
	}
}

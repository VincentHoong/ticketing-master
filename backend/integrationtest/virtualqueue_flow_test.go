//go:build integration

package integrationtest

import (
	"errors"
	"testing"
	"time"

	"ticketing-master/config"
	"ticketing-master/repository/virtualqueues"
)

func TestVirtualQueue_HappyPath_EnqueueWithRoomAdmitsImmediately(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	event := createTestEvent(t, repos, 10, 10)
	userId := createTestUser(t, repos)

	admitted, err := repos.VirtualQueueRepository.Enqueue(ctx, event.Id, userId)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if !admitted {
		t.Fatalf("expected the user to be newly added to the queue")
	}

	if _, err := repos.VirtualQueueRepository.TryWhitelistEventQueue(ctx, event.Id, 1); err != nil {
		t.Fatalf("try whitelist: %v", err)
	}

	ttl, err := repos.VirtualQueueRepository.GetWhitelistedUserTTL(ctx, event.Id, userId)
	if err != nil {
		t.Fatalf("get whitelisted user ttl: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("expected user to be whitelisted with a positive ttl, got %v", ttl)
	}

	whitelistTTL, alive, err := repos.VirtualQueueRepository.Ping(ctx, event.Id, userId)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if !alive || whitelistTTL <= 0 {
		t.Fatalf("expected ping to report admitted, alive=%v ttl=%v", alive, whitelistTTL)
	}
}

func TestVirtualQueue_HappyPath_PromotesNextUserWhenSlotFrees(t *testing.T) {
	repos := newTestRepositories(t, func(cfg *config.Config) {
		cfg.MaxConcurrentQueue = 1
		cfg.PromoteInterval = 50 * time.Millisecond
	})
	resetState(t, repos)
	ctx := newTestCtx(t)

	event := createTestEvent(t, repos, 10, 10)
	first := createTestUser(t, repos)
	second := createTestUser(t, repos)

	if _, err := repos.VirtualQueueRepository.Enqueue(ctx, event.Id, first); err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	if _, err := repos.VirtualQueueRepository.TryWhitelistEventQueue(ctx, event.Id, 1); err != nil {
		t.Fatalf("try whitelist first: %v", err)
	}
	if _, err := repos.VirtualQueueRepository.Enqueue(ctx, event.Id, second); err != nil {
		t.Fatalf("enqueue second: %v", err)
	}

	secondTTL, err := repos.VirtualQueueRepository.GetWhitelistedUserTTL(ctx, event.Id, second)
	if err == nil && secondTTL > 0 {
		t.Fatalf("expected second user to remain queued while max_concurrent_queue=1 is full, got ttl=%v", secondTTL)
	}

	if _, err := repos.VirtualQueueRepository.DeleteWhitelistedUserTTL(ctx, event.Id, first); err != nil {
		t.Fatalf("release first's slot: %v", err)
	}

	waitFor(t, 2*time.Second, 50*time.Millisecond, func() bool {
		ttl, err := repos.VirtualQueueRepository.GetWhitelistedUserTTL(ctx, event.Id, second)
		return err == nil && ttl > 0
	})
}

func TestVirtualQueue_Unhappy_EnqueueSameUserTwiceBeforePromotion(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	event := createTestEvent(t, repos, 10, 10)
	userId := createTestUser(t, repos)

	if _, err := repos.VirtualQueueRepository.Enqueue(ctx, event.Id, userId); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}

	_, err := repos.VirtualQueueRepository.Enqueue(ctx, event.Id, userId)
	if !errors.Is(err, virtualqueues.ErrVirtualQueueExist) {
		t.Fatalf("expected ErrVirtualQueueExist on duplicate enqueue, got %v", err)
	}
}

func TestVirtualQueue_Unhappy_DequeueNeverEnqueuedUser(t *testing.T) {
	repos := newTestRepositories(t)
	resetState(t, repos)
	ctx := newTestCtx(t)

	event := createTestEvent(t, repos, 10, 10)
	userId := createTestUser(t, repos)

	removed, releasedSlot, err := repos.VirtualQueueRepository.Dequeue(ctx, event.Id, userId)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if removed || releasedSlot {
		t.Fatalf("expected a no-op dequeue for a user who was never enqueued, got removed=%v releasedSlot=%v", removed, releasedSlot)
	}
}

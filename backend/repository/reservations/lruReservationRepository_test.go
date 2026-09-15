package reservations

import (
	"context"
	"testing"
	"time"
)

func TestLRUReservationRepository_BlockThenIsBlock(t *testing.T) {
	ctx := context.Background()
	r, err := newLRUReservationRepositoryWithTTL(time.Minute)
	if err != nil {
		t.Fatalf("newLRUReservationRepositoryWithTTL: %v", err)
	}

	r.BlockEvent(ctx, "event-1")

	if !r.IsBlockEvent(ctx, "event-1") {
		t.Fatal("expected event-1 to be blocked immediately after BlockEvent")
	}
}

func TestLRUReservationRepository_ExpiresAfterTTL(t *testing.T) {
	ctx := context.Background()
	ttl := 20 * time.Millisecond
	r, err := newLRUReservationRepositoryWithTTL(ttl)
	if err != nil {
		t.Fatalf("newLRUReservationRepositoryWithTTL: %v", err)
	}

	r.BlockEvent(ctx, "event-1")

	if !r.IsBlockEvent(ctx, "event-1") {
		t.Fatal("expected event-1 to be blocked before TTL elapses")
	}

	time.Sleep(ttl * 3)

	if r.IsBlockEvent(ctx, "event-1") {
		t.Fatal("expected event-1 to be unblocked after TTL elapses")
	}
}

func TestLRUReservationRepository_UnblockClearsBeforeTTL(t *testing.T) {
	ctx := context.Background()
	r, err := newLRUReservationRepositoryWithTTL(time.Minute)
	if err != nil {
		t.Fatalf("newLRUReservationRepositoryWithTTL: %v", err)
	}

	r.BlockEvent(ctx, "event-1")
	r.UnblockEvent(ctx, "event-1")

	if r.IsBlockEvent(ctx, "event-1") {
		t.Fatal("expected event-1 to be unblocked immediately after UnblockEvent, before TTL")
	}
}

func TestLRUReservationRepository_ClearBlockedEvents(t *testing.T) {
	ctx := context.Background()
	r, err := newLRUReservationRepositoryWithTTL(time.Minute)
	if err != nil {
		t.Fatalf("newLRUReservationRepositoryWithTTL: %v", err)
	}

	r.BlockEvent(ctx, "event-1")
	r.BlockEvent(ctx, "event-2")

	r.ClearBlockedEvents()

	if r.IsBlockEvent(ctx, "event-1") || r.IsBlockEvent(ctx, "event-2") {
		t.Fatal("expected all events to be unblocked after ClearBlockedEvents")
	}
}

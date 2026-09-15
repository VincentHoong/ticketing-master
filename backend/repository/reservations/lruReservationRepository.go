package reservations

import (
	"context"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

const lruBlockTTL = 5 * time.Second

type LRUReservationRepository struct {
	eventFullyBookedLRU *expirable.LRU[string, bool]
}

func newLRUReservationRepository() (*LRUReservationRepository, error) {
	return newLRUReservationRepositoryWithTTL(lruBlockTTL)
}

func newLRUReservationRepositoryWithTTL(ttl time.Duration) (*LRUReservationRepository, error) {
	eventFullyBookedLRU := expirable.NewLRU[string, bool](128, nil, ttl)

	r := &LRUReservationRepository{eventFullyBookedLRU: eventFullyBookedLRU}

	return r, nil
}

func (r *LRUReservationRepository) BlockEvent(ctx context.Context, eventId string) (evicted bool) {
	return r.eventFullyBookedLRU.Add(eventId, true)
}

func (r *LRUReservationRepository) UnblockEvent(ctx context.Context, eventId string) (evicted bool) {
	return r.eventFullyBookedLRU.Remove(eventId)
}

func (r *LRUReservationRepository) IsBlockEvent(ctx context.Context, eventId string) (evicted bool) {
	_, ok := r.eventFullyBookedLRU.Get(eventId)
	return ok
}

func (r *LRUReservationRepository) ClearBlockedEvents() {
	r.eventFullyBookedLRU.Purge()
}

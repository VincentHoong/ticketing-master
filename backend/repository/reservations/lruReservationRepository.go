package reservations

import (
	"context"

	lru "github.com/hashicorp/golang-lru/v2"
)

type LRUReservationRepository struct {
	eventFullyBookedLRU *lru.Cache[string, bool]
}

func newLRUReservationRepository() (*LRUReservationRepository, error) {
	eventFullyBookedLRU, err := lru.New[string, bool](128)
	if err != nil {
		return nil, err
	}

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
	return r.eventFullyBookedLRU.Contains(eventId)
}

func (r *LRUReservationRepository) ClearBlockedEvents() {
	r.eventFullyBookedLRU.Purge()
}

package reservations

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const reservationTTL = 10 * time.Minute

var ErrReservationNotFound = errors.New("reservation not found")

type ReservationStatus string

const (
	StatusHeld      ReservationStatus = "held"
	StatusConfirmed ReservationStatus = "confirmed"
	StatusReleased  ReservationStatus = "released"
	StatusExpired   ReservationStatus = "expired"
)

type ReservationItem struct {
	Id             string            `db:"id" json:"id"`
	UserId         string            `db:"user_id" json:"userId"`
	EventId        string            `db:"event_id" json:"eventId"`
	Quantity       uint32            `db:"quantity" json:"quantity"`
	Status         ReservationStatus `db:"status" json:"status"`
	IdempotencyKey *string           `db:"idempotency_key" json:"idempotencyKey,omitempty"`
	ReservedAt     *time.Time        `db:"reserved_at" json:"reservedAt"`
	ExpiresAt      *time.Time        `db:"expires_at" json:"expiresAt"`
	ConfirmedAt    *time.Time        `db:"confirmed_at" json:"confirmedAt,omitempty"`
	ReleasedAt     *time.Time        `db:"released_at" json:"releasedAt,omitempty"`
	CreatedAt      *time.Time        `db:"created_at" json:"createdAt"`
	UpdatedAt      *time.Time        `db:"updated_at" json:"updatedAt"`
}

type IReservationRepository interface {
	Close()
	GetReservation(ctx context.Context, id string) (*ReservationItem, error)
	GetReservationByIdempotencyKey(ctx context.Context, eventId string, userId string, idempotencyKey string) (*ReservationItem, error)
	SetReservationByIdempotencyKey(ctx context.Context, eventId string, userId string, idempotencyKey string, reservationItem *ReservationItem) error
	ReserveEvent(ctx context.Context, eventId string, userId string, quantity uint32, idempotencyKey *string, maxReservePerUser uint32) (*ReservationItem, bool, error)
	ConfirmReservation(ctx context.Context, userId string, reservationId string) error
	ReleaseReservation(ctx context.Context, userId string, reservationId string) error
	RefreshEventStatus(ctx context.Context) error
	GetTotalReserved(ctx context.Context, eventId string) (int64, error)
	BlockEvent(ctx context.Context, eventId string) (evicted bool)
	UnblockEvent(ctx context.Context, eventId string) (evicted bool)
	IsBlockEvent(ctx context.Context, eventId string) bool
	ClearBlockedEvents()
}

type ReservationRepository struct {
	PostgresRepository         *PostgresReservationRepository
	RemoteCacheRepository      *RemoteCacheReservationRepository
	stopRefreshEventStatusChan chan bool
	stopRefreshEventStatusOnce sync.Once
	eventFullyBookedLRU        *lru.Cache[string, bool]
}

func NewReservationRepository(dbpool *pgxpool.Pool, rdb *redis.Client) (IReservationRepository, error) {
	lruCache, err := lru.New[string, bool](128)
	if err != nil {
		return nil, err
	}
	r := &ReservationRepository{
		PostgresRepository:         newPostgresReservationRepository(dbpool),
		RemoteCacheRepository:      newRemoteCacheReservationRepository(rdb),
		stopRefreshEventStatusChan: make(chan bool),
		eventFullyBookedLRU:        lruCache,
	}
	r.startRefreshEventStatusTicker()

	return r, nil
}

func (r *ReservationRepository) Close() {
	r.stopRefreshEventStatusTicker()
}

func (r *ReservationRepository) GetReservation(ctx context.Context, id string) (*ReservationItem, error) {
	return r.PostgresRepository.GetReservation(ctx, id)
}

func (r *ReservationRepository) GetReservationByIdempotencyKey(ctx context.Context, eventId string, userId string, idempotencyKey string) (*ReservationItem, error) {
	if cached, _ := r.RemoteCacheRepository.GetReservationByIdempotencyKey(ctx, eventId, userId, idempotencyKey); cached != nil {
		return cached, nil
	}

	return r.PostgresRepository.GetReservationByIdempotencyKey(ctx, eventId, userId, idempotencyKey)
}

func (r *ReservationRepository) SetReservationByIdempotencyKey(ctx context.Context, eventId string, userId string, idempotencyKey string, reservationItem *ReservationItem) error {
	return r.RemoteCacheRepository.SetReservationByIdempotencyKey(ctx, eventId, userId, idempotencyKey, reservationItem)
}

func (r *ReservationRepository) ReserveEvent(ctx context.Context, eventId string, userId string, quantity uint32, idempotencyKey *string, maxReservePerUser uint32) (*ReservationItem, bool, error) {
	reservationItem, isCapped, err := r.PostgresRepository.ReserveEvent(ctx, eventId, userId, quantity, idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	return reservationItem, isCapped, nil
}

func (r *ReservationRepository) GetTotalReserved(ctx context.Context, eventId string) (int64, error) {
	return r.PostgresRepository.GetTotalReserved(ctx, eventId)
}

func (r *ReservationRepository) ConfirmReservation(ctx context.Context, userId string, reservationId string) error {
	_, err := r.PostgresRepository.UpdateReservation(ctx, userId, reservationId, StatusConfirmed)
	return err
}

func (r *ReservationRepository) ReleaseReservation(ctx context.Context, userId string, reservationId string) error {
	eventId, err := r.PostgresRepository.UpdateReservation(ctx, userId, reservationId, StatusReleased)
	if err != nil {
		return err
	}

	r.UnblockEvent(ctx, eventId)

	return nil
}

func (r *ReservationRepository) startRefreshEventStatusTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	log.Print("starting refresh event status ticker")

	go func() {
		defer ticker.Stop()
		defer close(r.stopRefreshEventStatusChan)
		for {
			select {
			case <-r.stopRefreshEventStatusChan:
				return
			case <-ticker.C:
				tickCtx, tickCancel := context.WithTimeout(context.Background(), 5*time.Minute)
				r.RefreshEventStatus(tickCtx)
				tickCancel()
			}
		}
	}()
}

func (r *ReservationRepository) stopRefreshEventStatusTicker() {
	r.stopRefreshEventStatusOnce.Do(func() {
		log.Print("stopping refresh event status ticker")
		r.stopRefreshEventStatusChan <- true
	})
}

func (r *ReservationRepository) RefreshEventStatus(ctx context.Context) error {
	token, err := r.RemoteCacheRepository.LockRefreshEventStatus(ctx)
	if err != nil {
		if errors.Is(err, ErrRefreshEventStatusInProgress) {
			return nil
		}
		return err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		r.RemoteCacheRepository.ReleaseRefreshEventStatus(releaseCtx, token)
	}()

	eventIds, expired, err := r.PostgresRepository.RefreshEventStatus(ctx)
	if err != nil {
		return err
	}

	for _, eventId := range eventIds {
		r.UnblockEvent(ctx, eventId)
	}
	if expired > 0 {
		log.Printf("refresh event status: expired %d holds across %d events", expired, len(eventIds))
	}

	return nil
}

func (r *ReservationRepository) BlockEvent(ctx context.Context, eventId string) (evicted bool) {
	return r.eventFullyBookedLRU.Add(eventId, true)
}

func (r *ReservationRepository) UnblockEvent(ctx context.Context, eventId string) (evicted bool) {
	return r.eventFullyBookedLRU.Remove(eventId)
}

func (r *ReservationRepository) IsBlockEvent(ctx context.Context, eventId string) bool {
	return r.eventFullyBookedLRU.Contains(eventId)
}

func (r *ReservationRepository) ClearBlockedEvents() {
	r.eventFullyBookedLRU.Purge()
}

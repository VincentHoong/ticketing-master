package events

import (
	"context"
	"errors"
	"ticketing-master/logging"
	"ticketing-master/repository/utils"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

var ErrEventNotFound = errors.New("event not found")

type EventItem struct {
	Id                string     `db:"id" json:"id"`
	Name              string     `db:"name" json:"name"`
	MaxReservePerUser uint32     `db:"max_reserve_per_user" json:"maxReservePerUser"`
	Capacity          uint32     `db:"capacity" json:"capacity"`
	StartSalesAt      *time.Time `db:"start_sales_at" json:"startSalesAt"`
	CreatedAt         *time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt         *time.Time `db:"updated_at" json:"updatedAt"`
}

type NewEvent struct {
	Name              string
	Capacity          uint32
	MaxReservePerUser uint32
	StartSalesAt      *time.Time
}

type IEventRepository interface {
	GetEvent(ctx context.Context, id string) (*EventItem, error)
	CreateEvent(ctx context.Context, event NewEvent) (*EventItem, error)
	CreateEvents(ctx context.Context, events []NewEvent) ([]EventItem, error)
	ListEvents(ctx context.Context, limit uint64) ([]EventItem, error)
	InvalidateCache(ctx context.Context, eventId string) error
}

type EventRepository struct {
	PostgresRepository    *PostgresEventRepository
	RemoteCacheRepository *RemoteCacheEventRepository
	group                 singleflight.Group
}

func NewEventRepository(dbpool *pgxpool.Pool, rdb *redis.Client) IEventRepository {
	return &EventRepository{
		PostgresRepository:    newPostgresEventRepository(dbpool),
		RemoteCacheRepository: newRemoteCacheEventRepository(rdb),
	}
}

func (e *EventRepository) CreateEvent(ctx context.Context, event NewEvent) (*EventItem, error) {
	return e.PostgresRepository.CreateEvent(ctx, event)
}

func (e *EventRepository) CreateEvents(ctx context.Context, events []NewEvent) ([]EventItem, error) {
	return e.PostgresRepository.CreateEvents(ctx, events)
}

func (e *EventRepository) ListEvents(ctx context.Context, limit uint64) ([]EventItem, error) {
	return e.PostgresRepository.ListEvents(ctx, limit)
}

func (e *EventRepository) InvalidateCache(ctx context.Context, eventId string) error {
	return e.RemoteCacheRepository.InvalidateCache(ctx, eventId)
}

func (e *EventRepository) GetEvent(ctx context.Context, id string) (*EventItem, error) {
	if event, err := e.RemoteCacheRepository.GetEvent(ctx, id); err == nil {
		return event, nil
	}

	ch := e.group.DoChan(id, func() (interface{}, error) {
		dbCtx := context.WithoutCancel(ctx)
		if deadline, ok := ctx.Deadline(); ok {
			var cancel context.CancelFunc
			dbCtx, cancel = context.WithDeadline(dbCtx, deadline)
			defer cancel()
		} else {
			return nil, utils.ErrMissingDeadline
		}

		eventItem, err := e.PostgresRepository.GetEvent(dbCtx, id)
		if err != nil {
			return nil, err
		}

		logger := logging.FromContext(ctx)
		go func() {
			cacheCtx, cancel := context.WithTimeout(logging.WithContext(context.Background(), logger), 2*time.Second)
			defer cancel()
			if err := e.RemoteCacheRepository.SetCache(cacheCtx, eventItem); err != nil {
				logger.Error("cache write failed", "event_id", id, "error", err)
			}
		}()
		return eventItem, nil
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		eventItem, ok := res.Val.(*EventItem)
		if !ok {
			return nil, utils.ErrSingleFlightValue
		}
		return eventItem, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

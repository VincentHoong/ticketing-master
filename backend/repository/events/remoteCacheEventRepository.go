package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ticketing-master/repository/utils"
	"time"

	"github.com/redis/go-redis/v9"
)

type RemoteCacheEventRepository struct {
	Rdb *redis.Client
}

func newRemoteCacheEventRepository(rdb *redis.Client) *RemoteCacheEventRepository {
	e := &RemoteCacheEventRepository{Rdb: rdb}

	return e
}

var eventIdCacheTTL = 1 * time.Minute

func eventIdCacheKey(id string) string {
	return "event:id:" + id
}

func (r *RemoteCacheEventRepository) GetEvent(ctx context.Context, id string) (*EventItem, error) {
	val, err := r.Rdb.Get(ctx, eventIdCacheKey(id)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, utils.ErrCacheMiss
	}
	if err != nil {
		return nil, fmt.Errorf("cache unavailable: %w", err)
	}
	var eventItem EventItem
	if err := json.Unmarshal([]byte(val), &eventItem); err != nil {
		return nil, utils.ErrCacheMalformed
	}
	return &eventItem, nil
}

func (r *RemoteCacheEventRepository) SetCache(ctx context.Context, event *EventItem) error {
	val, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.Rdb.Set(ctx, eventIdCacheKey(event.Id), val, utils.JitterTtl(eventIdCacheTTL)).Err()
}

func (r *RemoteCacheEventRepository) InvalidateCache(ctx context.Context, eventId string) error {
	return r.Rdb.Del(ctx, eventIdCacheKey(eventId)).Err()
}

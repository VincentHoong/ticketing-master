package users

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

var userCacheTTL = 5 * time.Minute

type RemoteCacheUserRepository struct {
	Rdb *redis.Client
}

func newRemoteCacheUserRepository(rdb *redis.Client) *RemoteCacheUserRepository {
	r := &RemoteCacheUserRepository{Rdb: rdb}

	return r
}

func userCacheKey(id string) string {
	return `user:{` + id + `}`
}

func (r *RemoteCacheUserRepository) GetUser(ctx context.Context, id string) (*UserItem, error) {
	encoded, err := r.Rdb.Get(ctx, userCacheKey(id)).Result()
	if err != nil {
		return nil, err
	}

	userItem := UserItem{}
	if err := json.Unmarshal([]byte(encoded), &userItem); err != nil {
		return nil, err
	}

	return &userItem, nil
}

func (r *RemoteCacheUserRepository) SetUser(ctx context.Context, userItem *UserItem) error {
	encoded, err := json.Marshal(userItem)
	if err != nil {
		return err
	}

	return r.Rdb.Set(ctx, userCacheKey(userItem.Id), encoded, userCacheTTL).Err()
}

package users

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
)

type RemoteCacheUserRepository struct {
	Rdb *redis.Client
}

func newRemoteCacheUserRepository(rdb *redis.Client) *RemoteCacheUserRepository {
	r := &RemoteCacheUserRepository{Rdb: rdb}

	return r
}

func (r *RemoteCacheUserRepository) GetUser(ctx context.Context, id string) (*UserItem, error) {
	return nil, errors.New("method not implemented")
}

package reservations

import (
	"context"
	"encoding/json"
	"errors"
	"ticketing-master/repository/utils"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrRefreshEventStatusInProgress = errors.New("refresh event status in progress")
var marginTTL = 5 * time.Second
var reservationItemTTL = 1 * time.Minute

type RemoteCacheReservationRepository struct {
	Rdb *redis.Client
}

func newRemoteCacheReservationRepository(rdb *redis.Client) *RemoteCacheReservationRepository {
	return &RemoteCacheReservationRepository{Rdb: rdb}
}

func getReservationByIdempotencyKey(eventId string, userId string, idempotencyKey string) string {
	return `reservation:event:{` + eventId + `}:user:{` + userId + `}:idempotency_key:{` + idempotencyKey + `}`
}

var unlockScript = redis.NewScript(`
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	else
		return 0
	end
`)

var refreshEventStatusLockKey = "reservations:refresh_event:lock"

func (r *RemoteCacheReservationRepository) LockRefreshEventStatus(ctx context.Context) (string, error) {
	token := uuid.NewString()
	remainingDeadline, err := utils.GetRemainingDeadline(ctx)
	if err != nil {
		return "", err
	}

	ok, err := r.Rdb.SetNX(ctx, refreshEventStatusLockKey, token, remainingDeadline+marginTTL).Result()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrRefreshEventStatusInProgress
	}
	return token, nil
}

func (r *RemoteCacheReservationRepository) ReleaseRefreshEventStatus(ctx context.Context, token string) error {
	return unlockScript.Run(ctx, r.Rdb, []string{refreshEventStatusLockKey}, token).Err()
}

func (r *RemoteCacheReservationRepository) SetReservationByIdempotencyKey(ctx context.Context, eventId string, userId string, idempotencyKey string, reservationItem *ReservationItem) error {
	cacheKey := getReservationByIdempotencyKey(eventId, userId, idempotencyKey)
	encoded, err := json.Marshal(reservationItem)
	if err != nil {
		return err
	}
	_, err = r.Rdb.Set(ctx, cacheKey, encoded, reservationItemTTL).Result()
	return err
}

func (r *RemoteCacheReservationRepository) GetReservationByIdempotencyKey(ctx context.Context, eventId string, userId string, idempotencyKey string) (*ReservationItem, error) {
	cacheKey := getReservationByIdempotencyKey(eventId, userId, idempotencyKey)
	encoded, err := r.Rdb.Get(ctx, cacheKey).Result()
	if err != nil {
		return nil, err
	}

	reservationItem := ReservationItem{}
	err = json.Unmarshal([]byte(encoded), &reservationItem)
	if err != nil {
		return nil, err
	}

	return &reservationItem, nil
}

package virtualqueues

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrVirtualQueueExist = errors.New("virtual queue already exist")
var ErrMissingWhitelistKey = errors.New("missing whitelist key")

type RemoteCacheVirtualQueueRepository struct {
	Rdb          *redis.Client
	WhitelistTTL time.Duration
	HeartbeatTTL time.Duration
	QueueTTL     time.Duration
}

func newRemoteCacheVirtualQueueRepository(rdb *redis.Client, whitelistTTL time.Duration, heartbeatTTL time.Duration, queueTTL time.Duration) *RemoteCacheVirtualQueueRepository {
	r := &RemoteCacheVirtualQueueRepository{
		Rdb:          rdb,
		WhitelistTTL: whitelistTTL,
		HeartbeatTTL: heartbeatTTL,
		QueueTTL:     queueTTL,
	}

	return r
}

func getEventVirtualQueueKey(eventId string) string {
	return `virtual_queue:{` + eventId + `}:event`
}

func getUserHeartbeatKey(eventId string, userId string) string {
	return `virtual_queue:{` + eventId + `}:user:` + userId
}

func getEventWhitelisKey(eventId string) string {
	return `virtual_queue:{` + eventId + `}:whitelist`
}

func getEventMaxConcurrentKey(eventId string) string {
	return `virtual_queue:{` + eventId + `}:max_concurrent`
}

func getActiveEventsKey() string {
	return `virtual_queue:active_events`
}

func (r *RemoteCacheVirtualQueueRepository) TrackActiveEvent(ctx context.Context, eventId string) error {
	return r.Rdb.SAdd(ctx, getActiveEventsKey(), eventId).Err()
}

var untrackIdleEventLuaScript = redis.NewScript(`
	local eventVirtualQueueKey = KEYS[1]
	local activeEventsKey = KEYS[2]
	local eventId = ARGV[1]

	if redis.call("ZCARD", eventVirtualQueueKey) > 0 then
		return 0
	end
	return redis.call("SREM", activeEventsKey, eventId)
`)

func (r *RemoteCacheVirtualQueueRepository) UntrackIdleEvent(ctx context.Context, eventId string) (bool, error) {
	removed, err := untrackIdleEventLuaScript.Run(
		ctx,
		r.Rdb,
		[]string{getEventVirtualQueueKey(eventId), getActiveEventsKey()},
		eventId,
	).Int64()
	if err != nil {
		return false, err
	}
	return removed > 0, nil
}

func (r *RemoteCacheVirtualQueueRepository) GetActiveEvents(ctx context.Context) ([]string, error) {
	return r.Rdb.SMembers(ctx, getActiveEventsKey()).Result()
}

func (r *RemoteCacheVirtualQueueRepository) SetEventMaxConcurrent(ctx context.Context, eventId string, maxConcurrent uint64) error {
	return r.Rdb.Set(ctx, getEventMaxConcurrentKey(eventId), maxConcurrent, 0).Err()
}

func (r *RemoteCacheVirtualQueueRepository) ClearEventMaxConcurrent(ctx context.Context, eventId string) error {
	return r.Rdb.Del(ctx, getEventMaxConcurrentKey(eventId)).Err()
}

func (r *RemoteCacheVirtualQueueRepository) GetEventMaxConcurrent(ctx context.Context, eventId string, fallback uint64) (uint64, error) {
	val, err := r.Rdb.Get(ctx, getEventMaxConcurrentKey(eventId)).Uint64()
	if errors.Is(err, redis.Nil) {
		return fallback, nil
	}
	if err != nil {
		log.Printf("unreadable max_concurrent override for event %s, using %d: %v", eventId, fallback, err)
		return fallback, nil
	}
	return val, nil
}

var enqueueLuaScript = redis.NewScript(`
	local eventVirtualQueueKey = KEYS[1]
	local userHeartbeatKey = KEYS[2]
	local currentTime = ARGV[1]
	local expiresAt = ARGV[2]
	local userId = ARGV[3]
	local heartbeatTTL = ARGV[4]

	redis.call("ZREMRANGEBYSCORE", eventVirtualQueueKey, "-inf", currentTime)
	local isSet = redis.call("SET", userHeartbeatKey, 1, "NX", "EX", heartbeatTTL)
	if isSet then
		return redis.call("ZADD", eventVirtualQueueKey, expiresAt, userId)
	else
		return -1
	end
`)

func (r *RemoteCacheVirtualQueueRepository) Enqueue(ctx context.Context, eventId string, userId string) (bool, error) {
	eventVirtualQueueKey := getEventVirtualQueueKey(eventId)
	userHeartbeatKey := getUserHeartbeatKey(eventId, userId)
	currentTime := time.Now()
	expiresAt := currentTime.Add(r.QueueTTL)
	affected, err := enqueueLuaScript.Run(
		ctx,
		r.Rdb,
		[]string{eventVirtualQueueKey, userHeartbeatKey},
		currentTime.Unix(),
		expiresAt.Unix(),
		userId,
		int64(r.HeartbeatTTL.Seconds()),
	).Int64()
	if err != nil {
		return false, err
	}

	if err := r.TrackActiveEvent(ctx, eventId); err != nil {
		log.Printf("failed to track active event %s: %v", eventId, err)
	}

	if affected < 0 {
		return false, ErrVirtualQueueExist
	}

	return affected > 0, nil
}

var dequeueLuaScript = redis.NewScript(`
	local eventVirtualQueueKey = KEYS[1]
	local eventWhitelistKey = KEYS[2]
	local userHeartbeatKey = KEYS[3]
	local userId = ARGV[1]

	local removedFromWhitelist = redis.call("HDEL", eventWhitelistKey, userId)
	local removedFromQueue = redis.call("ZREM", eventVirtualQueueKey, userId)
	redis.call("DEL", userHeartbeatKey)

	return {removedFromQueue + removedFromWhitelist, removedFromWhitelist}
`)

func (r *RemoteCacheVirtualQueueRepository) Dequeue(ctx context.Context, eventId string, userId string) (bool, bool, error) {
	eventVirtualQueueKey := getEventVirtualQueueKey(eventId)
	eventWhitelistKey := getEventWhitelisKey(eventId)
	userHeartbeatKey := getUserHeartbeatKey(eventId, userId)
	result, err := dequeueLuaScript.Run(
		ctx,
		r.Rdb,
		[]string{eventVirtualQueueKey, eventWhitelistKey, userHeartbeatKey},
		userId).Int64Slice()
	if err != nil {
		return false, false, err
	}
	if len(result) < 2 {
		return false, false, nil
	}

	return result[0] > 0, result[1] > 0, nil
}

var pingLuaScript = redis.NewScript(`
	local eventWhitelistKey = KEYS[1]
	local userHeartbeatKey = KEYS[2]
	local userId = ARGV[1]
	local heartbeatTTL = ARGV[2]

	local ttl = redis.call("HTTL", eventWhitelistKey, "FIELDS", 1, userId)
	if ttl and ttl[1] and tonumber(ttl[1]) > 0 then
		return tonumber(ttl[1])
	end

	if redis.call("EXPIRE", userHeartbeatKey, heartbeatTTL) == 1 then
		return 0
	end
	return -1
`)

func (r *RemoteCacheVirtualQueueRepository) Ping(ctx context.Context, eventId string, userId string) (time.Duration, bool, error) {
	result, err := pingLuaScript.Run(
		ctx,
		r.Rdb,
		[]string{getEventWhitelisKey(eventId), getUserHeartbeatKey(eventId, userId)},
		userId,
		int64(r.HeartbeatTTL.Seconds()),
	).Int64()
	if err != nil {
		return 0, false, err
	}
	if result < 0 {
		return 0, false, nil
	}
	return time.Duration(result) * time.Second, true, nil
}

var tryWhitelistEventQueueLuaScript = redis.NewScript(`
	local eventVirtualQueueKey = KEYS[1]
	local eventWhitelistKey = KEYS[2]
	local eventMaxConcurrentKey = KEYS[3]
	local eventId = ARGV[1]
	local desiredCount = tonumber(ARGV[2])
	local maxConcurrency = tonumber(ARGV[3])
	local whitelistTTL = ARGV[4]

	local override = redis.call("GET", eventMaxConcurrentKey)
	if override then
		maxConcurrency = tonumber(override) or maxConcurrency
	end

	local room = maxConcurrency - redis.call("HLEN", eventWhitelistKey)
	if room <= 0 then
		return 0
	end
	if desiredCount > room then
		desiredCount = room
	end

	local batchSize = desiredCount * 2
	local alive = {}
	while #alive < desiredCount do
		local candidates = redis.call("ZRANGE", eventVirtualQueueKey, 0, batchSize - 1)

		if #candidates == 0 then
			break
		end

		for _, userId in ipairs(candidates) do
			local heartbeatKey = "virtual_queue:{" .. eventId .. "}:user:" .. userId
			local whitelistKey = "virtual_queue:{" .. eventId .. "}:whitelist"

			if redis.call("EXISTS", heartbeatKey) == 1 then
				table.insert(alive, userId)
				redis.call("ZREM", eventVirtualQueueKey, userId)
				redis.call("HSET", whitelistKey, userId, "1")
				redis.call("HEXPIRE", whitelistKey, whitelistTTL, "FIELDS", 1, userId)
				redis.call("DEL", heartbeatKey)

				if #alive >= desiredCount then
					break
				end
			else
				redis.call("ZREM", eventVirtualQueueKey, userId)
			end
		end
	end
	return #alive
`)

func (r *RemoteCacheVirtualQueueRepository) TryWhitelistEventQueue(ctx context.Context, eventId string, desiredCount uint64, maxConcurrency uint64) (bool, error) {
	eventVirtualQueueKey := getEventVirtualQueueKey(eventId)
	eventWhitelisKey := getEventWhitelisKey(eventId)
	eventMaxConcurrentKey := getEventMaxConcurrentKey(eventId)
	result, err := tryWhitelistEventQueueLuaScript.Run(
		ctx,
		r.Rdb,
		[]string{eventVirtualQueueKey, eventWhitelisKey, eventMaxConcurrentKey},
		eventId,
		desiredCount,
		maxConcurrency,
		int64(r.WhitelistTTL.Seconds()),
	).Int64()
	if err != nil {
		return false, err
	}
	if result > 0 {
		return true, nil
	}
	return false, nil
}

func (r *RemoteCacheVirtualQueueRepository) GetTotalVirtualQueue(ctx context.Context, eventId string) (int64, error) {
	eventVirtualQueueKey := getEventVirtualQueueKey(eventId)
	return r.Rdb.ZCard(ctx, eventVirtualQueueKey).Result()
}

func (r *RemoteCacheVirtualQueueRepository) GetTotalEventWhitelist(ctx context.Context, eventId string) (int64, error) {
	eventWhitelistKey := getEventWhitelisKey(eventId)
	total, err := r.Rdb.HLen(ctx, eventWhitelistKey).Result()
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (r *RemoteCacheVirtualQueueRepository) GetWhitelistedUserTTL(ctx context.Context, eventId string, userId string) (time.Duration, error) {
	eventWhitelistKey := getEventWhitelisKey(eventId)
	val, err := r.Rdb.HTTL(ctx, eventWhitelistKey, userId).Result()
	if err != nil {
		return 0, err
	}
	if len(val) == 0 {
		return 0, ErrMissingWhitelistKey
	}
	return time.Duration(val[0]) * time.Second, nil
}

func (r *RemoteCacheVirtualQueueRepository) DeleteWhitelistedUserTTL(ctx context.Context, eventId string, userId string) (bool, error) {
	eventWhitelistKey := getEventWhitelisKey(eventId)
	affected, err := r.Rdb.HDel(ctx, eventWhitelistKey, userId).Result()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	return false, ErrMissingWhitelistKey
}

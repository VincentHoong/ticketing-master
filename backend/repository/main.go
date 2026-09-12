package repository

import (
	"context"
	"fmt"
	"log/slog"
	"ticketing-master/config"
	"ticketing-master/repository/events"
	"ticketing-master/repository/reservations"
	"ticketing-master/repository/users"
	"ticketing-master/repository/virtualqueues"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Repositories struct {
	EventRepository        events.IEventRepository
	ReservationRepository  reservations.IReservationRepository
	UserRepository         users.IUserRepository
	VirtualQueueRepository virtualqueues.IVirtualQueueRepository
	DbPool                 *pgxpool.Pool
	Rdb                    *redis.Client
	logger                 *slog.Logger
}

func NewRepositories(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Repositories, error) {
	dbpool, err := newPostgresConnection(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}
	rdb, err := newRedisConnection(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}
	reservationRepository, err := reservations.NewReservationRepository(dbpool, rdb, logger)
	if err != nil {
		return nil, err
	}

	return &Repositories{
		EventRepository:        events.NewEventRepository(dbpool, rdb),
		ReservationRepository:  reservationRepository,
		UserRepository:         users.NewUserRepository(dbpool, rdb),
		VirtualQueueRepository: virtualqueues.NewVirtualQueueRepository(rdb, cfg, logger),
		DbPool:                 dbpool,
		Rdb:                    rdb,
		logger:                 logger,
	}, nil
}

func (r *Repositories) Close() {
	r.VirtualQueueRepository.Close()
	r.ReservationRepository.Close()
	r.logger.Info("disconnecting postgresql")
	r.DbPool.Close()
	r.logger.Info("disconnecting redis")
	if err := r.Rdb.Close(); err != nil {
		r.logger.Error("failed to disconnect redis", "error", err)
	}
}

func newPostgresConnection(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := newPostgresConfig(cfg)
	if err != nil {
		return nil, err
	}

	dbpool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := dbpool.Ping(pingCtx); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}
	logger.Info("successfully connected to postgresql")

	return dbpool, nil
}

func newPostgresConfig(cfg *config.Config) (*pgxpool.Config, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.PostgreSQLUrl)
	if err != nil {
		return &pgxpool.Config{}, fmt.Errorf("invalid postgres url: %w", err)
	}
	poolCfg.MaxConns = 20
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = time.Minute

	return poolCfg, nil
}

func newRedisConnection(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*redis.Client, error) {
	redisCfg, err := newRedisConfig(cfg)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(redisCfg)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := rdb.Ping(pingCtx).Result(); err != nil {
		return nil, fmt.Errorf("unable to ping redis: %w", err)
	}
	logger.Info("successfully connected to redis")

	return rdb, nil
}

func newRedisConfig(cfg *config.Config) (*redis.Options, error) {
	return &redis.Options{
		Addr: cfg.RedisUrl,
	}, nil
}

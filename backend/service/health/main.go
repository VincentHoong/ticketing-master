package health

import (
	"context"
	"ticketing-master/repository"
)

type IHealthService interface {
	GetHealth(ctx context.Context) *HealthResponse
}

type HealthService struct {
	Repositories *repository.Repositories
}

func NewHealthService(repositories *repository.Repositories) IHealthService {
	return &HealthService{Repositories: repositories}
}

type HealthResponse struct {
	Postgres bool `json:"postgres"`
	Redis    bool `json:"redis"`
}

func (s *HealthService) GetHealth(ctx context.Context) *HealthResponse {
	var postgresHealth = true
	if err := s.Repositories.DbPool.Ping(ctx); err != nil {
		postgresHealth = false
	}

	var redisHealth = true
	if _, err := s.Repositories.Rdb.Ping(ctx).Result(); err != nil {
		redisHealth = false
	}

	return &HealthResponse{
		Postgres: postgresHealth,
		Redis:    redisHealth,
	}
}

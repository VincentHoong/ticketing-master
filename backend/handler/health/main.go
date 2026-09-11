package health

import (
	"ticketing-master/service/health"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	getHealthTimeout = 10 * time.Second
)

type Health struct {
	Router        *chi.Mux
	HealthService health.IHealthService
}

func NewHandler(r *chi.Mux, healthService health.IHealthService) *Health {
	h := &Health{Router: r, HealthService: healthService}

	h.getHealth()

	return h
}

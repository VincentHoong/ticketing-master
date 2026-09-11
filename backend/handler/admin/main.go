package admin

import (
	"time"

	"ticketing-master/repository"
	"ticketing-master/service"
	"ticketing-master/simulation"

	"github.com/go-chi/chi/v5"
)

type AdminHandler struct {
	Router       *chi.Mux
	Services     *service.Services
	Repositories *repository.Repositories
	Simulations  *simulation.Registry
}

const (
	mintUsersTimeout        = 60 * time.Second
	mintEventsTimeout       = 30 * time.Second
	resetTimeout            = 30 * time.Second
	simulateTimeout         = 120 * time.Second
	simulateSnapshotTimeout = 10 * time.Second
	simulateCancelTimeout   = 10 * time.Second
)

func NewHandler(router *chi.Mux, services *service.Services, repositories *repository.Repositories) *AdminHandler {
	h := AdminHandler{
		Router:       router,
		Services:     services,
		Repositories: repositories,
		Simulations:  simulation.NewRegistry(services, repositories),
	}

	h.mintUsersHandler(mintUsersTimeout)
	h.mintEventsHandler(mintEventsTimeout)
	h.resetHandler(resetTimeout)
	h.simulateHandler(simulateTimeout)
	h.simulateSnapshotHandler(simulateSnapshotTimeout)
	h.simulateCancelHandler(simulateCancelTimeout)
	h.simulateStreamHandler()

	return &h
}

package admin

import (
	"time"

	"ticketing-master/repository"
	"ticketing-master/service"

	"github.com/go-chi/chi/v5"
)

type AdminHandler struct {
	Router       *chi.Mux
	Services     *service.Services
	Repositories *repository.Repositories
}

const (
	mintUsersTimeout  = 60 * time.Second
	mintEventsTimeout = 30 * time.Second
	resetTimeout      = 30 * time.Second
)

func NewHandler(router *chi.Mux, services *service.Services, repositories *repository.Repositories) *AdminHandler {
	h := AdminHandler{Router: router, Services: services, Repositories: repositories}

	h.mintUsersHandler(mintUsersTimeout)
	h.mintEventsHandler(mintEventsTimeout)
	h.resetHandler(resetTimeout)

	return &h
}

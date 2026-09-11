package handler

import (
	"log"
	"ticketing-master/config"
	"ticketing-master/handler/admin"
	"ticketing-master/handler/events"
	"ticketing-master/handler/health"
	"ticketing-master/handler/reservations"
	"ticketing-master/handler/users"
	"ticketing-master/handler/virtualqueues"
	"ticketing-master/repository"
	"ticketing-master/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func NewHandler(cfg *config.Config, services *service.Services, repositories *repository.Repositories) *chi.Mux {
	r := chi.NewRouter()
	r.Use(
		middleware.Logger,
		middleware.Recoverer,
		cors.Handler(cors.Options{
			// AllowedOrigins:   []string{"https://foo.com"}, // Use this to allow specific origin hosts
			AllowedOrigins: []string{"https://*", "http://*"},
			// AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: false,
			MaxAge:           300, // Maximum value not ignored by any of major browsers
		}),
	)

	health.NewHandler(r, services.HealthService)
	u := users.NewHandler(r, services.UserService, cfg.JwtSecret)
	events.NewHandler(r, services.EventService)
	reservations.NewHandler(r, services.ReservationService, u)
	virtualqueues.NewHandler(r, services.VirtualQueueService, u)

	if cfg.DemoMode {
		log.Print("DEMO_MODE enabled: registering /admin routes")
		admin.NewHandler(r, services, repositories, cfg.JwtSecret, cfg.Port)
	}

	return r
}

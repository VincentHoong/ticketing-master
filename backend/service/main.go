package service

import (
	"log/slog"
	"ticketing-master/repository"
	"ticketing-master/service/events"
	"ticketing-master/service/health"
	"ticketing-master/service/reservations"
	"ticketing-master/service/users"
	"ticketing-master/service/virtualqueues"
)

type Services struct {
	EventService        events.IEventService
	ReservationService  reservations.IReservationService
	UserService         users.IUserService
	HealthService       health.IHealthService
	VirtualQueueService virtualqueues.IVirtualQueueService
}

func NewServices(repositories *repository.Repositories, logger *slog.Logger) *Services {
	return &Services{
		EventService:        events.NewEventService(repositories),
		ReservationService:  reservations.NewReservationService(repositories),
		UserService:         users.NewUserService(repositories),
		HealthService:       health.NewHealthService(repositories),
		VirtualQueueService: virtualqueues.NewVirtualQueueService(repositories, logger),
	}
}

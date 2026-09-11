package events

import (
	"context"
	"errors"
	"ticketing-master/repository"
	"ticketing-master/repository/events"
	"time"
)

const defaultEventListLimit = 100

var ErrInvalidEvent = errors.New("event name, capacity and max reserve per user are required")

type CreateEventRequest struct {
	Name              string     `json:"name"`
	Capacity          uint32     `json:"capacity"`
	MaxReservePerUser uint32     `json:"maxReservePerUser"`
	StartSalesAt      *time.Time `json:"startSalesAt,omitempty"`
}

type IEventService interface {
	GetEvent(ctx context.Context, id string) (*events.EventItem, error)
	CreateEvent(ctx context.Context, request *CreateEventRequest) (*events.EventItem, error)
	ListEvents(ctx context.Context, limit uint64) ([]events.EventItem, error)
}

type EventService struct {
	Repositories *repository.Repositories
}

func NewEventService(repositories *repository.Repositories) IEventService {
	return &EventService{Repositories: repositories}
}

func (s *EventService) GetEvent(ctx context.Context, id string) (*events.EventItem, error) {
	return s.Repositories.EventRepository.GetEvent(ctx, id)
}

func (s *EventService) CreateEvent(ctx context.Context, request *CreateEventRequest) (*events.EventItem, error) {
	if request.Name == "" || request.Capacity == 0 || request.MaxReservePerUser == 0 {
		return nil, ErrInvalidEvent
	}

	return s.Repositories.EventRepository.CreateEvent(ctx, events.NewEvent{
		Name:              request.Name,
		Capacity:          request.Capacity,
		MaxReservePerUser: request.MaxReservePerUser,
		StartSalesAt:      request.StartSalesAt,
	})
}

func (s *EventService) ListEvents(ctx context.Context, limit uint64) ([]events.EventItem, error) {
	if limit == 0 || limit > defaultEventListLimit {
		limit = defaultEventListLimit
	}

	return s.Repositories.EventRepository.ListEvents(ctx, limit)
}

package events

import (
	eventRepo "ticketing-master/repository/events"
	"ticketing-master/service/events"
	"time"

	"github.com/go-chi/chi/v5"
)

type EventDto struct {
	Id                string     `json:"id"`
	Name              string     `json:"name"`
	MaxReservePerUser uint32     `json:"maxReservePerUser"`
	Capacity          uint32     `json:"capacity"`
	CreatedAt         *time.Time `json:"createdAt"`
	UpdatedAt         *time.Time `json:"updatedAt"`
}

func (e *EventDto) toDto(eventItem *eventRepo.EventItem) *EventDto {
	return &EventDto{
		Id:                eventItem.Id,
		Name:              eventItem.Name,
		MaxReservePerUser: eventItem.MaxReservePerUser,
		Capacity:          eventItem.Capacity,
		CreatedAt:         eventItem.CreatedAt,
		UpdatedAt:         eventItem.UpdatedAt,
	}
}

type EventHandler struct {
	Router       *chi.Mux
	EventService events.IEventService
}

const (
	createEventTimeout     = 5 * time.Second
	createEventBulkTimeout = 30 * time.Second
	getEventTimeout        = 5 * time.Second
	listEventsTimeout      = 5 * time.Second
	softDeleteEventTimeout = 5 * time.Second
	updateEventTimeout     = 5 * time.Second
)

func NewHandler(router *chi.Mux, eventService events.IEventService) *EventHandler {
	h := EventHandler{Router: router, EventService: eventService}
	h.createEventHandler(createEventTimeout)
	h.createEventBulkHandler(createEventBulkTimeout)
	h.getEventHandler(getEventTimeout)
	h.listEventsHandler(listEventsTimeout)
	h.updateEventHandler(updateEventTimeout)
	h.softDeleteEventHandler(softDeleteEventTimeout)

	return &h
}

package events

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"ticketing-master/handler/utils"
	"ticketing-master/service/events"

	"github.com/go-chi/chi/v5/middleware"
)

func (h *EventHandler) createEventHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Post("/events", func(w http.ResponseWriter, r *http.Request) {
		var createEventReq = &events.CreateEventRequest{}
		if err := utils.DecodeRequestBody(w, r, createEventReq); err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}

		eventItem, err := h.EventService.CreateEvent(r.Context(), createEventReq)
		if err != nil {
			switch {
			case errors.Is(err, events.ErrInvalidEvent):
				utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			case errors.Is(err, context.Canceled):
				utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			case errors.Is(err, context.DeadlineExceeded):
				utils.WriteErrorResponse(w, http.StatusGatewayTimeout, err)
				log.Printf("request timed out: %v", err)
			default:
				utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
				log.Printf("unexpected error: %v", err)
			}
			return
		}

		eventDto := &EventDto{}
		utils.WriteJSONResponse(w, http.StatusCreated, eventDto.toDto(eventItem))
	})
}

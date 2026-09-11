package events

import (
	"context"
	"errors"
	"log"
	"net/http"
	"ticketing-master/handler/utils"
	"ticketing-master/repository/events"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (h *EventHandler) getEventHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Get("/events/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		eventItem, err := h.EventService.GetEvent(r.Context(), id)
		if err != nil {
			switch {
			case errors.Is(err, events.ErrEventNotFound):
				utils.WriteErrorResponse(w, http.StatusNotFound, err)
			case errors.Is(err, context.Canceled):
				utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			case errors.Is(err, context.DeadlineExceeded):
				utils.WriteErrorResponse(w, http.StatusGatewayTimeout, err)
				log.Printf("request timed out: %v", err)
			default:
				utils.WriteJSONResponse(w, http.StatusInternalServerError, nil)
				log.Printf("unexpected error: %v", err)
			}
			return
		}

		eventDto := &EventDto{}
		utils.WriteJSONResponse(w, http.StatusOK, eventDto.toDto(eventItem))
	})
}

package events

import (
	"context"
	"errors"
	"net/http"
	"ticketing-master/handler/utils"
	"ticketing-master/logging"
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
				utils.WriteErrorResponse(w, r, http.StatusNotFound, err)
			case errors.Is(err, context.Canceled):
				utils.WriteErrorResponse(w, r, http.StatusBadRequest, err)
			case errors.Is(err, context.DeadlineExceeded):
				utils.WriteErrorResponse(w, r, http.StatusGatewayTimeout, err)
				logging.FromContext(r.Context()).Warn("request timed out", "error", err)
			default:
				utils.WriteJSONResponse(w, r, http.StatusInternalServerError, nil)
				logging.FromContext(r.Context()).Error("unexpected error", "error", err)
			}
			return
		}

		eventDto := &EventDto{}
		utils.WriteJSONResponse(w, r, http.StatusOK, eventDto.toDto(eventItem))
	})
}

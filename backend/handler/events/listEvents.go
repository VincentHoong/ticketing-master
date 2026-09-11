package events

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"ticketing-master/handler/utils"

	"github.com/go-chi/chi/v5/middleware"
)

func (h *EventHandler) listEventsHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Get("/events", func(w http.ResponseWriter, r *http.Request) {
		var limit uint64
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				utils.WriteErrorResponse(w, http.StatusBadRequest, errors.New("limit must be a positive integer"))
				return
			}
			limit = parsed
		}

		eventItems, err := h.EventService.ListEvents(r.Context(), limit)
		if err != nil {
			switch {
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

		eventDtos := make([]*EventDto, 0, len(eventItems))
		for i := range eventItems {
			eventDto := &EventDto{}
			eventDtos = append(eventDtos, eventDto.toDto(&eventItems[i]))
		}

		utils.WriteJSONResponse(w, http.StatusOK, eventDtos)
	})
}

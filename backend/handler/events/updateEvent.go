package events

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func (h *EventHandler) updateEventHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Patch("/events/{id}", func(w http.ResponseWriter, r *http.Request) {})
}

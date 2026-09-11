package events

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func (h *EventHandler) softDeleteEventHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Delete("/events/{id}", func(w http.ResponseWriter, r *http.Request) {})
}

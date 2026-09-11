package events

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func (h *EventHandler) createEventBulkHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Post("/events/bulk", func(w http.ResponseWriter, r *http.Request) {})
}

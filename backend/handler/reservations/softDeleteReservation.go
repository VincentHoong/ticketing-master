package reservations

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func (h *ReservationHandler) softDeleteReservationHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Delete("/reservations/{id}", func(w http.ResponseWriter, r *http.Request) {})
}

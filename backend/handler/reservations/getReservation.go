package reservations

import (
	"context"
	"errors"
	"log"
	"net/http"
	"ticketing-master/handler/utils"
	appmiddleware "ticketing-master/middleware"
	"ticketing-master/repository/reservations"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (h *ReservationHandler) getReservationHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout), appmiddleware.RequireAuth(h.UserHandler)).Get("/reservations/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		reservationItem, err := h.ReservationService.GetReservation(r.Context(), id)
		if err != nil {
			switch {
			case errors.Is(err, reservations.ErrReservationNotFound):
				utils.WriteJSONResponse(w, http.StatusNotFound, nil)
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

		reservationDto := &ReservationDto{}
		utils.WriteJSONResponse(w, http.StatusOK, reservationDto.toDto(reservationItem))
	})
}

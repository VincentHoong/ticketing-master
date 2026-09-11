package reservations

import (
	"net/http"
	"time"

	"ticketing-master/handler/users"
	"ticketing-master/handler/utils"
	appmiddleware "ticketing-master/middleware"
	"ticketing-master/service/reservations"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (h *ReservationHandler) releaseReservationHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout), appmiddleware.RequireAuth(h.UserHandler)).Post("/reservations/{id}/release", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		userDto, ok := appmiddleware.UserFromContext[*users.UserDto](r)
		if !ok {
			utils.WriteJSONResponse(w, http.StatusBadRequest, nil)
			return
		}

		err := h.ReservationService.ReleaseReservation(r.Context(), &reservations.ReleaseReservationRequest{
			UserId:        userDto.Id,
			ReservationId: id,
		})
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}
		utils.WriteJSONResponse(w, http.StatusCreated, nil)
	})
}

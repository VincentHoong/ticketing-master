package reservations

import (
	"context"
	"errors"
	"net/http"
	"time"

	"ticketing-master/handler/users"
	"ticketing-master/handler/utils"
	"ticketing-master/logging"
	appmiddleware "ticketing-master/middleware"
	eventRepo "ticketing-master/repository/events"
	reservationRepo "ticketing-master/repository/reservations"
	"ticketing-master/service/reservations"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type ReserveEventRequestBody struct {
	IdempotencyKey string `json:"idempotencyKey"`
	Quantity       uint32 `json:"quantity"`
}

func (h *ReservationHandler) reserveEventHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout), appmiddleware.RequireAuth(h.UserHandler)).Post("/events/{id}/reserve", func(w http.ResponseWriter, r *http.Request) {
		eventId := chi.URLParam(r, "id")
		user, ok := appmiddleware.UserFromContext[*users.UserDto](r)
		if !ok {
			utils.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("unable to identify user"))
			return
		}

		var reserveEventReq = &ReserveEventRequestBody{}
		err := utils.DecodeRequestBody(w, r, reserveEventReq)
		if err != nil {
			utils.WriteErrorResponse(w, r, http.StatusBadRequest, err)
			return
		}

		if reserveEventReq.Quantity < 1 {
			utils.WriteJSONResponse(w, r, http.StatusBadRequest, errors.New("quantity cannot be less than 0"))
			return
		}

		var idempotencyKey *string
		if reserveEventReq.IdempotencyKey != "" {
			idempotencyKey = &reserveEventReq.IdempotencyKey
		}
		reservationItem, err := h.ReservationService.ReserveEvent(r.Context(), &reservations.ReserveEventRequest{
			EventId:        eventId,
			UserId:         user.Id,
			Quantity:       reserveEventReq.Quantity,
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			switch {
			case errors.Is(err, reservationRepo.ErrInsufficientCapacity), errors.Is(err, reservationRepo.ErrExceedMaxReserveQuantity):
				utils.WriteErrorResponse(w, r, http.StatusConflict, err)
			case errors.Is(err, eventRepo.ErrEventNotFound):
				utils.WriteErrorResponse(w, r, http.StatusNotFound, errors.New("event not found"))
			case errors.Is(err, reservations.ErrQueueInProgress):
				utils.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("not currently whitelisted for reservation"))
			case errors.Is(err, reservations.ErrIdempotencyKeyReused):
				utils.WriteErrorResponse(w, r, http.StatusForbidden, err)
			case errors.Is(err, context.Canceled):
				utils.WriteErrorResponse(w, r, http.StatusBadRequest, err)
			case errors.Is(err, context.DeadlineExceeded):
				utils.WriteErrorResponse(w, r, http.StatusGatewayTimeout, err)
				logging.FromContext(r.Context()).Warn("request timed out", "error", err)
			default:
				utils.WriteErrorResponse(w, r, http.StatusInternalServerError, err)
				logging.FromContext(r.Context()).Error("unexpected error", "error", err)
			}
			return
		}

		utils.WriteJSONResponse(w, r, http.StatusOK, reservationItem)
	})
}

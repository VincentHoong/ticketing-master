package reservations

import (
	"ticketing-master/handler/users"
	reservationRepo "ticketing-master/repository/reservations"
	"ticketing-master/service/reservations"
	"time"

	"github.com/go-chi/chi/v5"
)

type ReservationDto struct {
	Id         string     `json:"id"`
	UserId     string     `json:"userId"`
	Status     string     `json:"status"`
	EventId    string     `json:"eventId"`
	Quantity   uint32     `json:"quantity"`
	ReservedAt *time.Time `json:"reservedAt,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
}

func (r *ReservationDto) toDto(reservationItem *reservationRepo.ReservationItem) *ReservationDto {
	return &ReservationDto{
		Id:         reservationItem.Id,
		UserId:     reservationItem.UserId,
		Status:     string(reservationItem.Status),
		EventId:    reservationItem.EventId,
		Quantity:   reservationItem.Quantity,
		ReservedAt: reservationItem.ReservedAt,
		ExpiresAt:  reservationItem.ExpiresAt,
	}
}

type ReservationHandler struct {
	Router             *chi.Mux
	ReservationService reservations.IReservationService
	UserHandler        *users.UserHandler
}

const (
	reserveEventTimeout          = 5 * time.Second
	confirmReservationTimeout    = 5 * time.Second
	releaseReservationTimeout    = 5 * time.Second
	createReservationBulkTimeout = 30 * time.Second
	getReservationTimeout        = 5 * time.Second
	softDeleteReservationTimeout = 5 * time.Second
	updateReservationTimeout     = 5 * time.Second
)

func NewHandler(router *chi.Mux, reservationService reservations.IReservationService, uh *users.UserHandler) *ReservationHandler {
	h := ReservationHandler{Router: router, ReservationService: reservationService, UserHandler: uh}
	h.reserveEventHandler(reserveEventTimeout)
	h.confirmReservationHandler(confirmReservationTimeout)
	h.releaseReservationHandler(releaseReservationTimeout)
	h.getReservationHandler(getReservationTimeout)
	h.updateReservationHandler(updateReservationTimeout)
	h.softDeleteReservationHandler(softDeleteReservationTimeout)

	return &h
}

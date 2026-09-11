package virtualqueues

import (
	"ticketing-master/handler/users"
	"ticketing-master/service/virtualqueues"
	"time"

	"github.com/go-chi/chi/v5"
)

type VirtualQueueHandler struct {
	Router              *chi.Mux
	VirtualQueueService virtualqueues.IVirtualQueueService
	UserHandler         *users.UserHandler
}

const (
	pingTimeout                 = 5 * time.Second
	enqueueTimeout              = 5 * time.Second
	dequeueTimeout              = 5 * time.Second
	getNextVirtualQueueTimeout  = 5 * time.Second
	getTotalVirtualQueueTimeout = 5 * time.Second
)

func NewHandler(router *chi.Mux, virtualQueueService virtualqueues.IVirtualQueueService, uh *users.UserHandler) *VirtualQueueHandler {
	h := VirtualQueueHandler{Router: router, VirtualQueueService: virtualQueueService, UserHandler: uh}

	h.pingHandler(pingTimeout)
	h.enqueueHandler(enqueueTimeout)
	h.dequeueHandler(dequeueTimeout)
	h.getTotalVirtualQueueHandler(getTotalVirtualQueueTimeout)

	return &h
}

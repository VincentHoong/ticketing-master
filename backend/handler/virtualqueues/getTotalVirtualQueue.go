package virtualqueues

import (
	"net/http"
	"ticketing-master/handler/utils"
	appmiddleware "ticketing-master/middleware"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type GetTotalVirtualQueueResponse struct {
	Total int64 `json:"total"`
}

func (h *VirtualQueueHandler) getTotalVirtualQueueHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout), appmiddleware.RequireAuth(h.UserHandler)).Get("/virtual-queues/{eventId}/total", func(w http.ResponseWriter, r *http.Request) {
		eventId := chi.URLParam(r, "eventId")

		total, err := h.VirtualQueueService.GetTotalVirtualQueue(r.Context(), eventId)
		if err != nil {
			utils.WriteErrorResponse(w, r, http.StatusBadRequest, err)
			return
		}

		utils.WriteJSONResponse(w, r, http.StatusOK, &GetTotalVirtualQueueResponse{
			Total: total,
		})
	})
}

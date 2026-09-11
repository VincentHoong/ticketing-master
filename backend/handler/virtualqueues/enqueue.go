package virtualqueues

import (
	"errors"
	"net/http"
	"ticketing-master/handler/users"
	"ticketing-master/handler/utils"
	appmiddleware "ticketing-master/middleware"
	"ticketing-master/repository/virtualqueues"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type EnqueueRequestBody struct {
	EventId string `json:"eventId"`
}

type EnqueueResponse struct {
	Status QueueStatus `json:"status"`
}

type QueueStatus string

const (
	StatusInQueue     QueueStatus = "in queue"
	StatusWhitelisted QueueStatus = "whitelisted"
)

func (h *VirtualQueueHandler) enqueueHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout), appmiddleware.RequireAuth(h.UserHandler)).Post("/virtual-queues/enqueue", func(w http.ResponseWriter, r *http.Request) {
		userDto, ok := appmiddleware.UserFromContext[*users.UserDto](r)
		if !ok {
			utils.WriteJSONResponse(w, http.StatusBadRequest, nil)
			return
		}

		var enqueueReq = &EnqueueRequestBody{}
		err := utils.DecodeRequestBody(w, r, enqueueReq)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}

		whitelistTTL, err := h.VirtualQueueService.Enqueue(r.Context(), enqueueReq.EventId, userDto.Id)
		if err != nil {
			switch {
			case errors.Is(err, virtualqueues.ErrVirtualQueueExist):
				utils.WriteErrorResponse(w, http.StatusConflict, err)
			case errors.Is(err, virtualqueues.ErrMissingWhitelistKey):
				utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			default:
				utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			}
			return
		}
		if whitelistTTL > 0 {
			utils.WriteJSONResponse(w, http.StatusCreated, &EnqueueResponse{
				Status: StatusWhitelisted,
			})
			return
		}

		utils.WriteJSONResponse(w, http.StatusCreated, &EnqueueResponse{
			Status: StatusInQueue,
		})
	})
}

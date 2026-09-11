package virtualqueues

import (
	"net/http"
	"ticketing-master/handler/users"
	"ticketing-master/handler/utils"
	appmiddleware "ticketing-master/middleware"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type DequeueRequestBody struct {
	EventId string `json:"eventId"`
}

func (h *VirtualQueueHandler) dequeueHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout), appmiddleware.RequireAuth(h.UserHandler)).Post("/virtual-queues/dequeue", func(w http.ResponseWriter, r *http.Request) {
		userDto, ok := appmiddleware.UserFromContext[*users.UserDto](r)
		if !ok {
			utils.WriteJSONResponse(w, http.StatusBadRequest, nil)
			return
		}

		var dequeueReq = &DequeueRequestBody{}
		err := utils.DecodeRequestBody(w, r, dequeueReq)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}

		err = h.VirtualQueueService.Dequeue(r.Context(), dequeueReq.EventId, userDto.Id)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}

		utils.WriteJSONResponse(w, http.StatusCreated, nil)
	})
}

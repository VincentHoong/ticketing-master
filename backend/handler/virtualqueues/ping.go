package virtualqueues

import (
	"errors"
	"net/http"
	"ticketing-master/handler/users"
	"ticketing-master/handler/utils"
	appmiddleware "ticketing-master/middleware"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type PingRequestBody struct {
	EventId string `json:"eventId"`
	UserId  string `json:"userId"`
}

func (h *VirtualQueueHandler) pingHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout), appmiddleware.RequireAuth(h.UserHandler)).Post("/virtual-queues/ping", func(w http.ResponseWriter, r *http.Request) {
		userDto, ok := appmiddleware.UserFromContext[*users.UserDto](r)
		if !ok {
			utils.WriteJSONResponse(w, http.StatusBadRequest, nil)
			return
		}

		var pingReq = &PingRequestBody{}
		err := utils.DecodeRequestBody(w, r, pingReq)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}

		ok, err = h.VirtualQueueService.Ping(r.Context(), pingReq.EventId, userDto.Id)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}
		if !ok {
			utils.WriteErrorResponse(w, http.StatusBadRequest, errors.New("queue does not exist"))
			return
		}

		utils.WriteJSONResponse(w, http.StatusCreated, nil)
	})
}

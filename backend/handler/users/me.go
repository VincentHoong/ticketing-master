package users

import (
	"net/http"
	"time"

	"ticketing-master/handler/utils"
	appmiddleware "ticketing-master/middleware"

	"github.com/go-chi/chi/v5/middleware"
)

func (h *UserHandler) meHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout), appmiddleware.RequireAuth(h)).Get("/users/me", func(w http.ResponseWriter, r *http.Request) {
		userDto, err := appmiddleware.UserFromContext[*UserDto](r)
		if !err {
			utils.WriteJSONResponse(w, http.StatusBadRequest, nil)
			return
		}
		utils.WriteJSONResponse(w, http.StatusOK, userDto)
	})
}

package users

import (
	"context"
	"errors"
	"log"
	"net/http"
	"ticketing-master/handler/utils"
	"ticketing-master/repository/users"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (h *UserHandler) getUserHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		userItem, err := h.UserService.GetUser(r.Context(), id)
		if err != nil {
			switch {
			case errors.Is(err, users.ErrUserNotFound):
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

		userDto := &UserDto{}
		utils.WriteJSONResponse(w, http.StatusOK, userDto.toDto(userItem))
	})
}

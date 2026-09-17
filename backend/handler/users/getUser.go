package users

import (
	"context"
	"errors"
	"net/http"
	"ticketing-master/handler/utils"
	"ticketing-master/logging"
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
				utils.WriteJSONResponse(w, r, http.StatusNotFound, nil)
			case errors.Is(err, context.Canceled):
				utils.WriteErrorResponse(w, r, http.StatusBadRequest, err)
			case errors.Is(err, context.DeadlineExceeded):
				utils.WriteErrorResponse(w, r, http.StatusGatewayTimeout, err)
				logging.FromContext(r.Context()).Warn("request timed out", "error", err)
			default:
				utils.WriteJSONResponse(w, r, http.StatusInternalServerError, nil)
				logging.FromContext(r.Context()).Error("unexpected error", "error", err)
			}
			return
		}

		userDto := &UserDto{}
		utils.WriteJSONResponse(w, r, http.StatusOK, userDto.toDto(userItem))
	})
}

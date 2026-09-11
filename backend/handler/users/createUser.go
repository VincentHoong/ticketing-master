package users

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"ticketing-master/handler/utils"
	userRepo "ticketing-master/repository/users"
	"ticketing-master/service/users"

	"github.com/go-chi/chi/v5/middleware"
)

func (h *UserHandler) createUserHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Post("/users", func(w http.ResponseWriter, r *http.Request) {
		var createUserReq = &users.CreateUserRequest{}
		if err := utils.DecodeRequestBody(w, r, createUserReq); err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}

		userItem, err := h.UserService.CreateUser(r.Context(), createUserReq)
		if err != nil {
			switch {
			case errors.Is(err, users.ErrInvalidUser):
				utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			case errors.Is(err, userRepo.ErrUserEmailTaken):
				utils.WriteErrorResponse(w, http.StatusConflict, err)
			case errors.Is(err, context.Canceled):
				utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			case errors.Is(err, context.DeadlineExceeded):
				utils.WriteErrorResponse(w, http.StatusGatewayTimeout, err)
				log.Printf("request timed out: %v", err)
			default:
				utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
				log.Printf("unexpected error: %v", err)
			}
			return
		}

		userDto := &UserDto{}
		utils.WriteJSONResponse(w, http.StatusCreated, userDto.toDto(userItem))
	})
}

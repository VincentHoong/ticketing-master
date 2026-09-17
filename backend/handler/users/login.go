package users

import (
	"net/http"
	"ticketing-master/handler/utils"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v4"
)

type LoginRequestBody struct {
	Id string `json:"id"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

func (h *UserHandler) loginHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Post("/users/login", func(w http.ResponseWriter, r *http.Request) {
		var loginReq = &LoginRequestBody{}
		err := utils.DecodeRequestBody(w, r, loginReq)
		if err != nil {
			utils.WriteErrorResponse(w, r, http.StatusBadRequest, err)
			return
		}

		userDto, err := h.UserService.GetUser(r.Context(), loginReq.Id)
		if err != nil {
			utils.WriteJSONResponse(w, r, http.StatusBadRequest, nil)
			return
		}
		jwtClaims := &userClaims{
			Name:  userDto.Name,
			Email: userDto.Email,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: userDto.Id,
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
		tokenString, err := token.SignedString([]byte(h.JwtSecret))

		if err != nil {
			utils.WriteErrorResponse(w, r, http.StatusBadRequest, err)
			return
		}
		utils.WriteJSONResponse(w, r, http.StatusOK, &LoginResponse{
			Token: tokenString,
		})
	})
}

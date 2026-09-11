package users

import (
	userRepo "ticketing-master/repository/users"
	"ticketing-master/service/users"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
)

type userClaims struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	jwt.RegisteredClaims
}

type UserDto struct {
	Id        string     `json:"id"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	CreatedAt *time.Time `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt"`
}

func (u *UserDto) toDto(userItem *userRepo.UserItem) *UserDto {
	return &UserDto{
		Id:        userItem.Id,
		Name:      userItem.Name,
		Email:     userItem.Email,
		CreatedAt: userItem.CreatedAt,
		UpdatedAt: userItem.UpdatedAt,
	}
}

type UserHandler struct {
	Router      *chi.Mux
	UserService users.IUserService
	JwtSecret   string
}

const (
	createUserTimeout = 5 * time.Second
	loginTimeout      = 5 * time.Second
	meTimeout         = 5 * time.Second
)

func NewHandler(router *chi.Mux, userService users.IUserService, jwtSecret string) *UserHandler {
	h := UserHandler{Router: router, UserService: userService, JwtSecret: jwtSecret}

	h.createUserHandler(createUserTimeout)
	h.loginHandler(loginTimeout)
	h.meHandler(meTimeout)

	return &h
}

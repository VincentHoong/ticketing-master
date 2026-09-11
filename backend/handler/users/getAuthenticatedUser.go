package users

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"
)

var (
	ErrMissingBearerToken = errors.New("missing or invalid bearer token")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

const prefix = "Bearer "

func (h *UserHandler) GetAuthenticatedUser(r *http.Request) (*UserDto, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return &UserDto{}, ErrMissingBearerToken
	}

	if !strings.HasPrefix(header, prefix) {
		return &UserDto{}, ErrMissingBearerToken
	}

	tokenString := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if tokenString == "" {
		return &UserDto{}, ErrMissingBearerToken
	}

	claims := &userClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(h.JwtSecret), nil
	})
	if err != nil || !token.Valid {
		return &UserDto{}, ErrInvalidToken
	}

	userItem, err := h.UserService.GetUser(r.Context(), claims.Subject)
	if err != nil {
		return &UserDto{}, err
	}
	userDto := &UserDto{}

	return userDto.toDto(userItem), nil
}

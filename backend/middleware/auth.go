package middleware

import (
	"context"
	"net/http"
	"ticketing-master/handler/utils"
)

type ctxKey string

const userCtxKey ctxKey = "user"

type Authenticator[T any] interface {
	GetAuthenticatedUser(r *http.Request) (T, error)
}

func RequireAuth[T any](a Authenticator[T]) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := a.GetAuthenticatedUser(r)
			if err != nil {
				utils.WriteJSONResponse(w, http.StatusUnauthorized, err)
				return
			}
			ctx := context.WithValue(r.Context(), userCtxKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Return user stored in the context where,
// T = user dto,
// bool = is in context
func UserFromContext[T any](r *http.Request) (T, bool) {
	u, ok := r.Context().Value(userCtxKey).(T)
	return u, ok
}

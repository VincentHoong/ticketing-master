package middleware

import (
	"context"
	"net/http"
	"ticketing-master/handler/utils"
	"ticketing-master/logging"
)

type ctxKey string

const userCtxKey ctxKey = "user"

type Authenticator[T any] interface {
	GetAuthenticatedUser(r *http.Request) (T, error)
}

type identifiable interface {
	AuthID() string
}

func RequireAuth[T any](a Authenticator[T]) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := a.GetAuthenticatedUser(r)
			if err != nil {
				utils.WriteJSONResponse(w, r, http.StatusUnauthorized, err)
				return
			}
			ctx := context.WithValue(r.Context(), userCtxKey, user)
			if id, ok := any(user).(identifiable); ok {
				ctx = logging.WithContext(ctx, logging.FromContext(ctx).With("user_id", id.AuthID()))
			}
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

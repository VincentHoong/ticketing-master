package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"ticketing-master/logging"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func RequestLogger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			reqLogger := base.With(
				"request_id", chimiddleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
			)
			r = r.WithContext(logging.WithContext(r.Context(), reqLogger))

			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			level := slog.LevelInfo
			if r.URL.Path == "/health" {
				level = slog.LevelDebug
			}
			reqLogger.Log(r.Context(), level, "http request",
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

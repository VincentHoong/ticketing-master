package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"ticketing-master/handler/utils"
	"ticketing-master/logging"
	"ticketing-master/simulation"

	"github.com/go-chi/chi/v5/middleware"
)

type ResetScope string

const (
	ScopeReservations ResetScope = "reservations"
	// ScopeUsers also clears users, which every simulation mints fresh on start and never
	// reuses. Without it the only way to stop them accumulating is ScopeAll, which also
	// destroys the events the demo was set up around.
	ScopeUsers ResetScope = "users"
	ScopeAll   ResetScope = "all"
)

func (s ResetScope) valid() bool {
	return s == ScopeReservations || s == ScopeUsers || s == ScopeAll
}

type ResetRequest struct {
	Scope ResetScope `json:"scope"`
}

type ResetResponse struct {
	Scope             ResetScope `json:"scope"`
	ReservationsCount int64      `json:"reservationsCleared"`
	EventsCount       int64      `json:"eventsCleared"`
	UsersCount        int64      `json:"usersCleared"`
	RedisFlushed      bool       `json:"redisFlushed"`
}

func (h *AdminHandler) resetHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Post("/admin/reset", func(w http.ResponseWriter, r *http.Request) {
		var req = &ResetRequest{}
		if err := utils.DecodeRequestBody(w, r, req); err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}
		if req.Scope == "" {
			req.Scope = ScopeReservations
		}
		if !req.Scope.valid() {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Errorf("scope must be %q, %q or %q", ScopeReservations, ScopeUsers, ScopeAll))
			return
		}

		// Reject before touching anything: truncating under a run in flight corrupts the
		// numbers it is measuring and can crash it mid-transaction. The UI disables this
		// button while a run is active, but curl doesn't know that.
		if h.Simulations.HasAnyActiveRun() {
			utils.WriteErrorResponse(w, http.StatusConflict, simulation.ErrRunInProgress)
			return
		}

		response, err := h.reset(r.Context(), req.Scope)
		if err != nil {
			switch {
			case errors.Is(err, context.Canceled):
				utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			case errors.Is(err, context.DeadlineExceeded):
				utils.WriteErrorResponse(w, http.StatusGatewayTimeout, err)
				logging.FromContext(r.Context()).Warn("request timed out", "error", err)
			default:
				utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
				logging.FromContext(r.Context()).Error("reset failed", "error", err)
			}
			return
		}

		utils.WriteJSONResponse(w, http.StatusOK, response)
	})
}

func (h *AdminHandler) reset(ctx context.Context, scope ResetScope) (*ResetResponse, error) {
	response := &ResetResponse{Scope: scope}

	tx, err := h.Repositories.DbPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := tx.QueryRow(ctx, `SELECT count(*) FROM reservations`).Scan(&response.ReservationsCount); err != nil {
		return nil, err
	}

	if scope == ScopeUsers || scope == ScopeAll {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&response.UsersCount); err != nil {
			return nil, err
		}
	}

	switch scope {
	case ScopeAll:
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM events`).Scan(&response.EventsCount); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `TRUNCATE reservations, events, users RESTART IDENTITY CASCADE`); err != nil {
			return nil, err
		}
	case ScopeUsers:
		// Naming reservations explicitly rather than relying on the cascade, so the
		// statement says what it truncates. Events are untouched.
		if _, err := tx.Exec(ctx, `TRUNCATE reservations, users RESTART IDENTITY CASCADE`); err != nil {
			return nil, err
		}
	default:
		if _, err := tx.Exec(ctx, `TRUNCATE reservations RESTART IDENTITY`); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	h.Repositories.ReservationRepository.ClearBlockedEvents()

	if err := h.Repositories.Rdb.FlushDB(ctx).Err(); err != nil {
		logging.FromContext(ctx).Error("reset: flush redis", "error", err)
		return response, nil
	}
	response.RedisFlushed = true

	return response, nil
}

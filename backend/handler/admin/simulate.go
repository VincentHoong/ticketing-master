package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"ticketing-master/handler/utils"
	"ticketing-master/logging"
	userRepo "ticketing-master/repository/users"
	"ticketing-master/simulation"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

const (
	maxSimulatedUsers = 20000
	// Each HTTP user holds a real socket, so the ceiling here is file descriptors rather
	// than goroutines. Well under the usual 10240 soft limit, leaving room for the pools.
	maxHTTPSimulatedUsers = 4000
	streamInterval        = 250 * time.Millisecond
)

type SimulateRequest struct {
	EventId       string `json:"eventId"`
	Users         int    `json:"users"`
	Quantity      uint32 `json:"quantity"`
	MaxConcurrent uint64 `json:"maxConcurrent"`
	Capacity      uint32 `json:"capacity"`
	Workers       int    `json:"workers"`
	PollMs        int64  `json:"pollMs"`
	UserTimeoutMs int64  `json:"userTimeoutMs"`
	Reset         bool   `json:"reset"`
	Transport     string `json:"transport"`
}

type SimulateResponse struct {
	RunId   string `json:"runId"`
	EventId string `json:"eventId"`
	Users   int    `json:"users"`
}

func (h *AdminHandler) simulateHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Post("/admin/simulate", func(w http.ResponseWriter, r *http.Request) {
		var req = &SimulateRequest{}
		if err := utils.DecodeRequestBody(w, r, req); err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}
		if req.EventId == "" {
			utils.WriteErrorResponse(w, http.StatusBadRequest, errors.New("eventId is required"))
			return
		}
		if req.Users <= 0 || req.Users > maxSimulatedUsers {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Errorf("users must be between 1 and %d", maxSimulatedUsers))
			return
		}

		transport := simulation.Transport(req.Transport)
		if transport == "" {
			transport = simulation.TransportInProcess
		}
		if transport != simulation.TransportInProcess && transport != simulation.TransportHTTP {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Errorf("transport must be %q or %q", simulation.TransportInProcess, simulation.TransportHTTP))
			return
		}
		if transport == simulation.TransportHTTP && req.Users > maxHTTPSimulatedUsers {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Errorf("http transport supports at most %d users, got %d", maxHTTPSimulatedUsers, req.Users))
			return
		}

		// Reject before touching anything: the capacity update, the truncate and the
		// Redis flush below would otherwise destroy the state of the run already in flight.
		if h.Simulations.HasActiveRun(req.EventId) {
			utils.WriteErrorResponse(w, http.StatusConflict, simulation.ErrRunInProgress)
			return
		}

		if req.Capacity > 0 {
			if _, err := h.Repositories.DbPool.Exec(r.Context(),
				`UPDATE events SET capacity = $1 WHERE id = $2`, req.Capacity, req.EventId); err != nil {
				utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
				return
			}
			if err := h.Repositories.EventRepository.InvalidateCache(r.Context(), req.EventId); err != nil {
				utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
				return
			}
		}

		if req.Reset {
			if _, err := h.Repositories.DbPool.Exec(r.Context(),
				`DELETE FROM reservations WHERE event_id = $1`, req.EventId); err != nil {
				utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
				return
			}
			if err := h.Repositories.Rdb.FlushDB(r.Context()).Err(); err != nil {
				utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
				return
			}
			// The sold-out markers live in this process, so clearing the rows without
			// clearing them leaves every later run short-circuiting on an event that is
			// now empty.
			h.Repositories.ReservationRepository.ClearBlockedEvents()
		}

		// Raising capacity means a previously full event has seats again.
		if req.Capacity > 0 {
			h.Repositories.ReservationRepository.UnblockEvent(r.Context(), req.EventId)
		}

		userIds, err := h.mintSimUsers(r, req.Users)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
			logging.FromContext(r.Context()).Error("simulate: mint users failed", "error", err)
			return
		}

		var tokens map[string]string
		if transport == simulation.TransportHTTP {
			if tokens, err = h.mintTokens(userIds); err != nil {
				utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
				return
			}
		}

		run, err := h.Simulations.Start(simulation.Config{
			EventId:       req.EventId,
			UserIds:       userIds,
			Quantity:      req.Quantity,
			MaxConcurrent: req.MaxConcurrent,
			Workers:       req.Workers,
			PollInterval:  time.Duration(req.PollMs) * time.Millisecond,
			UserTimeout:   time.Duration(req.UserTimeoutMs) * time.Millisecond,
			Transport:     transport,
			BaseURL:       h.BaseURL,
			Tokens:        tokens,
		})
		if err != nil {
			if errors.Is(err, simulation.ErrRunInProgress) {
				utils.WriteErrorResponse(w, http.StatusConflict, err)
				return
			}
			utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
			return
		}

		utils.WriteJSONResponse(w, http.StatusAccepted, &SimulateResponse{
			RunId:   run.Id,
			EventId: run.EventId,
			Users:   len(userIds),
		})
	})
}

func (h *AdminHandler) simulateSnapshotHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Get("/admin/simulate/{runId}", func(w http.ResponseWriter, r *http.Request) {
		run, err := h.Simulations.Get(chi.URLParam(r, "runId"))
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusNotFound, err)
			return
		}

		utils.WriteJSONResponse(w, http.StatusOK, run.Runner.Snapshot(r.Context()))
	})
}

func (h *AdminHandler) simulateCancelHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Post("/admin/simulate/{runId}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if err := h.Simulations.Cancel(chi.URLParam(r, "runId")); err != nil {
			utils.WriteErrorResponse(w, http.StatusNotFound, err)
			return
		}

		utils.WriteJSONResponse(w, http.StatusAccepted, nil)
	})
}

func (h *AdminHandler) simulateStreamHandler() {
	h.Router.Get("/admin/simulate/{runId}/stream", func(w http.ResponseWriter, r *http.Request) {
		run, err := h.Simulations.Get(chi.URLParam(r, "runId"))
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusNotFound, err)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			utils.WriteErrorResponse(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
			return
		}
		rc := http.NewResponseController(w)
		if err := rc.SetWriteDeadline(time.Time{}); err != nil {
			utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		send := func() bool {
			snapshot := run.Runner.Snapshot(r.Context())
			payload, err := json.Marshal(snapshot)
			if err != nil {
				return false
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return false
			}
			flusher.Flush()
			return snapshot.Done
		}

		if done := send(); done {
			return
		}

		ticker := time.NewTicker(streamInterval)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if done := send(); done {
					return
				}
			}
		}
	})
}

func (h *AdminHandler) mintSimUsers(r *http.Request, count int) ([]string, error) {
	newUsers := make([]userRepo.NewUser, 0, count)
	for i := 0; i < count; i++ {
		newUsers = append(newUsers, userRepo.NewUser{
			Name:         fmt.Sprintf("Sim User %d", i+1),
			Email:        fmt.Sprintf("sim+%s@example.com", uuid.NewString()),
			PasswordHash: mintedHash,
		})
	}

	return h.Repositories.UserRepository.CreateUsers(r.Context(), newUsers)
}

// mintTokens issues the bearer token each simulated user authenticates with, using the
// same claims and secret as /users/login so the run exercises the real auth path.
func (h *AdminHandler) mintTokens(userIds []string) (map[string]string, error) {
	tokens := make(map[string]string, len(userIds))
	secret := []byte(h.JwtSecret)

	for _, userId := range userIds {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{Subject: userId})
		signed, err := token.SignedString(secret)
		if err != nil {
			return nil, err
		}
		tokens[userId] = signed
	}

	return tokens, nil
}

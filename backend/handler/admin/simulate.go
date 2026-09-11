package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"ticketing-master/handler/utils"
	userRepo "ticketing-master/repository/users"
	"ticketing-master/simulation"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const (
	maxSimulatedUsers = 20000
	streamInterval    = 250 * time.Millisecond
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
		}

		userIds, err := h.mintSimUsers(r, req.Users)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
			log.Printf("simulate: mint users: %v", err)
			return
		}

		run, err := h.Simulations.Start(simulation.Config{
			EventId:       req.EventId,
			UserIds:       userIds,
			Quantity:      req.Quantity,
			MaxConcurrent: req.MaxConcurrent,
			Workers:       req.Workers,
			PollInterval:  time.Duration(req.PollMs) * time.Millisecond,
			UserTimeout:   time.Duration(req.UserTimeoutMs) * time.Millisecond,
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

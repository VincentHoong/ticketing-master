package admin

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"ticketing-master/handler/utils"
	"ticketing-master/logging"
	eventRepo "ticketing-master/repository/events"
	userRepo "ticketing-master/repository/users"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const (
	maxMintUsers  = 20000
	maxMintEvents = 500
	mintedHash    = "$2a$10$GdGI14Uzy6WzBWBjujbZD.W5VI8K51Ur3lrRpdfknbzsjeTZD/Wjq"
)

var mintedFirstNames = []string{"Ava", "Marcus", "Priya", "Diego", "Grace", "Noor", "Kenji", "Lucia", "Omar", "Freya"}
var mintedLastNames = []string{"Thompson", "Lee", "Sharma", "Fernandez", "Kim", "Haddad", "Tanaka", "Rossi", "Farouk", "Berg"}
var mintedVenues = []string{"Arena", "Stadium", "Amphitheatre", "Hall", "Garden", "Dome"}
var mintedActs = []string{"Neon Tide", "Paper Kites", "Midnight Circuit", "Velvet Static", "Echo Harbour", "Glass Animals"}

type MintUsersRequest struct {
	Count uint64 `json:"count"`
}

type MintUsersResponse struct {
	Requested uint64   `json:"requested"`
	Created   int      `json:"created"`
	UserIds   []string `json:"userIds"`
}

type MintEventsRequest struct {
	Count             uint64 `json:"count"`
	Capacity          uint32 `json:"capacity"`
	MaxReservePerUser uint32 `json:"maxReservePerUser"`
}

type MintEventsResponse struct {
	Requested uint64                `json:"requested"`
	Created   int                   `json:"created"`
	Events    []eventRepo.EventItem `json:"events"`
}

func (h *AdminHandler) mintUsersHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Post("/admin/mint/users", func(w http.ResponseWriter, r *http.Request) {
		var req = &MintUsersRequest{}
		if err := utils.DecodeRequestBody(w, r, req); err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}
		if req.Count == 0 || req.Count > maxMintUsers {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Errorf("count must be between 1 and %d", maxMintUsers))
			return
		}

		newUsers := make([]userRepo.NewUser, 0, req.Count)
		for i := uint64(0); i < req.Count; i++ {
			name := fmt.Sprintf("%s %s",
				mintedFirstNames[rand.Intn(len(mintedFirstNames))],
				mintedLastNames[rand.Intn(len(mintedLastNames))],
			)
			newUsers = append(newUsers, userRepo.NewUser{
				Name:         name,
				Email:        fmt.Sprintf("minted+%s@example.com", uuid.NewString()),
				PasswordHash: mintedHash,
			})
		}

		userIds, err := h.Repositories.UserRepository.CreateUsers(r.Context(), newUsers)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
			logging.FromContext(r.Context()).Error("mint users failed", "error", err)
			return
		}

		utils.WriteJSONResponse(w, http.StatusCreated, &MintUsersResponse{
			Requested: req.Count,
			Created:   len(userIds),
			UserIds:   userIds,
		})
	})
}

func (h *AdminHandler) mintEventsHandler(requestTimeout time.Duration) {
	h.Router.With(middleware.Timeout(requestTimeout)).Post("/admin/mint/events", func(w http.ResponseWriter, r *http.Request) {
		var req = &MintEventsRequest{}
		if err := utils.DecodeRequestBody(w, r, req); err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err)
			return
		}
		if req.Count == 0 || req.Count > maxMintEvents {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Errorf("count must be between 1 and %d", maxMintEvents))
			return
		}

		startSalesAt := time.Now().Add(-1 * time.Minute)
		newEvents := make([]eventRepo.NewEvent, 0, req.Count)
		for i := uint64(0); i < req.Count; i++ {
			capacity := req.Capacity
			if capacity == 0 {
				capacity = uint32(50 + rand.Intn(450))
			}
			maxReserve := req.MaxReservePerUser
			if maxReserve == 0 {
				maxReserve = uint32(1 + rand.Intn(6))
			}
			name := fmt.Sprintf("%s at the %s",
				mintedActs[rand.Intn(len(mintedActs))],
				mintedVenues[rand.Intn(len(mintedVenues))],
			)
			newEvents = append(newEvents, eventRepo.NewEvent{
				Name:              name,
				Capacity:          capacity,
				MaxReservePerUser: maxReserve,
				StartSalesAt:      &startSalesAt,
			})
		}

		created, err := h.Repositories.EventRepository.CreateEvents(r.Context(), newEvents)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusInternalServerError, err)
			logging.FromContext(r.Context()).Error("mint events failed", "error", err)
			return
		}
		if created == nil {
			created = []eventRepo.EventItem{}
		}

		utils.WriteJSONResponse(w, http.StatusCreated, &MintEventsResponse{
			Requested: req.Count,
			Created:   len(created),
			Events:    created,
		})
	})
}

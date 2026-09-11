package health

import (
	"net/http"
	"ticketing-master/handler/utils"

	"github.com/go-chi/chi/v5/middleware"
)

type HealthResponse struct {
	Postgres bool `json:"postgres"`
}

func (h *Health) getHealth() {
	h.Router.With(middleware.Timeout(getHealthTimeout)).Get("/health", func(w http.ResponseWriter, r *http.Request) {
		healthResponse := h.HealthService.GetHealth(r.Context())
		status := http.StatusOK
		if !healthResponse.Postgres || !healthResponse.Redis {
			status = http.StatusServiceUnavailable
		}

		utils.WriteJSONResponse(w, status, healthResponse)
	})
}

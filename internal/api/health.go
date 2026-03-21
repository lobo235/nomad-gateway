package api

import (
	"net/http"
)

type healthResponse struct {
	Status string `json:"status"`
}

// healthHandler returns a handler for GET /health.
// It pings Nomad and returns 200 if reachable, 503 otherwise.
// This endpoint intentionally skips Bearer auth — it is called by Nomad's health check.
func (s *Server) healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.nomad.Ping(); err != nil {
			s.log.Error("nomad ping failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	}
}

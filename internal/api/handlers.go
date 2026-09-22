package api

import (
	"net/http"

	"github.com/DaviRodrigues/opspulse/internal/checker"
)

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	targets, err := s.targetLoader.Load()
	if err != nil {
		sendError(w,
			http.StatusInternalServerError,
			"falha ao carregar targets configurados",
		)
		return
	}

	sendJSON(w,
		http.StatusOK,
		checker.CheckAll(r.Context(), targets),
	)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, http.StatusOK, map[string]string{
		"status": "OK",
	})
}

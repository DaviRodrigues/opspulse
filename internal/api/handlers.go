package api

import (
	"net/http"

	"github.com/DaviRodrigues/opspulse/internal/checker"
)

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	sendJSON(w,
		http.StatusOK,
		checker.CheckAll(r.Context(), 
		s.monitorConfig.TargetURLs),
	)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, http.StatusOK, map[string]string{
		"status": "OK",
	})
}

package api

import (
	"net/http"
	"os"

	"github.com/DaviRodrigues/opspulse/internal/checker"
)

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	sendJSON(w,
		http.StatusOK,
		checker.CheckAll(r.Context(), s.monitorConfig.TargetURLs),
	)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, http.StatusOK, map[string]string{
		"status": "OK",
	})
}

func (s *Server) handleHTMLBroker(w http.ResponseWriter, r *http.Request) {
	paths := []string{"web/index.html", "./web/index.html", "../web/index.html"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			http.ServeFile(w, r, p)
			return
		}
	}

	http.Error(w, "Dashboard HTML não encontrado na pasta web/", http.StatusNotFound)
}


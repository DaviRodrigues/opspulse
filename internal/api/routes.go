package api

import (
	"net/http"

	"github.com/DaviRodrigues/opspulse/internal/domain"
	"github.com/go-chi/chi/v5"
)

func targetRoutes(r chi.Router, s *Server, t []domain.TargetResult) {
	r.Route("/targets", func(r chi.Router) {
		r.Get("/status", func(w http.ResponseWriter, r *http.Request) {
			// Mais pra frente remover a dependência de target e repassar ao banco
			GetStatus(w, r, t)
		})

		r.Get("/events", func(w http.ResponseWriter, r *http.Request) {
			HandleSSE(w, r, s.Broker)
		})
	})
}

package api

import (
	"github.com/go-chi/chi/v5"
)

func (s *Server) registerRoutes() {
	s.router.Get("/health", s.handleHealth)

	s.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/status", s.getStatus)
	})
}

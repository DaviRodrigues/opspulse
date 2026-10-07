package api

import (
	"github.com/go-chi/chi/v5"
)

func targetRoutes(r chi.Router, targetHandler TargetHandler) {
	r.Route("/targets", func(r chi.Router) {
		r.Get("/status", targetHandler.GetStatus)

		r.Get("/events", targetHandler.HandleSSE)
	})
}

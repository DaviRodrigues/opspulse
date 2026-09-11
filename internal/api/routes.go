package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func apiV1(r *chi.Mux) {
	r.Route("/api/v1", func(r chi.Router) {

	})
}

func apiIsOk(r *chi.Mux) {
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"OK"}`))
	})
}

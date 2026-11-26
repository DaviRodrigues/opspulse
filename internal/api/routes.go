package api

import (
	"github.com/go-chi/chi/v5"
)

func targetRoutes(r chi.Router, targetHandler TargetHandler) {
	r.Get("/targets", targetHandler.ListTargets)
	r.Post("/targets", targetHandler.CreateTarget)
	r.Get("/targets/status", targetHandler.GetStatus)
	r.Get("/targets/events", targetHandler.HandleSSE)
	r.Get("/targets/{id}", targetHandler.GetTarget)
	r.Put("/targets/{id}", targetHandler.UpdateTarget)
	r.Patch("/targets/{id}", targetHandler.PatchTarget)
	r.Delete("/targets/{id}", targetHandler.DeleteTarget)
}

func checkLogRoutes(r chi.Router, checkLogHandler CheckLogHandler) {
	r.Get("/check-logs", checkLogHandler.ListCheckLogs)
	r.Get("/check-logs/{id}", checkLogHandler.GetCheckLog)
}

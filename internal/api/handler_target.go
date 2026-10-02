package api

import (
	"net/http"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/domain"
)

func GetStatus(w http.ResponseWriter, r *http.Request, targets []domain.Target) {
	SendJSON(w,
		http.StatusOK,
		checker.CheckAll(r.Context(), targets),
	)
}

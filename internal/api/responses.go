package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/DaviRodrigues/opspulse/internal/domain"
)

type ErrorResponse struct {
	Error string `json:"error" validate:"required"`
}

type HealthResponse struct {
	Status string `json:"status" validate:"required"`
}

type PaginationMeta struct {
	Total  int64 `json:"total" validate:"required"`
	Limit  int64 `json:"limit" validate:"required"`
	Offset int64 `json:"offset" validate:"required"`
}

type PaginatedTargetsResponse struct {
	Data       []*domain.Target `json:"data" validate:"required"`
	Pagination PaginationMeta   `json:"pagination" validate:"required"`
}

type PaginatedCheckLogsResponse struct {
	Data       []domain.CheckLog `json:"data" validate:"required"`
	Pagination PaginationMeta    `json:"pagination" validate:"required"`
}

func SendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func SendError(w http.ResponseWriter, status int, message string) {
	SendJSON(w, status, ErrorResponse{
		Error: message,
	})
}

func PrepareSSE(w http.ResponseWriter) (http.Flusher, bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return flusher, ok
	}

	return flusher, ok
}

func FormatEvent(e domain.Event) ([]byte, error) {
	var sb strings.Builder

	if e.ID != "" {
		sb.WriteString(fmt.Sprintf("id: %s\n", e.ID))
	}

	if e.Name != "" {
		sb.WriteString(fmt.Sprintf("event: %s\n", e.Name))
	}

	if e.Retry > 0 {
		sb.WriteString(fmt.Sprintf("retry: %d\n", e.Retry))
	}

	if e.Data != nil {
		jsonData, err := json.Marshal(e.Data)
		if err != nil {
			return nil, err
		}
		sb.WriteString(fmt.Sprintf("data: %s\n", string(jsonData)))
	}

	sb.WriteString("\n")

	return []byte(sb.String()), nil
}

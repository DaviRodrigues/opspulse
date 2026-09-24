package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func sendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, map[string]string{
		"error": message,
	})
}

func prepareSSE(w http.ResponseWriter) (http.Flusher, bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return flusher, ok
	}

	return flusher, ok
}

func formatEvent(e Event) ([]byte, error) {
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

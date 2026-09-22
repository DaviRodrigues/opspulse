package api

import (
	"encoding/json"
	"net/http"
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

package api

import (
	"log/slog"
	"net/http"
	"time"

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

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := prepareSSE(w)
	if !ok {
		slog.Error("Streaming not supported")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	clientChan := make(chan Event, 10)
	s.broker.Register(clientChan)
	defer s.broker.UnRegister(clientChan)

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-clientChan:
			data, err := formatEvent(event)
			if err != nil {
				continue
			}
			slog.Debug("Event ", "data", event)
			w.Write(data)
			flusher.Flush()
		case <-heartbeat.C:
			w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}

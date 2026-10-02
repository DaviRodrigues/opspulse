package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/domain"
)

func HandleSSE(w http.ResponseWriter, r *http.Request, broker *EventBroker) {
	flusher, ok := PrepareSSE(w)
	if !ok {
		slog.Error("Streaming not supported")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	clientChan := make(chan domain.Event, 10)
	broker.Register(clientChan)
	defer broker.UnRegister(clientChan)

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-clientChan:
			data, err := FormatEvent(event)
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

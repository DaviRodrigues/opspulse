package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/domain"
)

// TODO: coloca repository aqui depois
type TargetHandler struct {
	broker  *EventBroker
	targets []domain.TargetResult // apenas por enquanto, retirar quanto tiver banco
}

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	SendJSON(w, http.StatusOK, map[string]string{
		"status": "OK",
	})
}

func (handler *TargetHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := PrepareSSE(w)
	if !ok {
		slog.Error("Streaming not supported")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	clientChan := make(chan domain.Event, 10)
	handler.broker.Register(clientChan)
	defer handler.broker.UnRegister(clientChan)

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

func (handler *TargetHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	SendJSON(w,
		http.StatusOK,
		checker.CheckAll(r.Context(), handler.targets),
	)
}

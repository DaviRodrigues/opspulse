package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	slogchi "github.com/samber/slog-chi"
)

type Server struct {
	router        *chi.Mux
	managerHttp   *http.Server
	monitorConfig config.MonitorConfig
	broker        *EventBroker
}

func NewServer(monitorConfig config.MonitorConfig, port string) *Server {
	r := chi.NewRouter()
	server := &Server{
		router:        r,
		monitorConfig: monitorConfig,
		managerHttp: &http.Server{
			Addr:         ":" + port,
			Handler:      r,
			ReadTimeout:  time.Second * 5,
			WriteTimeout: 0,
		},
		broker: NewCheckBroker(),
	}

	return server
}

func (s *Server) SetConfigures(loggerManager *slog.Logger) {
	s.router.Use(slogchi.New(loggerManager))
	s.router.Use(middleware.RequestID)                 // Injects unique ID into request context
	s.router.Use(middleware.RealIP)                    // Captures actual client IP
	s.router.Use(middleware.Logger)                    // Clean, structured request logging
	s.router.Use(middleware.Recoverer)                 // Recovers from panics without crashing server
	s.router.Use(middleware.Timeout(60 * time.Second)) // Automatic request timeout

	s.registerRoutes()
}

func (s *Server) Setup(ctx context.Context) error {
	go func() {
		slog.Info("Starting API server on :3333...")
		if err := s.managerHttp.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("Shutdown signal received, starting graceful teardown...")

	timeoutCtx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second)
	defer cancel()

	if err := s.managerHttp.Shutdown(timeoutCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		return err
	}

	slog.Info("Server gracefully stopped.")

	return nil
}

func (s *Server) StartMonitoring(ctx context.Context, cfg config.MonitorConfig) {
	ticker := time.NewTicker(time.Second*30)
	defer ticker.Stop()

	event := NewStatusEvent(checker.CheckAll(ctx, cfg.TargetURLs))
	s.broker.Publish(event)

	for {
		select {
		case <-ctx.Done():
			slog.Info("🛑 Encerrando monitoramento de forma segura")
			return
		case <-ticker.C:
			event := NewStatusEvent(checker.CheckAll(ctx, cfg.TargetURLs))
			s.broker.Publish(event)
		}
	}
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
			slog.Info("Event ", "data", event)
			w.Write(data)
			flusher.Flush()
		case <-heartbeat.C:
			w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}

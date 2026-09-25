package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
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

func NewServer(ctx context.Context, monitorConfig config.MonitorConfig, port string) *Server {
	r := chi.NewRouter()
	server := &Server{
		router:        r,
		monitorConfig: monitorConfig,
		managerHttp: &http.Server{
			Addr:         ":" + port,
			Handler:      r,
			ReadTimeout:  time.Second * 5,
			WriteTimeout: 0,
			BaseContext: func(l net.Listener) context.Context {
				return ctx
			},
		},
		broker: NewCheckBroker(),
	}

	return server
}

func (s *Server) SetConfigures(loggerManager *slog.Logger) {
	s.router.Use(slogchi.New(loggerManager))
	s.router.Use(middleware.RequestID) // Injects unique ID into request context
	s.router.Use(middleware.RealIP)    // Captures actual client IP
	s.router.Use(middleware.Logger)    // Clean, structured request logging
	s.router.Use(middleware.Recoverer) // Recovers from panics without crashing server

	s.registerRoutes()
}

func (s *Server) Setup(ctx context.Context, monitorCfg config.MonitorConfig) error {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Starting API server on :3333...")
		if err := s.managerHttp.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.StartMonitoring(ctx, monitorCfg)
	}()

	<-ctx.Done()
	slog.Info("Shutdown signal received, starting graceful teardown...")

	timeoutCtx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second)
	defer cancel()

	err := s.managerHttp.Shutdown(timeoutCtx)
	if err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		return err
	}

	wg.Wait()
	slog.Info("Server gracefully stopped.")
	return nil
}

func (s *Server) StartMonitoring(ctx context.Context, cfg config.MonitorConfig) {
	ticker := time.NewTicker(time.Second * 30)
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

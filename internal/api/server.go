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
	router      *chi.Mux
	managerHttp *http.Server
	config config.Config
	broker      *EventBroker
}

func NewServer(ctx context.Context, geralConfig config.Config) *Server {
	r := chi.NewRouter()
	return &Server{
		router:        r,
		config: geralConfig,
		managerHttp: &http.Server{
			Addr:         ":" + geralConfig.Server.Port,
			Handler:      r,
			ReadTimeout:  geralConfig.Server.ReadTimeout,
			WriteTimeout: geralConfig.Server.WriteTimeout,
			BaseContext: func(l net.Listener) context.Context {
				return ctx
			},
		},
		broker: NewCheckBroker(),
	}
}

func (s *Server) SetConfigures(loggerManager *slog.Logger) {
	s.router.Use(slogchi.New(loggerManager))
	s.router.Use(middleware.RequestID) // Injects unique ID into request context
	s.router.Use(middleware.RealIP)    // Captures actual client IP
	s.router.Use(middleware.Logger)    // Clean, structured request logging
	s.router.Use(middleware.Recoverer) // Recovers from panics without crashing server

	s.registerRoutes()
}

func (s *Server) Setup(ctx context.Context) error {
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
		s.StartMonitoring(ctx)
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

func (s *Server) StartMonitoring(ctx context.Context) {
	ticker := time.NewTicker(s.config.Monitor.Interval)
	defer ticker.Stop()

	event := NewStatusEvent(checker.CheckAll(ctx, s.config.Monitor.TargetURLs))
	s.broker.Publish(event)

	for {
		select {
		case <-ctx.Done():
			slog.Info("🛑 Encerrando monitoramento de forma segura")
			return
		case <-ticker.C:
			event := NewStatusEvent(checker.CheckAll(ctx, s.config.Monitor.TargetURLs))
			s.broker.Publish(event)
		}
	}
}

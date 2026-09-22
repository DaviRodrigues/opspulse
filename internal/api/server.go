package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	slogchi "github.com/samber/slog-chi"
)

type Server struct {
	router       *chi.Mux
	managerHttp  *http.Server // posso criar depois um setup pro manager
	targetLoader file.TargetLoader
}

func NewServer(loader file.TargetLoader, port string, loggerManager *slog.Logger) *Server {
	r := chi.NewRouter()
	server := Server{
		router: r,
		managerHttp: &http.Server{
			Addr:         ":" + port,
			Handler:      r,
			ReadTimeout:  time.Second * 5,
			WriteTimeout: time.Second * 5,
		},
	}

	// Global Built-in Middlewares
	server.router.Use(slogchi.New(loggerManager))
	server.router.Use(middleware.RequestID)                 // Injects unique ID into request context
	server.router.Use(middleware.RealIP)                    // Captures actual client IP
	server.router.Use(middleware.Logger)                    // Clean, structured request logging
	server.router.Use(middleware.Recoverer)                 // Recovers from panics without crashing server
	server.router.Use(middleware.Timeout(60 * time.Second)) // Automatic request timeout

	apiIsOk(server.router)
	apiV1(server.router)

	return &server
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

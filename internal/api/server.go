package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/contextG"
	"github.com/DaviRodrigues/opspulse/internal/file"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
)

type Server struct {
	router       *chi.Mux
	managerHttp  *http.Server // posso criar depois um setup pro manager
	targetLoader file.TargetLoader
}

func Setup() error {
	// TODO não esquecer do target depois
	r := chi.NewRouter()
	server := Server{
		router: r,
		managerHttp: &http.Server{
			Addr:         ":3333",
			Handler:      r,
			ReadTimeout:  time.Second * 5,
			WriteTimeout: time.Second * 5,
		},
	}

	// Global Built-in Middlewares
	// TODO MELHORAR ESSA BOMBA DEPOIS TA COM MEIO MUNDO DE CONTEXTO NO SETUP MANE
	server.router.Use(middleware.RequestID)                 // Injects unique ID into request context
	server.router.Use(middleware.RealIP)                    // Captures actual client IP
	server.router.Use(middleware.Logger)                    // Clean, structured request logging
	server.router.Use(middleware.Recoverer)                 // Recovers from panics without crashing server
	server.router.Use(middleware.Timeout(60 * time.Second)) // Automatic request timeout

	apiIsOk(server.router)
	apiV1(server.router)


	// TODO mandar pra parte de contexto isso aqui, no condition filhão
	go func() {
		slog.Info("Starting API server on :3333...")
		if err := server.managerHttp.ListenAndServe(); err != nil {
			slog.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	shutdownCtx, stop := contextG.CreateNotifyContext()
	defer stop()
	<-shutdownCtx.Done()
	slog.Info("Shutdown signal received, starting graceful teardown...")

	timeoutCtx, cancel := contextG.CreateContextTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.managerHttp.Shutdown(timeoutCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		return err
	}

	slog.Info("Server gracefully stopped.")

	return nil
}

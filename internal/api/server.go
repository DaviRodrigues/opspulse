package api

import (
	"context"
	"errors"
	"fmt"
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
	serverConfig  config.ServerConfig
	monitorConfig config.MonitorConfig
	broker        *EventBroker
}

func NewServer(ctx context.Context, serverCfg config.ServerConfig, monitorCfg config.MonitorConfig) *Server {
	r := chi.NewRouter()
	fmt.Printf("WRITE_TIMEOUT %v READ_TIMEOUT %v", serverCfg.WriteTimeout, serverCfg.ReadTimeout)
	return &Server{
		router:        r,
		serverConfig:  serverCfg,
		monitorConfig: monitorCfg,
		managerHttp: &http.Server{
			Addr:    ":" + serverCfg.Port,
			Handler: r,
			BaseContext: func(l net.Listener) context.Context {
				return ctx
			},
			ReadTimeout:  serverCfg.ReadTimeout,
			WriteTimeout: serverCfg.WriteTimeout, // 0 para SSE streaming contínuo
		},
		broker: NewCheckBroker(),
	}
}

func (s *Server) SetConfigures(loggerManager *slog.Logger) {
	s.router.Use(slogchi.New(loggerManager))
	s.router.Use(middleware.RequestID) // Injeta ID único para rastreamento de requests
	s.router.Use(middleware.RealIP)    // Captura o IP real do cliente
	s.router.Use(middleware.Logger)    // Log de requisições estruturado
	s.router.Use(middleware.Recoverer) // Recupera de panics sem derrubar a API

	s.registerRoutes()
}

func (s *Server) Setup(ctx context.Context) error {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Starting API server...", "port", s.serverConfig.Port)
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

	shutdownTimeout := s.serverConfig.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 15 * time.Second
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
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
	interval := s.monitorConfig.Interval
	fmt.Printf("INTERVAL %v", interval)
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	event := NewStatusEvent(checker.CheckAll(ctx, s.monitorConfig.TargetURLs))
	s.broker.Publish(event)

	for {
		select {
		case <-ctx.Done():
			slog.Info("🛑 Encerrando monitoramento da API de forma segura")
			return
		case <-ticker.C:
			event := NewStatusEvent(checker.CheckAll(ctx, s.monitorConfig.TargetURLs))
			s.broker.Publish(event)
		}
	}
}

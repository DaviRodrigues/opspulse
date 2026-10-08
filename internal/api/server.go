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
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	slogchi "github.com/samber/slog-chi"
)

const scalarHTML = `<!doctype html>
<html>
  <head>
    <title>OpsPulse - API Documentation</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style>
      body { margin: 0; }
    </style>
  </head>
  <body>
    <script
      id="api-reference"
      data-url="/docs/openapi.yaml"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`

type Server struct {
	Router       *chi.Mux
	managerHttp  *http.Server
	serverConfig config.ServerConfig
	Broker       *EventBroker
	MongoClient  *database.MongoClient
}

func NewServer(
	ctx context.Context,
	mongoClient *database.MongoClient,
	serverCfg config.ServerConfig,
	monitorCfg config.MonitorConfig,
) (*Server, error) {
	r := chi.NewRouter()

	return &Server{
		Router: r,
		managerHttp: &http.Server{
			Addr:    ":" + serverCfg.Port,
			Handler: r,
			BaseContext: func(l net.Listener) context.Context {
				return ctx
			},
			ReadTimeout:  serverCfg.ReadTimeout,
			WriteTimeout: serverCfg.WriteTimeout, // 0 para SSE streaming contínuo
		},
		serverConfig: serverCfg,
		Broker:       NewCheckBroker(),
		MongoClient:  mongoClient,
	}, nil
}

func (s *Server) SetConfigures(loggerManager *slog.Logger, monitorConfig config.MonitorConfig) {
	s.Router.Use(slogchi.New(loggerManager))
	s.Router.Use(middleware.RequestID) // Injeta ID único para rastreamento de requests
	s.Router.Use(middleware.RealIP)    // Captura o IP real do cliente
	s.Router.Use(middleware.Logger)    // Log de requisições estruturado
	s.Router.Use(middleware.Recoverer) // Recupera de panics sem derrubar a API
	s.Router.Use(corsMiddleware)

	s.registerRoutes(monitorConfig)
}

func (s *Server) Setup(ctx context.Context, monitorConfig config.MonitorConfig) error {
	var wg sync.WaitGroup

	wg.Go(func() {
		slog.Info("Starting API server...", "port", s.serverConfig.Port)
		if err := s.managerHttp.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	})

	wg.Go(func() {
		s.StartMonitoring(ctx, monitorConfig)
	})

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

	if err := s.MongoClient.Close(ctx); err != nil {
		slog.Error("Failed to close connection with Database", "error", err)
		return err
	}

	return nil
}
/* TODO:
Tirar essa parte do server e passar para o checker, utilizar um padrão de projeto strategy, talvez com
adapter pra abstrair ainda mais as operações. A questão toda é lidar com os cases do channel, provavel
que dará um trabalho então ATENÇÃO!
*/
func (s *Server) StartMonitoring(ctx context.Context, monitorConfig config.MonitorConfig) {
	interval := monitorConfig.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	event := NewStatusEvent(checker.CheckAll(ctx, monitorConfig.TargetURLs))
	s.Broker.Publish(event)

	for {
		select {
		case <-ctx.Done():
			slog.Info("🛑 Encerrando monitoramento da API de forma segura")
			return
		case <-ticker.C:
			event := NewStatusEvent(checker.CheckAll(ctx, monitorConfig.TargetURLs))
			s.Broker.Publish(event)
		}
	}
}

func (s *Server) registerRoutes(monitorConfig config.MonitorConfig) {
	s.Router.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", HandleHealth)

		targetRoutes(r, TargetHandler{
			broker:  s.Broker,
			targets: monitorConfig.TargetURLs,
		})
	})

	s.Router.Get("/docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "docs/openapi.yaml")
	})

	s.Router.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(scalarHTML))
	})
}

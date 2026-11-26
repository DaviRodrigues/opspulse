package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
	Router        *chi.Mux
	managerHttp   *http.Server
	serverConfig  config.ServerConfig
	Broker        *EventBroker
	MongoClient   *database.MongoClient
	targetRepo    database.TargetRepository
	checkLogRepo  database.CheckLogRepository
	reconcileChan chan struct{}
}

func NewServer(
	ctx context.Context,
	mongoClient *database.MongoClient,
	serverCfg config.ServerConfig,
	monitorCfg config.MonitorConfig,
) (*Server, error) {
	var targetRepo database.TargetRepository
	var checkLogRepo database.CheckLogRepository
	if mongoClient != nil && mongoClient.Database() != nil {
		db := mongoClient.Database()
		targetRepo = database.NewMongoDBTargetRepository(db)
		checkLogRepo = database.NewMongoDBCheckLogRepository(db)
	}

	return NewServerWithRepos(ctx, mongoClient, targetRepo, checkLogRepo, serverCfg, monitorCfg)
}

func NewServerWithRepos(
	ctx context.Context,
	mongoClient *database.MongoClient,
	targetRepo database.TargetRepository,
	checkLogRepo database.CheckLogRepository,
	serverCfg config.ServerConfig,
	monitorCfg config.MonitorConfig,
) (*Server, error) {
	r := chi.NewRouter()
	readHeaderTimeout := serverCfg.ReadTimeout
	if readHeaderTimeout <= 0 {
		readHeaderTimeout = 5 * time.Second
	}

	return &Server{
		Router: r,
		managerHttp: &http.Server{
			Addr:    ":" + serverCfg.Port,
			Handler: r,
			BaseContext: func(l net.Listener) context.Context {
				return ctx
			},
			ReadTimeout:       serverCfg.ReadTimeout,
			ReadHeaderTimeout: readHeaderTimeout,
			WriteTimeout:      serverCfg.WriteTimeout, // 0 para SSE streaming contínuo
		},
		serverConfig:  serverCfg,
		Broker:        NewCheckBroker(),
		MongoClient:   mongoClient,
		targetRepo:    targetRepo,
		checkLogRepo:  checkLogRepo,
		reconcileChan: make(chan struct{}, 1),
	}, nil
}

func (s *Server) SetConfigures(loggerManager *slog.Logger, monitorConfig config.MonitorConfig) {
	s.Router.Use(middleware.RequestID) // Injeta ID único para rastreamento de requests
	s.Router.Use(middleware.RealIP)    // Captura o IP real do cliente
	if loggerManager != nil {
		s.Router.Use(slogchi.New(loggerManager))
	} else {
		s.Router.Use(middleware.Logger) // Log de requisições estruturado (fallback)
	}
	s.Router.Use(middleware.Recoverer)    // Recupera de panics sem derrubar a API
	s.Router.Use(middleware.StripSlashes) // Normaliza trailing slash em rotas REST
	s.Router.Use(corsMiddleware(s.serverConfig.CORSAllowedOrigins))

	s.registerRoutes(monitorConfig)
}

func (s *Server) Setup(ctx context.Context, monitorConfig config.MonitorConfig) error {
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	serveErr := make(chan error, 1)
	var wg sync.WaitGroup

	wg.Go(func() {
		slog.Info("Starting API server...", "port", s.serverConfig.Port)
		if err := s.managerHttp.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed to start", "error", err)
			serveErr <- err
			runCancel()
		}
	})

	wg.Go(func() {
		s.StartMonitoring(runCtx, monitorConfig)
	})

	var startErr error
	select {
	case <-ctx.Done():
		slog.Info("Shutdown signal received, starting graceful teardown...")
	case startErr = <-serveErr:
		slog.Error("API server aborted due to listen error", "error", startErr)
	}

	runCancel()
	if s.Broker != nil {
		s.Broker.Close()
	}

	shutdownTimeout := s.serverConfig.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 15 * time.Second
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := s.managerHttp.Shutdown(timeoutCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		_ = s.managerHttp.Close()
		wg.Wait()
		return err
	}

	wg.Wait()
	slog.Info("Server gracefully stopped.")

	return startErr
}

func (s *Server) signalReconcile() {
	if s.reconcileChan == nil {
		return
	}
	select {
	case s.reconcileChan <- struct{}{}:
	default:
	}
}

func (s *Server) runMonitorCycle(ctx context.Context, defaultTimeout time.Duration) {
	if s.targetRepo == nil {
		return
	}

	targetPtrs, err := s.targetRepo.FindEnabled(ctx)
	if err != nil {
		slog.Error("Falha ao buscar alvos habilitados na API", "error", err)
		return
	}

	targets := make([]domain.Target, 0, len(targetPtrs))
	for _, t := range targetPtrs {
		if t != nil {
			targets = append(targets, *t)
		}
	}

	results, logs := checker.CheckTargets(ctx, targets, defaultTimeout)
	if s.checkLogRepo != nil && len(logs) > 0 {
		if err := s.checkLogRepo.CreateBatch(ctx, logs); err != nil {
			slog.Error("Falha ao persistir lote de check_logs na API", "error", err)
		}
	}

	if s.Broker != nil {
		s.Broker.Publish(NewStatusEvent(results))
		for _, res := range results {
			if !res.IsUp {
				s.Broker.Publish(NewAlertEvent(res))
			}
		}
	}
}

func (s *Server) StartMonitoring(ctx context.Context, monitorConfig config.MonitorConfig) {
	checker.RunTargetScheduler(ctx, s.targetRepo, s.checkLogRepo, s.reconcileChan, monitorConfig, checker.SchedulerHooks{
		OnResult: func(res domain.CheckResult, snapshot []domain.CheckResult) {
			if s.Broker == nil {
				return
			}
			s.Broker.Publish(NewStatusEvent(snapshot))
			if !res.IsUp {
				s.Broker.Publish(NewAlertEvent(res))
			}
		},
		OnSnapshot: func(snapshot []domain.CheckResult) {
			if s.Broker == nil {
				return
			}
			s.Broker.Publish(NewStatusEvent(snapshot))
		},
	})
	slog.Info("🛑 Encerrando monitoramento da API de forma segura")
}

func (s *Server) registerRoutes(monitorConfig config.MonitorConfig) {
	s.Router.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", HandleHealth)

		targetRoutes(r, TargetHandler{
			broker:         s.Broker,
			targetRepo:     s.targetRepo,
			checkLogRepo:   s.checkLogRepo,
			defaultTimeout: monitorConfig.DefaultTimeout,
			onTargetChange: s.signalReconcile,
		})

		checkLogRoutes(r, CheckLogHandler{
			checkLogRepo: s.checkLogRepo,
		})
	})

	s.Router.Get("/docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		http.ServeFile(w, r, "docs/openapi.yaml")
	})

	s.Router.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(scalarHTML))
	})
}

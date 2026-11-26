package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/api"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/file"
	"github.com/DaviRodrigues/opspulse/internal/logger"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	envFile := file.NewEnvFile()
	cfg, err := config.Load(
		config.API,
		envFile,
	)
	if err != nil {
		slog.Error("Falha crítica ao carregar configurações", "error", err)
		return err
	}

	loggerManager, err := logger.InitLogger(cfg.App, cfg.Log)
	if err != nil {
		slog.Error("Não foi possível iniciar o log", "error", err)
		return err
	}

	mongoClient, err := database.NewMongoClient(ctx, cfg.Database)
	if err != nil {
		slog.Error("Não foi possível instanciar a conexão com banco", "error", err)
		return err
	}
	defer func() {
		closeTimeout := cfg.Database.Timeout
		if closeTimeout <= 0 {
			closeTimeout = 5 * time.Second
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		if err := mongoClient.Close(shutdownCtx); err != nil {
			slog.Error("Falha ao encerrar conexão com banco de dados", "error", err)
		}
	}()

	db := mongoClient.Database()
	targetRepo := database.NewMongoDBTargetRepository(db)
	checkLogRepo := database.NewMongoDBCheckLogRepository(db)

	initTimeout := cfg.Database.Timeout
	if initTimeout <= 0 {
		initTimeout = 5 * time.Second
	}
	initCtx, initCancel := context.WithTimeout(ctx, initTimeout)
	if err := targetRepo.EnsureIndexes(initCtx); err != nil {
		slog.Warn("Não foi possível garantir índices de targets na inicialização", "error", err)
	}
	if err := database.EnsureCheckLogsCollection(initCtx, db); err != nil {
		slog.Warn("Não foi possível garantir coleção time-series check_logs na inicialização", "error", err)
	}
	initCancel()

	server, err := api.NewServerWithRepos(ctx, mongoClient, targetRepo, checkLogRepo, cfg.Server, cfg.Monitor)
	if err != nil {
		slog.Error("Não foi possível instanciar o servidor", "error", err)
		return err
	}
	server.SetConfigures(loggerManager, cfg.Monitor)

	if err = server.Setup(ctx, cfg.Monitor); err != nil {
		slog.Error("Falha na execução do servidor", "error", err)
		return err
	}

	return nil
}

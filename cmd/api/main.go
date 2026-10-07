package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/DaviRodrigues/opspulse/internal/api"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/file"
	"github.com/DaviRodrigues/opspulse/internal/logger"
)

func main() {
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
		os.Exit(1)
	}

	loggerManager, err := logger.InitLogger(cfg.App, cfg.Log)
	if err != nil {
		slog.Error("Não foi possível iniciar o log", "error", err)
		os.Exit(1)
	}

	mongoClient, err := database.NewMongoClient(ctx, cfg.Database)
	if err != nil {
		slog.Error("Não foi possível inistanciar a conexão com banco", "error", err)
		os.Exit(1)
	}
	defer mongoClient.Close(ctx)

	server, err := api.NewServer(ctx, mongoClient, cfg.Server, cfg.Monitor)
	if err != nil {
		slog.Error("Não foi possível inistanciar o servidor", "error", err)
		os.Exit(1)
	}
	server.SetConfigures(loggerManager, cfg.Monitor)

	if err = server.Setup(ctx, cfg.Monitor); err != nil {
		slog.Error("Falha na execução do servidor", "error", err)
		os.Exit(1)
	}
}

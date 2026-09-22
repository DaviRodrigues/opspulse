package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/DaviRodrigues/opspulse/internal/api"
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

	handler, err := logger.HandlerDefaultText(slog.LevelDebug, "./log/api")
	if err != nil {
		slog.Error("Não foi possível carregar o handler do log", "error", err)
		os.Exit(1)
	}

	loggerManager, err := logger.SetupSlog(handler)
	if err != nil {
		slog.Error("Não foi possível iniciar o log", "error", err)
		os.Exit(1)
	}

	targetLoader := &file.JSONFile{
		FileDefault: file.FileDefault{Name: "target.json", Path: "./target"},
	}
	server := api.NewServer(targetLoader, "3333", loggerManager)
	if err = server.Setup(ctx); err != nil {
		slog.Error("Falha na execução do servidor", "error", err)
		os.Exit(1)
	}
}

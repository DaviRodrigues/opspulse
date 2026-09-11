package main

import (
	"log/slog"
	"os"

	"github.com/DaviRodrigues/opspulse/internal/api"
	"github.com/DaviRodrigues/opspulse/internal/logger"
)

func main() {
	handler, err := logger.HandlerDefaultText(slog.LevelDebug, "./log")
	if err != nil {
		slog.Error("Não foi possível carregar o handler do log", "error", err)
		os.Exit(1)
	}

	err = logger.SetupSlog(handler)
	if err != nil {
		slog.Error("Não foi possível iniciar o log", "error", err)
		os.Exit(1)
	}

	err = api.Setup()
	if err != nil {
		os.Exit(1)
	}
}

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/discord"
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

	cfg, err := config.Load(
		config.APP,
		&file.JSONFile{},
		file.NewEnvFile(),
	)
	if err != nil {
		slog.Error("Falha crítica ao carregar configurações", "error", err)
		os.Exit(1)
	}

	_, err = logger.InitLogger(cfg.App, cfg.Log)
	if err != nil {
		slog.Error("Não foi possível iniciar o log", "error", err)
		os.Exit(1)
	}

	var notifier checker.Notifier
	var triggerChan chan struct{}

	bot, err := discord.New(&cfg.Discord)
	if err != nil {
		slog.Warn("Não foi possível iniciar o bot do Discord, continuando apenas com monitor local", "error", err)
	} else {
		defer bot.Close()
		triggerChan, err = bot.Setup(ctx, cfg.Monitor)
		if err != nil {
			slog.Error("Falha crítica ao setar configurações do bot", "error", err)
			os.Exit(1)
		}
		notifier = bot
	}

	checker.StartMonitoring(ctx, notifier, triggerChan, cfg.Monitor)
}


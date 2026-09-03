package main

import (
	"log/slog"
	"os"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/context"
	"github.com/DaviRodrigues/opspulse/internal/logger"
	// "github.com/DaviRodrigues/opspulse/internal/discord"
)

func main() {
	handler, err := logger.HandlerDefaultText(slog.LevelInfo, "./log")
	if err != nil {
		slog.Error("Não foi possível carregar o handler do log", "error", err)
		os.Exit(1)
	}

	err = logger.SetupSlog(handler)
	if err != nil {
		slog.Error("Não foi possível iniciar o log", "error", err)
		os.Exit(1)
	}

	ctx, stop := context.CreateNotifyContext()
	defer stop()

	cfg, err := config.Load("./.env.test")
	if err != nil {
		slog.Error("Falha crítica ao carregar configurações", "error", err)
		os.Exit(1)
	}

//	bot, err := discord.New(cfg.DiscordToken, cfg.DiscordChannelID)
//	if err != nil {
//		slog.Warn("Falha ao iniciar bot", "error", err)
//	}
//	defer bot.Close()

	checker.StartMonitoring(ctx, nil, cfg.TargetURLs, cfg.CheckInterval, cfg.CheckTimeout)
}

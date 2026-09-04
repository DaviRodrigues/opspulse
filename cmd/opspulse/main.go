package main

import (
	"log/slog"
	"os"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/context"
	"github.com/DaviRodrigues/opspulse/internal/logger"
	"github.com/DaviRodrigues/opspulse/internal/discord"
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

	bot, err := discord.New(cfg.Discord.Token, cfg.Discord.ChannelID)
	if err != nil {
		slog.Warn("Falha ao iniciar bot", "error", err)
	}
	defer bot.Close()

	triggerChan := make(chan struct{}, 1)
	bot.RegisterHandlers(func() []checker.CheckResult {
		results := checker.CheckAll(ctx, cfg.Monitor.TargetURLs, cfg.Monitor.Timeout)
		
		select {
		case triggerChan <-struct{}{}:
		default:
		}

		return results
	})

	checker.StartMonitoring(ctx, bot, triggerChan, cfg)
}

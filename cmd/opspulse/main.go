package main

import (
	"log/slog"
	"os"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/context"
	"github.com/DaviRodrigues/opspulse/internal/discord"
	"github.com/DaviRodrigues/opspulse/internal/logger"
)

// TODO preciso depois testar a integração disso de forma manual (remova o .env.test NÃO ESQUECER)

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

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Falha crítica ao carregar configurações", "error", err)
		os.Exit(1)
	}

	var bot *discord.Bot
	triggerChan := make(chan struct{}, 1)
	bot, err = discord.New(cfg.Discord.Token, cfg.Discord.ChannelID)
	if err != nil {
		slog.Warn("Não foi possível iniciar o bot do Discord, continuando apenas com monitor local", "error", err)
	} else {
		defer bot.Close()
		bot.RegisterHandlers(func() []checker.CheckResult {
			results := checker.CheckAll(ctx, cfg.Monitor.TargetURLs, cfg.Monitor.Timeout)

			select {
			case triggerChan <- struct{}{}:
			default:
			}
			return results
		})
	}

	checker.StartMonitoring(ctx, bot, triggerChan, cfg)
}

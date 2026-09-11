package main

import (
	"log/slog"
	"os"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/contextG"
	"github.com/DaviRodrigues/opspulse/internal/discord"
	"github.com/DaviRodrigues/opspulse/internal/file"
	"github.com/DaviRodrigues/opspulse/internal/logger"
)

// TODO preciso depois testar a integração disso de forma manual (remova o .env.test NÃO ESQUECER)

func main() {
	ctx, stop := contextG.CreateNotifyContext()
	defer stop()

	// TODO depois vou precisar perguntar ao usuário qual arquivo ele quer carregar antes de continuar
	cfg, err := config.Load(&file.JSONFile{
		FileDefault: file.FileDefault{
			Name: "target.json",
			Path: "./target",
		},
	})
	if err != nil {
		slog.Error("Falha crítica ao carregar configurações", "error", err)
		os.Exit(1)
	}

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

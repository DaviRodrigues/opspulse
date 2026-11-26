package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
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

	mongoClient, err := database.NewMongoClient(ctx, cfg.Database)
	if err != nil {
		slog.Error("Não foi possível instanciar a conexão com banco", "error", err)
		os.Exit(1)
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

	var notifier checker.Notifier
	var triggerChan chan struct{}

	bot, err := discord.New(&cfg.Discord)
	if err != nil {
		slog.Warn("Não foi possível iniciar o bot do Discord, continuando apenas com monitor local", "error", err)
	} else {
		defer bot.Close()
		triggerChan, err = bot.Setup(ctx, targetRepo, checkLogRepo, cfg.Monitor.DefaultTimeout)
		if err != nil {
			slog.Error("Falha crítica ao setar configurações do bot", "error", err)
			os.Exit(1)
		}
		notifier = bot
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		checker.StartMonitoring(ctx, targetRepo, checkLogRepo, notifier, triggerChan, cfg.Monitor)
	})

	<-ctx.Done()
	wg.Wait()
}

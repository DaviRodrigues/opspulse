package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/file"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	_ "github.com/golang-migrate/migrate/v4/source/file"
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

	mongoClient, err := database.NewMongoClient(ctx, cfg.Database)
	if err != nil {
		slog.Error("Falha crítica iniciar conexão com banco de dados", "error", err)
		os.Exit(1)
	}

	slog.Info("Conexão com banco bem sucedida", "connect", mongoClient)

	m, err := migrate.New(
		"file://migrations",
		fmt.Sprintf("%s/%s?authSource=admin", cfg.Database.URI, cfg.Database.Name),
	)
	if err != nil {
		slog.Error("Falha ao inicializar migrador", "error", err)
		os.Exit(1)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		slog.Error("Falha ao executar migrações", "error", err)
		os.Exit(1)
	}

	slog.Info("Migrações aplicadas com sucesso (ou banco já atualizado)!")
}

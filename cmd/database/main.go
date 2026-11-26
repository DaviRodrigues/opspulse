package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/file"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
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
		return err
	}

	mongoClient, err := database.NewMongoClient(ctx, cfg.Database)
	if err != nil {
		slog.Error("Falha crítica iniciar conexão com banco de dados", "error", err)
		return err
	}
	defer func() {
		closeTimeout := cfg.Database.Timeout
		if closeTimeout <= 0 {
			closeTimeout = 5 * time.Second
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		if err := mongoClient.Close(closeCtx); err != nil {
			slog.Error("Falha ao encerrar conexão com banco de dados", "error", err)
		}
	}()

	slog.Info("Conexão com banco bem sucedida", "database", cfg.Database.Name)

	migrateURL, err := buildMigrateDatabaseURL(cfg.Database.URI, cfg.Database.Name)
	if err != nil {
		slog.Error("Falha ao construir URL de migração", "error", err)
		return err
	}

	m, err := migrate.New("file://migrations", migrateURL)
	if err != nil {
		slog.Error("Falha ao inicializar migrador", "error", err)
		return err
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil || dbErr != nil {
			slog.Error("Falha ao encerrar migrador", "source_error", srcErr, "db_error", dbErr)
		}
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		slog.Error("Falha ao executar migrações", "error", err)
		return err
	}

	slog.Info("Migrações aplicadas com sucesso (ou banco já atualizado)!")
	return nil
}

func buildMigrateDatabaseURL(uri, dbName string) (string, error) {
	trimmedURI := strings.TrimSpace(uri)
	if trimmedURI == "" {
		return "", errors.New("database URI is required")
	}

	scheme, rest, found := strings.Cut(trimmedURI, "://")
	if !found || scheme == "" {
		return "", fmt.Errorf("invalid database URI: missing scheme")
	}
	scheme = strings.ToLower(scheme)
	if scheme != "mongodb" && scheme != "mongodb+srv" {
		return "", fmt.Errorf("unsupported database URI scheme %q", scheme)
	}

	// Strip fragment if present
	if fragIdx := strings.IndexByte(rest, '#'); fragIdx >= 0 {
		rest = rest[:fragIdx]
	}

	var rawQuery string
	if qIdx := strings.IndexByte(rest, '?'); qIdx >= 0 {
		rawQuery = rest[qIdx+1:]
		rest = rest[:qIdx]
	}

	var authority, path string
	if slashIdx := strings.IndexByte(rest, '/'); slashIdx >= 0 {
		authority = rest[:slashIdx]
		path = rest[slashIdx:]
	} else {
		authority = rest
	}

	hostPart := authority
	if atIdx := strings.LastIndexByte(authority, '@'); atIdx >= 0 {
		hostPart = authority[atIdx+1:]
	}
	if strings.TrimSpace(hostPart) == "" {
		return "", errors.New("database URI must include a host")
	}
	for _, h := range strings.Split(hostPart, ",") {
		if strings.TrimSpace(h) == "" {
			return "", errors.New("database URI contains an empty host entry")
		}
	}

	resolvedDB := strings.Trim(strings.TrimSpace(dbName), "/")
	if resolvedDB == "" {
		resolvedDB = strings.Trim(path, "/")
	}
	if resolvedDB == "" {
		return "", errors.New("database name is required")
	}

	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("invalid database URI query: %w", err)
	}
	if !q.Has("authSource") {
		q.Set("authSource", "admin")
	}

	return fmt.Sprintf("%s://%s/%s?%s", scheme, authority, resolvedDB, q.Encode()), nil
}

package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
)

func makePathLog(logDir string) (io.Writer, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	fileName := fmt.Sprintf("opspulse-%s.log", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(logDir, fileName)

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return io.MultiWriter(os.Stdout, file), nil
}

func SetupSlog(handler slog.Handler) (*slog.Logger, error) {
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, nil
}

func HandlerDefaultJSON(level slog.Level, logDir string) (slog.Handler, error) {
	multiWriter, err := makePathLog(logDir)
	if err != nil {
		return nil, err
	}

	return slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{Level: level}), nil
}

func HandlerDefaultText(level slog.Level, logDir string) (slog.Handler, error) {
	multiWriter, err := makePathLog(logDir)
	if err != nil {
		return nil, err
	}

	return slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: level}), nil
}

// InitLogger cria e configura o logger da aplicação com base nas configurações de App e Log.
// Injeta metadados globais (app e env) e aplica travas de segurança para produção.
func InitLogger(app config.AppConfig, cfg config.LogConfig) (*slog.Logger, error) {
	var handler slog.Handler
	var err error

	if cfg.Format == "json" {
		handler, err = HandlerDefaultJSON(cfg.Level, cfg.OutputDir)
	} else {
		handler, err = HandlerDefaultText(cfg.Level, cfg.OutputDir)
	}
	
	if err != nil {
		return nil, fmt.Errorf("falha ao inicializar handler de log: %w", err)
	}

	baseLogger := slog.New(handler)
	appLogger := baseLogger.With(
		slog.String("app", app.Name),
		slog.String("env", app.Env),
	)

	slog.SetDefault(appLogger)

	if !app.IsProduction() {
		return appLogger, nil
	}

	if cfg.Level == slog.LevelDebug {
		appLogger.Warn("TRAVA DE SEGURANÇA: Nível DEBUG ativado em ambiente de PRODUÇÃO! Risco de exposição de dados sensíveis e degradação de I/O.")
	}
	if cfg.Format != "json" {
		appLogger.Info("Recomendação de produção: LOG_FORMAT=json é recomendado para agregadores de log estruturado (ex: Datadog, Loki, CloudWatch).")
	}

	return appLogger, nil
}

package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

type LogConfig struct {
	Level     slog.Level
	Format    string
	OutputDir string
}

func ParseLogLevel(levelStr string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("nível de log inválido: '%s' (use debug, info, warn ou error)", levelStr)
	}
}

func LoadLogConfig(envManager file.EnvFile, typeLog string, defaultDir string) (LogConfig, error) {
	var err_s []error

	rawLevel, err := envManager.LoadVariable("LOG_LEVEL", "info")
	if err != nil {
		err_s = append(err_s, err)
	}

	level, err := ParseLogLevel(rawLevel)
	if err != nil {
		err_s = append(err_s, err)
	}

	format, err := envManager.LoadVariable("LOG_FORMAT", "json")
	if err != nil {
		err_s = append(err_s, err)
	}

	// TODO: gambiarra essa forma de carregar o diretório, arrumar depois
	outputDir, err := envManager.LoadVariable("LOG_OUTPUT_DIR_" + typeLog, defaultDir)
	if err != nil {
		err_s = append(err_s, err)
	}

	if len(err_s) > 0 {
		return LogConfig{}, errors.Join(err_s...)
	}

	return LogConfig{
		Level:     level,
		Format:    strings.ToLower(strings.TrimSpace(format)),
		OutputDir: outputDir,
	}, nil
}


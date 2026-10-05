package config

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

// TODO: solução temporária não é ideal isso, antipattern
var API = "API"
var APP = "APP"

type LogConfig struct {
	Level     slog.Level
	Format    string
	OutputDir string
}

func ParseLogLevel(levelStr string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		fmt.Printf("nível de log inválido: '%s' (use debug, info, warn ou error). Usando Info como fallback", levelStr)
		return slog.LevelInfo
	}
}

func LoadLogConfig(envManager file.EnvFile, typeLog string, defaultDir string) LogConfig {
	rawLevel := envManager.LoadVariable("LOG_LEVEL", defaultFallback.Log.Level.String())

	level := ParseLogLevel(rawLevel)

	format := envManager.LoadVariable("LOG_FORMAT", defaultFallback.Log.Format)

	if defaultDir == "" {
		defaultDir = defaultFallback.Log.OutputDir
	}

	// TODO: gambiarra essa forma de carregar o diretório, arrumar depois
	outputDir := envManager.LoadVariable("LOG_OUTPUT_DIR_"+typeLog, defaultDir)

	return LogConfig{
		Level:     level,
		Format:    strings.ToLower(strings.TrimSpace(format)),
		OutputDir: outputDir,
	}
}

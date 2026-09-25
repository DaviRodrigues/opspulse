package file

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/errs"
	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
)

type EnvFile struct{}

func NewEnvFile(filenames ...string) EnvFile {
    if err := godotenv.Load(filenames...); err != nil {
        slog.Warn("Não foi possível carregar arquivo .env (usando variáveis do sistema se existirem)", "error", err)
    }
	return EnvFile{}
}

func (e *EnvFile) LoadVariable(envVariable string, fallback string) (string, error) {
	value, exists := os.LookupEnv(envVariable)
	if !exists || strings.TrimSpace(value) == "" {
		if fallback != "" {
			return fallback, nil
		}
		slog.Error("Variável não existe no .env",
			"variable", envVariable,
		)
		return "", errs.ErrConfigNotFound
	}

	return value, nil
}

func (e *EnvFile) LoadListEnv(envVariable string) ([]string, error) {
	value, err := e.LoadVariable(
		envVariable,
		"https://github.com/, https://www.google.com/",
	)
	if err != nil {
		return make([]string, 0), err
	}

	rawUrls := strings.Split(value, ",")

	var cleanUrls []string
	for _, u := range rawUrls {
		trimmed := strings.TrimSpace(u)
		if trimmed != "" {
			cleanUrls = append(cleanUrls, trimmed)
		}
	}

	return cleanUrls, nil
}

func (e *EnvFile) LoadDurationEnv(envVariable string, fallback string) (time.Duration, error) {
	value, err := e.LoadVariable(
		envVariable,
		fallback,
	)
	if err != nil {
		return 0, err
	}

	interval, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errs.ErrInvalidInterval, value)
	}
	return interval, nil
}

package file

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/errs"
)

// TODO por agora o FileDefault não é necessário
type EnvFile struct {}

func (e *EnvFile) LoadVariable(envVariable string, fallback string) (string, error) {
	value, exists := os.LookupEnv(envVariable)
	if !exists {
		slog.Error("Variável não existe no .env",
			"variable", envVariable,
		)
		return "", errs.ErrConfigNotFound
	}

	if strings.TrimSpace(value) == "" {
		return fallback, nil
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

func (e *EnvFile) LoadDurationEnv(envVariable string) (time.Duration, error) {
	value, err := e.LoadVariable(
		envVariable,
		(30 * time.Second).String(),
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

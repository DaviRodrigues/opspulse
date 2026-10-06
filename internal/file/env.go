package file

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/domain"
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

func (e *EnvFile) LoadVariable(envVariable string, fallback string) (string) {
	value, exists := os.LookupEnv(envVariable)
	if !exists || strings.TrimSpace(value) == "" {
		slog.Warn("Variável não existe no .env carregando fallback",
			"variable", envVariable,
			"fallback", fallback,
		)
		return fallback
	}

	return value
}

func (e *EnvFile) LoadDurationEnv(envVariable string, fallback string) (time.Duration, error) {
	value := e.LoadVariable(
		envVariable,
		fallback,
	)

	interval, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", domain.ErrInvalidInterval, value)
	}
	return interval, nil
}

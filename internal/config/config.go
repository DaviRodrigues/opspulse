package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/errs"
	"github.com/joho/godotenv"
)

/*
TODO: fazer uma alteração para formas diferentes de carregas as urls
seja por arquivo, por env, por api, etc
*/

type Config struct {
	Discord DiscordConfig
	Monitor MonitorConfig
}

func LoadVariable(envVariable string, fallback string) (string, error) {
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

func loadListEnv(envVariable string) ([]string, error) {
	value, err := LoadVariable(
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

func loadDurationEnv(envVariable string) (time.Duration, error) {
	value, err := LoadVariable(
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

// TODO: vou ter que fazer uma abstract factory ou factory aqui depois, devido aos tipos de loader
func Load(targetLoader TargetLoader, filenames ...string) (Config, error) {
	_ = godotenv.Load(filenames...)

	var err_s []error
	monitorConfig, errMonitor := loadMonitorConfig(targetLoader)
	if errMonitor != nil {
		err_s = append(err_s, errMonitor)
	}

	discordConfig, errDiscord := loadDiscordConfig()
	if errDiscord != nil {
		err_s = append(err_s, errDiscord)
	}

	if len(err_s) > 0 {
		return Config{}, errors.Join(err_s...)
	}

	return Config{
		Monitor: monitorConfig,
		Discord: discordConfig,
	}, nil
}

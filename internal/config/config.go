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

type DiscordConfig struct {
	Token     string
	ChannelID string
	GuildID   string
}
type MonitorConfig struct {
	Interval       time.Duration
	Timeout        time.Duration
	TargetURLs     []string
	AlertThreshold int
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
		(30*time.Second).String(),
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

func Load(filenames ...string) (Config, error) {
	_ = godotenv.Load(filenames...)

	var errs []error

	token, err := LoadVariable(
		"DISCORD_TOKEN",
		"",
	)
	if err != nil {
		errs = append(errs, err)
	}

	channelID, err := LoadVariable("DISCORD_CHANNEL_ID","",
	)
	if err != nil {
		errs = append(errs, err)
	}

	checkInterval, err := loadDurationEnv("CHECK_INTERVAL")
	if err != nil {
		errs = append(errs, err)
	}

	checkTimeout, err := loadDurationEnv("CHECK_TIMEOUT")
	if err != nil {
		errs = append(errs, err)
	}

	targetUrls, err := loadListEnv("TARGET_URLS")
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}

	return Config{
		DiscordConfig{
			Token:     token,
			ChannelID: channelID,
			GuildID:   "", // TODO: preciso colocar como variável de ambiente depois
		},
		MonitorConfig{
			Interval:       checkInterval,
			Timeout:        checkTimeout,
			TargetURLs:     targetUrls,
			AlertThreshold: 0,
		},
	}, nil
}

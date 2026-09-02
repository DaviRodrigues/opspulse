package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	DiscordToken     string
	DiscordChannelID string
	CheckInterval    time.Duration
	CheckTimeout     time.Duration
	TargetURLs       []string
	AlertThreshold   int
}

func loadListEnv(env_variable string) ([]string, error) {
	envInterval := os.Getenv(env_variable)
	if envInterval == "" || envInterval == " " {
		return make([]string, 0), fmt.Errorf("a variável %s é obrigatória e não pode estar vazia", env_variable)
	}

	rawUrls := strings.Split(envInterval, ",")

	var cleanUrls []string
	for _, u := range rawUrls {
		trimmed := strings.TrimSpace(u)
		if trimmed != "" {
			cleanUrls = append(cleanUrls, trimmed)
		}
	}

	return cleanUrls, nil
}

func loadDurationEnv(env_variable string, defaultVal time.Duration) (time.Duration, error) {
	envInterval := os.Getenv(env_variable)
	if envInterval != "" {
		checkInterval, err := time.ParseDuration(envInterval)
		if err != nil {
			return 0, fmt.Errorf("Erro ao ler %v: %v. Use formatos como '30s' ou '5m'", env_variable, err)
		}
		return checkInterval, nil
	}

	return defaultVal, nil
}

func Load() (Config, error) {
	checkInterval, err := loadDurationEnv(
		"CHECK_INTERVAL",
		30 * time.Second)
	if err != nil {
		return Config{}, err
	}

	checkTimeout, err := loadDurationEnv(
		"CHECK_TIMEOUT",
		10 * time.Second)
	if err != nil {
		return Config{}, err
	}

	targetUrls, err := loadListEnv("TARGET_URLS")
	if err != nil {
		return Config{}, err
	}

	return Config{
		DiscordToken:     os.Getenv("DISCORD_TOKEN"),
		DiscordChannelID: os.Getenv("DISCORD_CHANNEL_ID"),
		CheckInterval:    checkInterval,
		CheckTimeout:     checkTimeout,
		TargetURLs:       targetUrls,
		AlertThreshold:   0,
	}, nil
}

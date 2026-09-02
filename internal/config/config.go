package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken     string
	DiscordChannelID string
	CheckInterval    time.Duration
	CheckTimeout     time.Duration
	TargetURLs       []string
	AlertThreshold   int
}

func loadVariable(envVariable string) string {
	return os.Getenv(envVariable)
}

func loadListEnv(envVariable string) ([]string, error) {
	envValue := loadVariable(envVariable)
	if strings.TrimSpace(envValue) == "" {
		return make([]string, 0), fmt.Errorf("a variável %s é obrigatória e não pode estar vazia", envVariable)
	}

	rawUrls := strings.Split(envValue, ",")

	var cleanUrls []string
	for _, u := range rawUrls {
		trimmed := strings.TrimSpace(u)
		if trimmed != "" {
			cleanUrls = append(cleanUrls, trimmed)
		}
	}

	return cleanUrls, nil
}

func loadDurationEnv(envVariable string, defaultVal time.Duration) (time.Duration, error) {
	envValue := loadVariable(envVariable)
	if envValue != "" {
		interval, err := time.ParseDuration(envValue)
		if err != nil {
			return 0, fmt.Errorf("Erro ao ler %v: %v. Use formatos como '30s' ou '5m'", envVariable, err)
		}
		return interval, nil
	}

	return defaultVal, nil
}

func loadStringEnv(envVariable string) (string, error) {
	envValue := loadVariable(envVariable)
	if strings.TrimSpace(envValue) == "" {
		return "", fmt.Errorf("Variável de ambiente %s não preenchida.", envVariable)
	}

	return envValue, nil
}

func Load(filenames ...string) (Config, error) {
	if err := godotenv.Load(filenames...); err != nil {
		log.Printf("Error loading .env file: %v", err)
	}

	var errs []error

	token, err := loadStringEnv(
		"DISCORD_TOKEN",
	)
	if err != nil {
		errs = append(errs, err)
	}

	channelID, err := loadStringEnv(
		"DISCORD_CHANNEL_ID",
	)
	if err != nil {
		errs = append(errs, err)
	}

	checkInterval, err := loadDurationEnv(
		"CHECK_INTERVAL",
		30*time.Second)
	if err != nil {
		errs = append(errs, err)
	}

	checkTimeout, err := loadDurationEnv(
		"CHECK_TIMEOUT",
		10*time.Second)
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
		DiscordToken:     token,
		DiscordChannelID: channelID,
		CheckInterval:    checkInterval,
		CheckTimeout:     checkTimeout,
		TargetURLs:       targetUrls,
		AlertThreshold:   0,
	}, nil
}

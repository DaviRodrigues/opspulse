package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/DaviRodrigues/opspulse/internal/errs"
)

/*
TODO: fazer uma alteração para formas diferentes de carregas as urls
seja por arquivo, por env, por api, etc
*/

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
		return make([]string, 0), fmt.Errorf("%w: %s", errs.ErrConfigNotFound, envVariable)
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
			return 0, fmt.Errorf("%w: %v", errs.ErrInvalidInterval, envVariable)
		}
		return interval, nil
	}

	return defaultVal, nil
}

func loadStringEnv(envVariable string, isEmpty bool) (string, error) {
	envValue := loadVariable(envVariable)
	if isEmpty {
		fmt.Printf("Essa variável %v pode ser vazia, mas algumas funções podem não funcionar", envVariable)
		return "", nil
	}


	if strings.TrimSpace(envValue) == "" {
		return "", fmt.Errorf("%w: %s", errs.ErrConfigNotFound, envVariable)
	}

	return envValue, nil
}

func Load(filenames ...string) (Config, error) {
	if err := godotenv.Load(filenames...); err != nil {
		// aplicar log corretamente aqui depois
		log.Printf("Error loading .env file: %v", err)
	}

	var errs []error

	token, err := loadStringEnv(
		"DISCORD_TOKEN",
		true,
	)
	if err != nil {
		errs = append(errs, err)
	}

	channelID, err := loadStringEnv(
		"DISCORD_CHANNEL_ID",
		true,
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

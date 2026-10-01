package config

import (
	"strings"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

type AppConfig struct {
	Name string
	Env  string // "development" | "staging" | "production"
}

func (a AppConfig) IsProduction() bool {
	env := strings.ToLower(strings.TrimSpace(a.Env))
	return env == "production" || env == "prod"
}

func (a AppConfig) IsDevelopment() bool {
	return !a.IsProduction()
}

func LoadAppConfig(envManager file.EnvFile) (AppConfig, error) {
	name, err := envManager.LoadVariable("APP_NAME", "opspulse")
	if err != nil {
		return AppConfig{}, err
	}

	env, err := envManager.LoadVariable("APP_ENV", "development")
	if err != nil {
		return AppConfig{}, err
	}

	return AppConfig{
		Name: name,
		Env:  strings.ToLower(strings.TrimSpace(env)),
	}, nil
}


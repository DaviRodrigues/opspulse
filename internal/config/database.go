package config

import (
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

type DatabaseConfig struct {
	URI     string
	Name    string
	Timeout time.Duration
}

func LoadDatabaseConfig(envManager file.EnvFile) (DatabaseConfig, error) {
	db_uri := envManager.LoadVariable("DB_URI", defaultFallback.Database.URI)

	db_name := envManager.LoadVariable("DB_NAME", defaultFallback.Database.Name)

	db_timeout, err := envManager.LoadDurationEnv("DB_TIMEOUT", defaultFallback.Database.Timeout.String())
	if err != nil {
		return DatabaseConfig{}, err
	}

	return DatabaseConfig{
		URI:     db_uri,
		Name:    db_name,
		Timeout: db_timeout,
	}, nil
}

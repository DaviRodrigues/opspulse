package config

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

type ServerConfig struct {
	Port               string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	ShutdownTimeout    time.Duration
	CORSAllowedOrigins []string
}

func parseAllowedOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	if len(origins) == 0 {
		return slices.Clone(defaultFallback.Server.CORSAllowedOrigins)
	}

	return origins
}

func LoadServerConfig(envManager file.EnvFile) (ServerConfig, error) {
	var err_s []error

	port := envManager.LoadVariable("SERVER_PORT", defaultFallback.Server.Port)

	readTimeout, err := envManager.LoadDurationEnv("SERVER_READ_TIMEOUT", defaultFallback.Server.ReadTimeout.String())
	if err != nil {
		err_s = append(err_s, err)
	}

	writeTimeout, err := envManager.LoadDurationEnv("SERVER_WRITE_TIMEOUT", defaultFallback.Server.WriteTimeout.String())
	if err != nil {
		err_s = append(err_s, err)
	}

	shutdownTimeout, err := envManager.LoadDurationEnv("SERVER_SHUTDOWN_TIMEOUT", defaultFallback.Server.ShutdownTimeout.String())
	if err != nil {
		err_s = append(err_s, err)
	}

	defaultOrigins := strings.Join(defaultFallback.Server.CORSAllowedOrigins, ",")
	rawOrigins := envManager.LoadVariable("CORS_ALLOWED_ORIGINS", defaultOrigins)
	allowedOrigins := parseAllowedOrigins(rawOrigins)

	if len(err_s) > 0 {
		return ServerConfig{}, errors.Join(err_s...)
	}

	return ServerConfig{
		Port:               port,
		ReadTimeout:        readTimeout,
		WriteTimeout:       writeTimeout,
		ShutdownTimeout:    shutdownTimeout,
		CORSAllowedOrigins: allowedOrigins,
	}, nil
}

package config

import (
	"errors"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func LoadServerConfig(envManager file.EnvFile) (ServerConfig, error) {
	var err_s []error

	port, err := envManager.LoadVariable("SERVER_PORT", "3333")
	if err != nil {
		err_s = append(err_s, err)
	}

	readTimeout, err := envManager.LoadDurationEnv("SERVER_READ_TIMEOUT", (5 * time.Second).String())
	if err != nil {
		err_s = append(err_s, err)
	}

	writeTimeout, err := envManager.LoadDurationEnv("SERVER_WRITE_TIMEOUT", (0 * time.Second).String())
	if err != nil {
		err_s = append(err_s, err)
	}

	if len(err_s) > 0 {
		return ServerConfig{}, errors.Join(err_s...)
	}

	return ServerConfig{
		Port:         port,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}, nil
}

package config

import (
	"errors"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

type MonitorConfig struct {
	Interval       time.Duration
	DefaultTimeout time.Duration
	AlertThreshold int
}

func LoadMonitorConfig(envManager file.EnvFile) (MonitorConfig, error) {
	var err_s []error

	checkInterval, err := envManager.LoadDurationEnv("MONITOR_INTERVAL", defaultFallback.Monitor.Interval.String())
	if err != nil {
		err_s = append(err_s, err)
	}

	defaultTimeout, err := envManager.LoadDurationEnv("MONITOR_DEFAULT_TIMEOUT", defaultFallback.Monitor.DefaultTimeout.String())
	if err != nil {
		err_s = append(err_s, err)
	}

	if len(err_s) > 0 {
		return MonitorConfig{}, errors.Join(err_s...)
	}

	return MonitorConfig{
		Interval:       checkInterval,
		DefaultTimeout: defaultTimeout,
		AlertThreshold: defaultFallback.Monitor.AlertThreshold,
	}, nil
}

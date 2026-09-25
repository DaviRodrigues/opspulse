package config

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

type MonitorConfig struct {
	Interval       time.Duration
	TargetURLs     []file.Target
	AlertThreshold int
}

func LoadMonitorConfig(targetLoader file.TargetLoader, envManager file.EnvFile) (MonitorConfig, error) {
	var err_s []error

	checkInterval, err := envManager.LoadDurationEnv("MONITOR_INTERVAL", (5 * time.Minute).String())
	if err != nil {
		err_s = append(err_s, err)
	}

	if customTargetFile, exists := os.LookupEnv("MONITOR_TARGETS_FILE"); exists && strings.TrimSpace(customTargetFile) != "" {
		if err := targetLoader.NewFile(strings.TrimSpace(customTargetFile)); err != nil {
			err_s = append(err_s, err)
		}
	}

	targetUrls, err := targetLoader.Load()
	if err != nil {
		err_s = append(err_s, err)
	}

	if len(err_s) > 0 {
		return MonitorConfig{}, errors.Join(err_s...)
	}

	return MonitorConfig{
		Interval:       checkInterval,
		TargetURLs:     targetUrls,
		AlertThreshold: 0,
	}, nil
}


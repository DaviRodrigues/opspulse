package config

import (
	"errors"
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

	targetsFile, err := envManager.LoadVariable("MONITOR_TARGETS_FILE", "./target/target.json")
	if err != nil {
		err_s = append(err_s, err)
	}

	if err := targetLoader.NewFile(targetsFile); err != nil {
		err_s = append(err_s, err)
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

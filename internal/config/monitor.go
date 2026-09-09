package config

import (
	"errors"
	"time"
)

type MonitorConfig struct {
	Interval       time.Duration
	TargetURLs     []Target
	AlertThreshold int
}

func loadMonitorConfig(targetLoader TargetLoader) (MonitorConfig, error) {
	var err_s []error

	checkInterval, err := loadDurationEnv("CHECK_INTERVAL")
	if err != nil {
		err_s = append(err_s, err)
	}

	targetUrls, err := targetLoader.LoadTargets()
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

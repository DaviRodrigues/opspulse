package config

import (
	"errors"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/errs"
	"github.com/DaviRodrigues/opspulse/internal/file"
)

type MonitorConfig struct {
	Interval       time.Duration
	TargetURLs     []file.Target
	AlertThreshold int
}

func LoadMonitorConfig(envManager file.EnvFile) (MonitorConfig, error) {
	var err_s []error

	checkInterval, err := envManager.LoadDurationEnv("MONITOR_INTERVAL", defaultFallback.Monitor.Interval.String())
	if err != nil {
		err_s = append(err_s, err)
	}

	pathTargetFile := envManager.LoadVariable("MONITOR_TARGETS_FILE", defaultTargetsFilePath)

	loader, isValidFormat := file.GetLoaderByFile(pathTargetFile)
	if !isValidFormat {
		err_s = append(err_s, errs.ErrInvalidFileFormat)
	}

	targetUrls, err := loader.Load()
	if err != nil {
		err_s = append(err_s, err)
	}

	if len(err_s) > 0 {
		return MonitorConfig{}, errors.Join(err_s...)
	}

	return MonitorConfig{
		Interval:       checkInterval,
		TargetURLs:     targetUrls,
		AlertThreshold: defaultFallback.Monitor.AlertThreshold,
	}, nil
}

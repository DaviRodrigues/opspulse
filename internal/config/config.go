package config

import (
	"errors"

	"github.com/DaviRodrigues/opspulse/internal/file"
	"github.com/joho/godotenv"
)

/*
TODO: fazer uma alteração para formas diferentes de carregas as urls
seja por arquivo, por env, por api, etc
*/

type Config struct {
	Discord DiscordConfig
	Monitor MonitorConfig
}

func Load(targetLoader file.TargetLoader, filenames ...string) (Config, error) {
	_ = godotenv.Load(filenames...)
	envManager := file.EnvFile{}

	var err_s []error
	checkInterval, err := envManager.LoadDurationEnv("CHECK_INTERVAL")
	if err != nil {
		err_s = append(err_s, err)
	}

	discordConfig, errDiscord := loadDiscordConfig(envManager)
	if errDiscord != nil {
		err_s = append(err_s, errDiscord)
	}

	monitorConfig, errMonitor := loadMonitorConfig(targetLoader, checkInterval)
	if errMonitor != nil {
		err_s = append(err_s, errMonitor)
	}

	if len(err_s) > 0 {
		return Config{}, errors.Join(err_s...)
	}

	return Config{
		Monitor: monitorConfig,
		Discord: discordConfig,
	}, nil
}

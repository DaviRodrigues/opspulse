package config

import (
	"errors"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

type Config struct {
	Discord DiscordConfig
	Monitor MonitorConfig
	Server  ServerConfig
}

func Load(targetLoader file.TargetLoader, envManager file.EnvFile) (Config, error) {
	var err_s []error

	discordConfig, errDiscord := LoadDiscordConfig(envManager)
	if errDiscord != nil {
		err_s = append(err_s, errDiscord)
	}

	monitorConfig, errMonitor := LoadMonitorConfig(targetLoader, envManager)
	if errMonitor != nil {
		err_s = append(err_s, errMonitor)
	}

	serverConfig, errServer := LoadServerConfig(envManager)
	if errServer != nil {
		err_s = append(err_s, errServer)
	}

	if len(err_s) > 0 {
		return Config{}, errors.Join(err_s...)
	}

	return Config{
		Monitor: monitorConfig,
		Discord: discordConfig,
		Server: serverConfig,
	}, nil
}

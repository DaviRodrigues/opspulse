package config

import (
	"errors"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

// TODO: solução temporária não é ideal isso, antipattern
var API = "API"
var APP = "APP"

type Config struct {
	App     AppConfig
	Log     LogConfig
	Discord DiscordConfig
	Monitor MonitorConfig
	Server  ServerConfig
}

func Load(typeLog string, targetLoader file.TargetLoader, envManager file.EnvFile) (Config, error) {
	var err_s []error

	appConfig, errApp := LoadAppConfig(envManager)
	if errApp != nil {
		err_s = append(err_s, errApp)
	}

	logConfig, errLog := LoadLogConfig(envManager, typeLog, "./log")
	if errLog != nil {
		err_s = append(err_s, errLog)
	}

	serverConfig, errServer := LoadServerConfig(envManager)
	if errServer != nil {
		err_s = append(err_s, errServer)
	}

	discordConfig, errDiscord := LoadDiscordConfig(envManager)
	if errDiscord != nil {
		err_s = append(err_s, errDiscord)
	}

	monitorConfig, errMonitor := LoadMonitorConfig(targetLoader, envManager)
	if errMonitor != nil {
		err_s = append(err_s, errMonitor)
	}

	if len(err_s) > 0 {
		return Config{}, errors.Join(err_s...)
	}

	return Config{
		App:     appConfig,
		Log:     logConfig,
		Monitor: monitorConfig,
		Discord: discordConfig,
		Server:  serverConfig,
	}, nil
}


package config

import (
	"errors"
	"log/slog"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

var defaultFallback = Config{
	App: AppConfig{
		Name: "opspulse",
		Env:  "development",
	},
	Log: LogConfig{
		Level:     slog.LevelInfo,
		Format:    "json",
		OutputDir: "./log",
	},
	Server: ServerConfig{
		Port:            "3333",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    0,
		ShutdownTimeout: 15 * time.Second,
	},
	Monitor: MonitorConfig{
		Interval:       5 * time.Minute,
		AlertThreshold: 0,
	},
	Discord: DiscordConfig{
		Token:     "meu-token-secreto",
		ChannelID: "123456789",
		GuildID:   "123456789",
	},
}

const defaultTargetsFilePath = "./target/target.json"

type Config struct {
	App     AppConfig
	Log     LogConfig
	Discord DiscordConfig
	Monitor MonitorConfig
	Server  ServerConfig
}

func Load(typeLog string, envManager file.EnvFile) (Config, error) {
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

	discordConfig := LoadDiscordConfig(envManager)

	monitorConfig, errMonitor := LoadMonitorConfig(envManager)
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

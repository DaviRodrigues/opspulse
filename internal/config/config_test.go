package config

import (
	"log/slog"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

func TestEnvLoadOk(t *testing.T) {
	cfg, err := Load(
		APP,
		file.NewEnvFile("./testdata/config_valid.env"),
	)
	if err != nil {
		t.Fatalf("não esperava erro, mas recebeu: %v", err)
	}

	if len(cfg.Monitor.TargetURLs) != 2 {
		t.Errorf("esperava 2 URLs, recebeu: %d", len(cfg.Monitor.TargetURLs))
	}

	if cfg.Monitor.Interval != 15*time.Second {
		t.Errorf("esperava intervalo 15s, recebeu: %v", cfg.Monitor.Interval)
	}

	if cfg.Discord.Token != defaultFallback.Discord.Token {
		t.Errorf("esperava token 'meu-token-secreto', recebeu: %s", cfg.Discord.Token)
	}

	if cfg.App.Name != defaultFallback.App.Name {
		t.Errorf("esperava app name 'custom-pulse', recebeu: %s", cfg.App.Name)
	}

	if !cfg.App.IsProduction() {
		t.Errorf("esperava que IsProduction fosse true")
	}

	if cfg.Log.Level != slog.LevelDebug {
		t.Errorf("esperava log level debug, recebeu: %v", cfg.Log.Level)
	}

	if cfg.Server.Port != defaultFallback.Server.Port {
		t.Errorf("esperava fallback para SERVER_PORT %s, recebeu: %s", cfg.Server.Port, defaultFallback.Server.Port)
	}
}

func TestAppConfig_EnvironmentHelpers(t *testing.T) {
	prod := AppConfig{Env: "production"}
	if !prod.IsProduction() || prod.IsDevelopment() {
		t.Errorf("esperava IsProduction=true e IsDevelopment=false para 'production'")
	}

	prodShort := AppConfig{Env: "prod"}
	if !prodShort.IsProduction() {
		t.Errorf("esperava IsProduction=true para 'prod'")
	}

	dev := AppConfig{Env: "development"}
	if dev.IsProduction() || !dev.IsDevelopment() {
		t.Errorf("esperava IsProduction=false e IsDevelopment=true para 'development'")
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input       string
		expected    slog.Level
		expectError bool
	}{
		{"debug", slog.LevelDebug, false},
		{"DEBUG", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"invalid_level", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lvl:= ParseLogLevel(tt.input)

			if !tt.expectError && lvl != tt.expected {
				t.Errorf("ParseLogLevel(%q) = %v, esperava %v", tt.input, lvl, tt.expected)
			}
		})
	}
}

func TestServerConfig_Fallback(t *testing.T) {
	cfg, err := LoadServerConfig(file.NewEnvFile())
	if err != nil {
		t.Fatalf("não esperava erro ao carregar server config: %v", err)
	}

	if cfg.Port != defaultFallback.Server.Port {
		t.Errorf("esperava fallback para SERVER_PORT %s, recebeu: %s", cfg.Port, defaultFallback.Server.Port)
	}
}

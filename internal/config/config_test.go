package config

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

func TestEnvLoadOk(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "target.json")
	jsonLoader := &file.JSONFile{
		FileDefault: file.FileDefault{
			Path: filePath,
		},
	}

	t.Setenv("DISCORD_TOKEN", "meu-token-secreto")
	t.Setenv("DISCORD_CHANNEL_ID", "123456789")
	t.Setenv("DISCORD_GUILD_ID", "123456789")
	t.Setenv("MONITOR_INTERVAL", "15s")
	t.Setenv("APP_NAME", "custom-pulse")
	t.Setenv("APP_ENV", "production")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("SERVER_PORT", "8080")

	cfg, err := Load(
		APP,
		jsonLoader, 
		file.NewEnvFile(),
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

	if cfg.Discord.Token != "meu-token-secreto" {
		t.Errorf("esperava token 'meu-token-secreto', recebeu: %s", cfg.Discord.Token)
	}

	if cfg.App.Name != "custom-pulse" {
		t.Errorf("esperava app name 'custom-pulse', recebeu: %s", cfg.App.Name)
	}

	if !cfg.App.IsProduction() {
		t.Errorf("esperava que IsProduction fosse true")
	}

	if cfg.Log.Level != slog.LevelDebug {
		t.Errorf("esperava log level debug, recebeu: %v", cfg.Log.Level)
	}

	if cfg.Server.Port != "8080" {
		t.Errorf("esperava porta 8080, recebeu: %s", cfg.Server.Port)
	}
}

func TestEnvLoadErr(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "target.json")
	jsonLoader := &file.JSONFile{
		FileDefault: file.FileDefault{
			Path: filePath,
		},
	}

	t.Setenv("DISCORD_TOKEN", "")

	_, err := Load(
		APP,
		jsonLoader, 
		file.NewEnvFile(),
	)

	if err == nil {
		t.Errorf("esperava que Load() retornasse erro de validação, mas retornou nil")
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
			lvl, err := ParseLogLevel(tt.input)
			if (err != nil) != tt.expectError {
				t.Errorf("ParseLogLevel(%q) erro inesperado: %v, esperava erro: %v", tt.input, err, tt.expectError)
			}
			if !tt.expectError && lvl != tt.expected {
				t.Errorf("ParseLogLevel(%q) = %v, esperava %v", tt.input, lvl, tt.expected)
			}
		})
	}
}

func TestServerConfig_Fallback(t *testing.T) {
	t.Setenv("SERVER_PORT", "")
	t.Setenv("PORT", "9090")

	cfg, err := LoadServerConfig(file.NewEnvFile())
	if err != nil {
		t.Fatalf("não esperava erro ao carregar server config: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("esperava fallback para PORT '9090', recebeu: %s", cfg.Port)
	}
}


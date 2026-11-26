package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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

	if cfg.Monitor.Interval != 15*time.Second {
		t.Errorf("esperava intervalo 15s, recebeu: %v", cfg.Monitor.Interval)
	}

	if cfg.Monitor.DefaultTimeout != 5*time.Second {
		t.Errorf("esperava DefaultTimeout 5s, recebeu: %v", cfg.Monitor.DefaultTimeout)
	}

	if cfg.Discord.Token != defaultFallback.Discord.Token {
		t.Errorf("esperava token %q, recebeu: %q", defaultFallback.Discord.Token, cfg.Discord.Token)
	}

	if cfg.App.Name != defaultFallback.App.Name {
		t.Errorf("esperava app name %q, recebeu: %q", defaultFallback.App.Name, cfg.App.Name)
	}

	if !cfg.App.IsProduction() {
		t.Errorf("esperava que IsProduction fosse true")
	}

	if cfg.Log.Level != slog.LevelDebug {
		t.Errorf("esperava log level debug, recebeu: %v", cfg.Log.Level)
	}

	if cfg.Server.Port != defaultFallback.Server.Port {
		t.Errorf("esperava fallback para SERVER_PORT %s, recebeu: %s", defaultFallback.Server.Port, cfg.Server.Port)
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
			lvl := ParseLogLevel(tt.input)

			if !tt.expectError && lvl != tt.expected {
				t.Errorf("ParseLogLevel(%q) = %v, esperava %v", tt.input, lvl, tt.expected)
			}
		})
	}
}

func TestServerConfig_FallbackAndCORS(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	cfg, err := LoadServerConfig(file.NewEnvFile())
	if err != nil {
		t.Fatalf("não esperava erro ao carregar server config: %v", err)
	}

	if cfg.Port != defaultFallback.Server.Port {
		t.Errorf("esperava fallback para SERVER_PORT %s, recebeu: %s", defaultFallback.Server.Port, cfg.Port)
	}

	expectedDefaultOrigins := []string{"http://localhost:5173", "http://localhost:5174"}
	if !slices.Equal(cfg.CORSAllowedOrigins, expectedDefaultOrigins) {
		t.Errorf("esperava fallback CORSAllowedOrigins=%v, recebeu: %v", expectedDefaultOrigins, cfg.CORSAllowedOrigins)
	}

	t.Setenv("CORS_ALLOWED_ORIGINS", " http://localhost:3000 , , https://opspulse.local , ")
	cfgCustom, err := LoadServerConfig(file.NewEnvFile())
	if err != nil {
		t.Fatalf("não esperava erro ao carregar server config customizado: %v", err)
	}

	expectedCustom := []string{"http://localhost:3000", "https://opspulse.local"}
	if !slices.Equal(cfgCustom.CORSAllowedOrigins, expectedCustom) {
		t.Errorf("esperava CORSAllowedOrigins=%v, recebeu: %v", expectedCustom, cfgCustom.CORSAllowedOrigins)
	}
}

func TestLoadMonitorConfig_NoTargetsFileDependency(t *testing.T) {
	nonExistentTargetFile := filepath.Join(t.TempDir(), "missing", "target.json")
	t.Setenv("MONITOR_TARGETS_FILE", nonExistentTargetFile)
	t.Setenv("MONITOR_DEFAULT_TIMEOUT", "7s")
	t.Setenv("MONITOR_INTERVAL", "20s")

	cfg, err := LoadMonitorConfig(file.NewEnvFile())
	if err != nil {
		t.Fatalf("esperava sucesso sem arquivo de targets, recebeu erro: %v", err)
	}

	if cfg.Interval != 20*time.Second {
		t.Errorf("esperava Interval=20s, recebeu %v", cfg.Interval)
	}
	if cfg.DefaultTimeout != 7*time.Second {
		t.Errorf("esperava DefaultTimeout=7s, recebeu %v", cfg.DefaultTimeout)
	}

	if _, statErr := os.Stat(nonExistentTargetFile); !os.IsNotExist(statErr) {
		t.Errorf("não esperava criação do arquivo %s", nonExistentTargetFile)
	}
	if _, statErr := os.Stat("./target/target.json"); !os.IsNotExist(statErr) {
		t.Errorf("não esperava criação de ./target/target.json")
	}
}

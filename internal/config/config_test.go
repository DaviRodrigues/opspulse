package config

import (
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/file"
)

func TestEnvLoadOk(t *testing.T) {
	tmpDir := t.TempDir()
	jsonLoader := &file.JSONFile{
		FileDefault: file.FileDefault{
			Name: "targets.json",
			Path: tmpDir,
		},
	}

	t.Setenv("DISCORD_TOKEN", "meu-token-secreto")
	t.Setenv("DISCORD_CHANNEL_ID", "123456789")
	t.Setenv("DISCORD_GUILD_ID", "123456789")
	t.Setenv("CHECK_INTERVAL", "15s")

	cfg, err := Load(jsonLoader)
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
}

func TestEnvLoadErr(t *testing.T) {
	tmpDir := t.TempDir()
	jsonLoader := &file.JSONFile{
		FileDefault: file.FileDefault{
			Name: "targets.json",
			Path: tmpDir,
		},
	}

	t.Setenv("DISCORD_TOKEN", "")

	_, err := Load(jsonLoader)
	if err == nil {
		t.Errorf("esperava que Load() retornasse erro de validação, mas retornou nil")
	}
}

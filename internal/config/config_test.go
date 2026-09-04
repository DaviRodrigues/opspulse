package config

import (
	"testing"
	"time"
)

func TestLoadOk(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "meu-token-secreto")
	t.Setenv("DISCORD_CHANNEL_ID", "123456789")
	t.Setenv("TARGET_URLS", "https://google.com, https://github.com")
	t.Setenv("CHECK_INTERVAL", "15s")
	t.Setenv("CHECK_TIMEOUT", "5s")

	cfg, err := Load()
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

func TestLoadErr(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "")
	t.Setenv("CHECK_INTERVAL", "tempo_invalido_123")
	t.Setenv("TARGET_URLS", "")

	_, err := Load()
	if err == nil {
		t.Errorf("esperava que Load() retornasse erro de validação, mas retornou nil")
	}
}
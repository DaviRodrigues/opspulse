package discord

import (
	"os"
	"testing"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/joho/godotenv"
)

func TestBot_Validation(t *testing.T) {
	_, err := New(&config.DiscordConfig{})
	if err == nil {
		t.Errorf("esperava erro ao passar token e channel vazios, mas retornou nil")
	}
}

func TestBot_Integration(t *testing.T) {
	// Caso queira testar, o ideal é colocar .env na raiz do módulo discord
	_ = godotenv.Load()
	token := os.Getenv("DISCORD_TOKEN")
	channelID := os.Getenv("DISCORD_CHANNEL_ID")
	if token == "" {
		t.Skip("Pulando teste de integração real: DISCORD_TOKEN não configurado no ambiente")
	}
	if channelID == "" {
		t.Skip("Pulando teste de integração real: DISCORD_CHANNEL_ID não configurado no ambiente")
	}

	bot, err := New(&config.DiscordConfig{
		Token: token,
		ChannelID: channelID,
		GuildID: "",
	})
	if err != nil {
		t.Fatalf("falha ao conectar ao discord: %v", err)
	}
	defer bot.Close()
}

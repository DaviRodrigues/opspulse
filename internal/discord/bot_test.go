package discord

import (
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/models"
)

func TestBot(t *testing.T) {
	cfg, err := config.Load("../../.env")
	if err != nil {
		t.Errorf("%v", err)
	}

	bot, err := New(cfg.DiscordToken, cfg.DiscordChannelID)
	if err != nil {
		t.Errorf("%v", err)
	}
	defer bot.Close()

	mockCheck := models.CheckResult{
		URL: "https://exemplo.com", 
		IsUp: true, 
		StatusCode: 200, 
		Latency: 120 * time.Millisecond,
	}

	err = bot.SendAlert(mockCheck)
	if err != nil {
		t.Errorf("%v", err)
	}
}

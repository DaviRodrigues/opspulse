package discord

import (
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/bwmarrin/discordgo"
)

func TestCreateEmbed_ColorsAndStatus(t *testing.T) {
	// 1. Testa serviço UP (Verde)
	upResult := checker.CheckResult{
		URL:        "https://api.exemplo.com",
		IsUp:       true,
		StatusCode: 200,
		Latency:    50 * time.Millisecond,
	}
	embedUp := createEmbed(upResult)
	if embedUp.Color != 0x2ECC71 {
		t.Errorf("esperava cor verde 0x2ECC71 para UP, recebeu: %x", embedUp.Color)
	}

	// 2. Testa serviço DOWN (Vermelho)
	downResult := checker.CheckResult{
		URL:        "https://api.exemplo.com/erro",
		IsUp:       false,
		StatusCode: 500,
		Latency:    100 * time.Millisecond,
	}
	embedDown := createEmbed(downResult)
	if embedDown.Color != 0xE74C3C {
		t.Errorf("esperava cor vermelha 0xE74C3C para DOWN, recebeu: %x", embedDown.Color)
	}
}

func TestCreateRecheckButton(t *testing.T) {
	row := createRecheckButton()
	if len(row.Components) == 0 {
		t.Fatalf("esperava ao menos 1 componente na linha")
	}

	btn, ok := row.Components[0].(discordgo.Button)
	if !ok {
		t.Fatalf("esperava que o componente fosse um discordgo.Button")
	}

	if btn.CustomID != "btn_recheck" {
		t.Errorf("esperava CustomID 'btn_recheck', recebeu: %s", btn.CustomID)
	}
}
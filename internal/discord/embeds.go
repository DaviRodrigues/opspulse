package discord

import (
	"fmt"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/bwmarrin/discordgo"
)

func titleAndColorStatus(result checker.CheckResult) (int, string) {
	var color int
	var title string

	if result.IsUp {
		color = 0x2ECC71 // Verde em Hexadecimal
		title = "🟢 [UP] Serviço Operacional"
	} else {
		color = 0xE74C3C // Vermelho em Hexadecimal
		title = "🔴 [DOWN] Alerta de Indisponibilidade"
	}

	return color, title
}

func createEmbed(result checker.CheckResult) *discordgo.MessageEmbed {
	color, title := titleAndColorStatus(result)

	statusText := fmt.Sprintf("%d", result.StatusCode)
	if result.StatusCode == 0 {
		statusText = "N/A (Falha de Conexão)"
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: fmt.Sprintf("Verificação realizada para o endpoint: **%s**", result.URL),
		Color:       color,
		Timestamp:   time.Now().Format(time.RFC3339),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Status HTTP",
				Value:  statusText,
				Inline: true,
			},
			{
				Name:   "Latência",
				Value:  result.Latency.String(),
				Inline: true,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "OpsPulse Monitor",
		},
	}
}

func createActionRowButton(label, customID string, style discordgo.ButtonStyle) discordgo.ActionsRow {
    return discordgo.ActionsRow{
        Components: []discordgo.MessageComponent{
            discordgo.Button{
                Label:    label,
                Style:    style,
                CustomID: customID,
            },
        },
    }
}

func createRecheckButton() discordgo.ActionsRow {
	return createActionRowButton(
		"🔄 Checar Novamente",
		"btn_recheck",
		discordgo.PrimaryButton,
	)
}

func createStatusSummaryEmbed(results []checker.CheckResult) []*discordgo.MessageEmbed {
	var embeds []*discordgo.MessageEmbed
	for _, result := range results {
		embeds = append(embeds, createEmbed(result))
	}

	return embeds
}

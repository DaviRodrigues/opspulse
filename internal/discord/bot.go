package discord

import (
	"fmt"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/models"
	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session   *discordgo.Session
	channelID string
}

func titleAndColorStatus(result models.CheckResult) (int, string) {
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

func createEmbed(result models.CheckResult) *discordgo.MessageEmbed {
	color, title := titleAndColorStatus(result)

	statusText := fmt.Sprintf("%d", result.StatusCode)
	if result.StatusCode == 0 {
		statusText = "N/A (Falha de Conexão)"
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: fmt.Sprintf("Verrificação realizada para o endpoint: **%s**", result.URL),
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


func New(token, channelId string) (*Bot, error) {
	// ATENÇÃO: Prefixo Bot é exigido antes do token pela documentação
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		// Posso depois criar uma parte para logs e erros padrões
		return nil, fmt.Errorf("")
	}

	err = dg.Open()
	if err != nil {
		return nil, fmt.Errorf("")
	}

	return &Bot{
		session:   dg,
		channelID: channelId,
	}, nil
}

func (b *Bot) Close() {
	b.session.Close()
}

func (b *Bot) SendAlert(result models.CheckResult) error {
	embed := createEmbed(result)

	if result.Error != nil {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name: "Detalhe do Erro",
			Value: fmt.Sprintf("`%v`", result.Error),
			Inline: false,
		})
	}

	_, err := b.session.ChannelMessageSendEmbed(
		b.channelID,
		embed,
	)

	return err
}

// Falta criar o token e conta no discord developer. Depois disso testar
package discord

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/errs"
	"github.com/DaviRodrigues/opspulse/internal/models"
	"github.com/bwmarrin/discordgo"
)

/*
TODO implementar comandos depois e comandos por interface do discord
*/

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

func validateRequiredVariables(token, channelId string) error {
	var errs_v []error

	if strings.TrimSpace(token) == "" {
		errs_v = append(errs_v, fmt.Errorf("Token %w", errs.ErrConfigNotFound))
	}

	if strings.TrimSpace(channelId) == "" {
		errs_v = append(errs_v, fmt.Errorf("Channel %w", errs.ErrConfigNotFound))
	}

	if len(errs_v) > 0 {
		return errors.Join(errs_v...)
	}

	return nil
}

func New(token, channelId string) (*Bot, error) {
	if err := validateRequiredVariables(token, channelId); err != nil {
		return nil, err
	}

	// ATENÇÃO: Prefixo Bot é exigido antes do token pela documentação
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		slog.Error("Session Unavaible",
			"errType", errs.ErrDiscordAuth,
			"error", err,
		)
		return nil, fmt.Errorf("%w session (more info: %w)", errs.ErrDiscordAuth, err)
	}

	err = dg.Open()
	if err != nil {
		slog.Error("Connection Error",
			"errType", errs.ErrDiscordAuth,
			"error", err,
		)
		return nil, fmt.Errorf("%w open conection (more info: %w)", errs.ErrDiscordAuth, err)
	}

	slog.Info("Conexão com Discord estabelecida", "channel_id", channelId)

	return &Bot{
		session:   dg,
		channelID: channelId,
	}, nil
}

func (b *Bot) Close() {
	slog.Info("Conexão com Discord encerrada")

	b.session.Close()
}

func (b *Bot) SendAlert(result models.CheckResult) error {
	embed := createEmbed(result)

	if result.Error != nil {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Detalhe do Erro",
			Value:  fmt.Sprintf("`%v`", result.Error),
			Inline: false,
		})
	}

	_, err := b.session.ChannelMessageSendEmbed(
		b.channelID,
		embed,
	)

	return err
}

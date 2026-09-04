package discord

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/errs"
	"github.com/bwmarrin/discordgo"
)

/*
TODO implementar comandos depois e comandos por interface do discord
*/

type Bot struct {
	session   *discordgo.Session
	channelID string
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

func (b *Bot) SendAlert(result checker.CheckResult) error {
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

// TODO ambas funções iguais a baixo, depois posso fazer uma com parâmetros, assim ficará melhor

func (b *Bot) handleStatus(s *discordgo.Session, i *discordgo.InteractionCreate, checkFn CheckerFunc) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     createStatusSummaryEmbed(checkFn()),
			Components: []discordgo.MessageComponent{createRecheckButton()},
		},
	})
	if err != nil {
		slog.Error("Erro ao responder comando /status", "error", err)
	}
}

func (b *Bot) handleRecheckButton(s *discordgo.Session, i *discordgo.InteractionCreate, checkFn CheckerFunc) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     createStatusSummaryEmbed(checkFn()),
			Components: []discordgo.MessageComponent{createRecheckButton()},
		},
	})
	if err != nil {
		slog.Error("Erro ao atualizar mensagem pelo botão", "error", err)
	}
}

package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/errs"
	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session *discordgo.Session
	configs *config.DiscordConfig
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

func New(cfgDiscord *config.DiscordConfig) (*Bot, error) {
	if err := validateRequiredVariables(cfgDiscord.Token, cfgDiscord.ChannelID); err != nil {
		return nil, err
	}

	// ATENÇÃO: Prefixo Bot é exigido antes do token pela documentação
	dg, err := discordgo.New("Bot " + cfgDiscord.Token)
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

	slog.Info("Conexão com Discord estabelecida!")

	return &Bot{
		session: dg,
		configs: cfgDiscord,
	}, nil
}

func (b *Bot) Setup(ctx context.Context, cfg config.MonitorConfig) (chan struct{}, error) {
	if err := b.RegisterCommands(); err != nil {
		return nil, err
	}

	// TODO: preciso melhorar a criação/registro dos handlers, ta muito acoplado com a função anônima
	triggerChan := make(chan struct{}, 1)
	b.RegisterHandlers(func() []checker.CheckResult {
		select {
		case triggerChan <- struct{}{}:
		default:
		}
		return checker.CheckAll(ctx, cfg.TargetURLs)
	})

	return triggerChan, nil
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
		b.configs.ChannelID,
		embed,
	)

	return err
}

func (b *Bot) RegisterCommands() error {
	_, err := b.session.ApplicationCommandBulkOverwrite(
		b.session.State.User.ID,
		b.configs.GuildID,
		commands,
	)
	if err != nil {
		slog.Error("Failed to register commands in discord app", "error", err)
		return err
	}

	slog.Info("Successfully registered all application commands!")
	return nil
}

func (b *Bot) responseWithStatus(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	respType discordgo.InteractionResponseType,
	checkFn CheckerFunc,
) {
	err := s.InteractionRespond(
		i.Interaction,
		&discordgo.InteractionResponse{
			Type: respType,
			Data: &discordgo.InteractionResponseData{
				Embeds:     createStatusSummaryEmbed(checkFn()),
				Components: []discordgo.MessageComponent{createRecheckButton()},
			},
		},
	)
	if err != nil {
		slog.Error("Erro ao responder interação do Discord", "error", err)
	}
}

func (b *Bot) handleStatus(s *discordgo.Session, i *discordgo.InteractionCreate, checkFn CheckerFunc) {
	b.responseWithStatus(
		s, i,
		discordgo.InteractionResponseChannelMessageWithSource,
		checkFn,
	)
}

func (b *Bot) handleRecheckButton(s *discordgo.Session, i *discordgo.InteractionCreate, checkFn CheckerFunc) {
	b.responseWithStatus(
		s, i,
		discordgo.InteractionResponseUpdateMessage,
		checkFn,
	)
}

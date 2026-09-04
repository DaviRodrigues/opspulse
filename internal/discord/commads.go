package discord

import (
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "status",
		Description: "Verifica a saúde atual de todos os serviços monitorados",
	},
}

func RegisterCommands(s *discordgo.Session, appID, guildID string) error {
	_, err := s.ApplicationCommandBulkOverwrite(appID, guildID, commands)
	if err != nil {
		slog.Error("Failed to register commands in discord app", "error", err)
		return err
	}

	slog.Info("Successfully registered all application commands!")
	return nil
}
package discord

import (
	"github.com/bwmarrin/discordgo"
)

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "status",
		Description: "Verifica a saúde atual de todos os serviços monitorados",
	},
}

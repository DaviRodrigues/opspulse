package discord

import (
	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/bwmarrin/discordgo"
)

type CheckerFunc func() []checker.CheckResult

func (b *Bot) RegisterHandlers(checkFn CheckerFunc) {
	b.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if i.ApplicationCommandData().Name == "status" {
				b.handleStatus(s, i, checkFn)
			}
		case discordgo.InteractionMessageComponent:
			if i.MessageComponentData().CustomID == "btn_recheck" {
				b.handleRecheckButton(s, i, checkFn)
			}
		}
	})
}
package config

import (
	"github.com/DaviRodrigues/opspulse/internal/file"
)

type DiscordConfig struct {
	Token     string
	ChannelID string
	GuildID   string
}

func LoadDiscordConfig(envManager file.EnvFile) DiscordConfig {
	token := envManager.LoadVariable("DISCORD_TOKEN", defaultFallback.Discord.Token)

	channelID := envManager.LoadVariable("DISCORD_CHANNEL_ID", defaultFallback.Discord.ChannelID)

	guildID := envManager.LoadVariable("DISCORD_GUILD_ID", defaultFallback.Discord.GuildID)

	return DiscordConfig{
		Token:     token,
		ChannelID: channelID,
		GuildID:   guildID,
	}
}

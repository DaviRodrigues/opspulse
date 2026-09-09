package config

import "errors"

type DiscordConfig struct {
	Token     string
	ChannelID string
	GuildID   string
}

func loadDiscordConfig() (DiscordConfig, error) {
	var err_s []error

	token, err := LoadVariable(
		"DISCORD_TOKEN",
		"",
	)
	if err != nil {
		err_s = append(err_s, err)
	}

	channelID, err := LoadVariable("DISCORD_CHANNEL_ID", "")
	if err != nil {
		err_s = append(err_s, err)
	}

	guildID, err := LoadVariable("DISCORD_GUILD_ID", "")
	if err != nil {
		err_s = append(err_s, err)
	}

	if len(err_s) > 0 {
        return DiscordConfig{}, errors.Join(err_s...)
    }

	return DiscordConfig{
		Token: token,
		ChannelID: channelID,
		GuildID: guildID,
	}, nil
}
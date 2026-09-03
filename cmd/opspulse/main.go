package main

import (
	"fmt"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/context"
//	"github.com/DaviRodrigues/opspulse/internal/discord"
)

func main() {
	ctx, stop := context.CreateNotifyContext()
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("%v", err)
	}

//	bot, err := discord.New(cfg.DiscordToken, cfg.DiscordChannelID)
//	if err != nil {
//		fmt.Printf("%v", err)
//	}
//	defer bot.Close()

	checker.StartMonitoring(ctx, nil, cfg.TargetURLs, cfg.CheckInterval, cfg.CheckTimeout)

}

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/discord"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("%v", err)
	}

	bot, err := discord.New(cfg.DiscordToken, cfg.DiscordChannelID)
	if err != nil {
		fmt.Printf("%v", err)
	}
	defer bot.Close()

	checker.StartMonitoring(ctx, bot, cfg.TargetURLs, cfg.CheckInterval, cfg.CheckTimeout)

}

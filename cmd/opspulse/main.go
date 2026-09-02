package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/checker"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	urls := []string{
		"https://github.com",
		"https://httpbin.org/status/200",
		"https://httpbin.org/status/500",
	}

	fmt.Println("OpsPulse iniciado! Pressione Ctrl+C para encerrar.")

	checker.StartMonitoring(ctx, urls, 5*time.Second, 3*time.Second)

	fmt.Println("Aplicação finalizada com sucesso.")
}

package main

import (
	"fmt"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/checker"
)

func main() {
	result := checker.CheckURL(
		"https://github.com",
		5*time.Second,
	)
	fmt.Printf("URL: %s | Status: %d | Latência: %v | Online: %t\n", result.URL, result.StatusCode, result.Latency, result.IsUp)
}

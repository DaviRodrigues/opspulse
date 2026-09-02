package checker

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/models"
)

func CheckURL(url string, timeout time.Duration) models.CheckResult {
	start := time.Now()
	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		return models.CheckResult{
			IsUp:       false,
			Error:      err,
			Latency:    time.Since(start),
			URL:        url,
			StatusCode: 0,
		}
	}

	defer resp.Body.Close()
	return models.CheckResult{
		URL:        url,
		IsUp:       resp.StatusCode >= 200 && resp.StatusCode < 400,
		StatusCode: resp.StatusCode,
		Latency:    time.Since(start),
		Error:      nil,
	}
}

func CheckAll(urls []string, timeout time.Duration) []models.CheckResult {
	var wg sync.WaitGroup

	resultsChan := make(chan models.CheckResult, len(urls))

	for _, url := range urls {
		wg.Add(1)

		go func(u string) {
			defer wg.Done()
			// time.Sleep(100 * time.Millisecond)
			resultsChan <- CheckURL(u, timeout)
		}(url)
	}

	wg.Wait()

	close(resultsChan)

	var results []models.CheckResult
	for res := range resultsChan {
		results = append(results, res)
	}

	return results
}

func StartMonitoring(ctx context.Context, urls []string, interval, timeout time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	results := CheckAll(urls, timeout)
	printResults(&results)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Encerrando monitoramento de forma segura")
			return
		case <-ticker.C:
			results := CheckAll(urls, timeout)
			printResults(&results)
		}
	}
}

func printResults(results *[]models.CheckResult) {
	fmt.Printf("\n--- Relatório de Saúde [%s] ---\n",
		time.Now().Format("15:04:05"))
	for _, res := range *results {
		status := "UP"
		if !res.IsUp {
			status = "DOWN"
		}

		fmt.Printf("[%s] %s | Código: %d | Latência: %v\n",
			status, res.URL, res.StatusCode, res.Latency)
	}
}

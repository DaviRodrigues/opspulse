package checker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/errs"
)

/*
TODO: validar depois formas de enviar notificação por outros serviços email, slack e etc..
Além disso, valdar de pegar outras informações fora o básico do healthcheck: headers, security, body e validar
serviços tipo banco de dados etc...
Fazer uma forma de ter um checker pra UP constante ou de tempos em tempos altos, o DOWN ainda é o mais importante
*/

type CheckResult struct {
	URL string
	StatusCode int
	Latency time.Duration
	IsUp bool
	Error error
}

type Notifier interface {
	SendAlert(result CheckResult) error
}

func checkURL(ctx context.Context, url string, timeout time.Duration) CheckResult {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return CheckResult{
			IsUp:       false,
			Error:      fmt.Errorf("%w (more info: %w)", errs.ErrServiceDown, err),
			Latency:    0,
			URL:        url,
			StatusCode: 0,
		}
	}

	start := time.Now()
	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{
			IsUp:       false,
			Error:      fmt.Errorf("%w (more info: %w)", errs.ErrServiceDown, err),
			Latency:    time.Since(start),
			URL:        url,
			StatusCode: 0,
		}
	}
	defer resp.Body.Close()

	return CheckResult{
		URL:        url,
		IsUp:       resp.StatusCode >= 200 && resp.StatusCode < 400,
		StatusCode: resp.StatusCode,
		Latency:    time.Since(start),
		Error:      nil,
	}
}

func CheckAll(ctx context.Context, urls []string, timeout time.Duration) []CheckResult {
	var wg sync.WaitGroup

	resultsChan := make(chan CheckResult, len(urls))

	for _, url := range urls {
		wg.Add(1)

		go func(u string) {
			defer wg.Done()
			// time.Sleep(100 * time.Millisecond)
			resultsChan <- checkURL(ctx, u, timeout)
		}(url)
	}

	wg.Wait()

	close(resultsChan)

	var results []CheckResult
	for res := range resultsChan {
		results = append(results, res)
	}

	return results
}

func StartMonitoring(ctx context.Context, ntf Notifier, triggerChan <-chan struct{}, cfg config.Config) {
	ticker := time.NewTicker(cfg.Monitor.Interval)
	defer ticker.Stop()

	results := CheckAll(ctx, cfg.Monitor.TargetURLs, cfg.Monitor.Timeout)
	printResults(results)
	notifierProcess(ntf, results)

	for {
		select {
		case <-ctx.Done():
			slog.Info("🛑 Encerrando monitoramento de forma segura")
			return
		case <-ticker.C:
			results := CheckAll(ctx, cfg.Monitor.TargetURLs, cfg.Monitor.Timeout)
			printResults(results)
			notifierProcess(ntf, results)
		case <-triggerChan:
			ticker.Reset(cfg.Monitor.Interval)
			slog.Info("🔄 Intervalo de monitoramento reiniciado por comando externo")
		}
	}
}

func notifierProcess(ntf Notifier, results []CheckResult) {
	if ntf == nil {
		return
	}

	for _, res := range results {
		if !res.IsUp {
			if err := ntf.SendAlert(res); err != nil {
				slog.Error("Falha ao enviar alerta para o notificador",
					"url", res.URL,
					"error", err,
				)
			}
		}
	}
}

func printResults(results []CheckResult) {
	fmt.Printf("\n--- Relatório de Saúde [%s] ---\n",
		time.Now().Format("15:04:05"))
	for _, res := range results {
		if res.IsUp {
			slog.Info("Serviço operacional",
				"status", "UP",
				"url", res.URL,
				"code", res.StatusCode,
				"latency", res.Latency.String(),
			)
		} else {
			slog.Warn("Serviço com problemas",
				"status", "DOWN",
				"url", res.URL,
				"code", res.StatusCode,
				"latency", res.Latency.String(),
				"error", res.Error,
			)
		}
	}
}

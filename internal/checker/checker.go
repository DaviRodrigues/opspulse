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
	"github.com/DaviRodrigues/opspulse/internal/file"
)

/*
TODO: validar depois formas de enviar notificação por outros serviços email, slack e etc..
Além disso, valdar de pegar outras informações fora o básico do healthcheck: headers, security, body e validar
serviços tipo banco de dados etc...
Fazer uma forma de ter um checker pra UP constante ou de tempos em tempos altos, o DOWN ainda é o mais importante
*/

type CheckResult struct {
	URL        string
	StatusCode int
	Latency    time.Duration
	IsUp       bool
	Error      error
}

// TODO: guardar para mais tarde no padrão strategy
type HTTPChecker struct {}
type TCPChecker struct {}
type SSLChecker struct {}

type Notifier interface {
	SendAlert(result CheckResult) error
}

type ServiceChecker interface {
	Check(ctx context.Context, t file.Target)
}

func checkURL(ctx context.Context, target file.Target) CheckResult {
	reqCtx, cancel := context.WithTimeout(ctx, target.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodGet,
		target.URL,
		nil,
	)
	if err != nil {
		return CheckResult{
			IsUp:       false,
			Error:      fmt.Errorf("%w (more info: %w)", errs.ErrServiceDown, err),
			Latency:    0,
			URL:        target.URL,
			StatusCode: 0,
		}
	}

	start := time.Now()
	client := &http.Client{Timeout: target.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{
			IsUp:       false,
			Error:      fmt.Errorf("%w (more info: %w)", errs.ErrServiceDown, err),
			Latency:    time.Since(start),
			URL:        target.URL,
			StatusCode: 0,
		}
	}
	defer resp.Body.Close()

	return CheckResult{
		URL:        target.URL,
		IsUp:       resp.StatusCode >= 200 && resp.StatusCode < 400,
		StatusCode: resp.StatusCode,
		Latency:    time.Since(start),
		Error:      nil,
	}
}

func CheckAll(ctx context.Context, targets []file.Target) []CheckResult {
	var wg sync.WaitGroup
	resultsChan := make(chan CheckResult, len(targets))

	for _, target := range targets {
		wg.Add(1)

		go func(t file.Target) {
			defer wg.Done()
			if t.Enabled {
				resultsChan <- checkURL(ctx, t)
			}
		}(target)
	}

	wg.Wait()
	close(resultsChan)

	var results []CheckResult
	for res := range resultsChan {
		results = append(results, res)
	}

	return results
}

func StartMonitoring(
	ctx context.Context,
	ntf Notifier,
	triggerChan <-chan struct{},
	cfg config.MonitorConfig,
) {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	processMonitor(ctx, ntf, cfg)

	for {
		select {
		case <-ctx.Done():
			slog.Info("🛑 Encerrando monitoramento de forma segura")
			return
		case <-ticker.C:
			processMonitor(ctx, ntf, cfg)
		case <-triggerChan:
			ticker.Reset(cfg.Interval)
			slog.Info("🔄 Intervalo de monitoramento reiniciado por comando externo")
		}
	}
}

func processMonitor(ctx context.Context, ntf Notifier, cfg config.MonitorConfig) {
	results := CheckAll(ctx, cfg.TargetURLs)
	printResults(results)
	notifierProcess(ntf, results)
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
				"status", "🟢 UP",
				"url", res.URL,
				"code", res.StatusCode,
				"latency", res.Latency.String(),
			)
		} else {
			slog.Warn("Serviço com problemas",
				"status", "🔴 DOWN",
				"url", res.URL,
				"code", res.StatusCode,
				"latency", res.Latency.String(),
				"error", res.Error,
			)
		}
	}
}

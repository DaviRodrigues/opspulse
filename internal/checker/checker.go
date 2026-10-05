package checker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/domain"
	"github.com/DaviRodrigues/opspulse/internal/errs"
)

// TODO: guardar para mais tarde no padrão strategy
type HTTPChecker struct{}
type TCPChecker struct{}
type SSLChecker struct{}

type Notifier interface {
	SendAlert(result domain.CheckLogs) error
}

type ServiceChecker interface {
	Check(ctx context.Context, t domain.Target)
}

func checkURL(ctx context.Context, target domain.Target) domain.CheckLogs {
	reqCtx, cancel := context.WithTimeout(ctx, target.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodGet,
		target.URL,
		nil,
	)
	if err != nil {
		slog.Debug(fmt.Sprintf("%s (more info: %s)", errs.ErrServiceDown.Error(), err.Error()))
		return domain.CheckLogs{
			Name:  target.Name,
			IsUp:  false,
			Error: fmt.Sprintf("%s (more info: %s)", errs.ErrServiceDown.Error(), err.Error()),
			URL:   target.URL,
		}
	}

	start := time.Now()
	client := &http.Client{Timeout: target.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug(fmt.Sprintf("%s (more info: %s)", errs.ErrServiceDown.Error(), err.Error()))
		return domain.CheckLogs{
			Name:    target.Name,
			IsUp:    false,
			Error:   fmt.Sprintf("%s (more info: %s)", errs.ErrServiceDown.Error(), err.Error()),
			Latency: time.Since(start),
			URL:     target.URL,
		}
	}
	defer resp.Body.Close()

	isUp := false
	if len(target.ExpectedStatus) > 0 {
		isUp = slices.Contains(target.ExpectedStatus, resp.StatusCode)
	} else {
		isUp = resp.StatusCode >= 200 && resp.StatusCode < 400
	}

	var errMsg string
	if !isUp {
		errMsg = fmt.Sprintf("status HTTP inesperado: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return domain.CheckLogs{
		Name:       target.Name,
		URL:        target.URL,
		IsUp:       isUp,
		StatusCode: resp.StatusCode,
		Latency:    time.Since(start),
		Error:      errMsg,
	}
}

func CheckAll(ctx context.Context, targets []domain.Target) []domain.CheckLogs {
	var wg sync.WaitGroup
	resultsChan := make(chan domain.CheckLogs, len(targets))

	for _, target := range targets {
		wg.Go(func() {
			if target.Enabled {
				resultsChan <- checkURL(ctx, target)
			}
		})
	}

	wg.Wait()
	close(resultsChan)

	var results []domain.CheckLogs
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

func notifierProcess(ntf Notifier, results []domain.CheckLogs) {
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

func printResults(results []domain.CheckLogs) {
	fmt.Printf("\n--- Relatório de Saúde [%s] ---\n",
		time.Now().Format("15:04:05"))
	for _, res := range results {
		if res.IsUp {
			slog.Info("Serviço operacional",
				"status", "🟢 UP",
				"name", res.Name,
				"url", res.URL,
				"code", res.StatusCode,
				"latency", res.Latency.String(),
			)
		} else {
			slog.Warn("Serviço com problemas",
				"status", "🔴 DOWN",
				"name", res.Name,
				"url", res.URL,
				"code", res.StatusCode,
				"latency", res.Latency.String(),
				"error", res.Error,
			)
		}
	}
}

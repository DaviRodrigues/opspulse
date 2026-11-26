package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/domain"
)

const (
	defaultJitterRatio   = 0.20
	defaultCheckTimeout  = 5 * time.Second
	defaultMonitorPeriod = 30 * time.Second
	maxTargetStagger     = 25 * time.Millisecond
	maxResponseBodyBytes = 1 << 20 // 1MB
)

var (
	defaultHTTPTransport = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	defaultHTTPClient = &http.Client{
		Transport: defaultHTTPTransport,
	}
)

// TODO: guardar para mais tarde no padrão strategy
type HTTPChecker struct{}
type TCPChecker struct{}
type SSLChecker struct{}

type Notifier interface {
	SendAlert(result domain.CheckResult) error
}

type ServiceChecker interface {
	Check(ctx context.Context, t domain.Target)
}

type checkOutcome struct {
	result domain.CheckResult
	log    domain.CheckLog
}

// ApplyJitter applies stochastic jitter within [1-ratio, 1+ratio] using math/rand/v2.
func ApplyJitter(base time.Duration, ratio float64) time.Duration {
	if base <= 0 {
		return 0
	}
	if math.IsNaN(ratio) || ratio <= 0 {
		if base < time.Millisecond {
			return time.Millisecond
		}
		return base
	}
	if math.IsInf(ratio, 1) || ratio >= 1 {
		ratio = 0.99
	}

	factor := (1.0 - ratio) + rand.Float64()*(2.0*ratio)
	scaled := float64(base) * factor
	if scaled >= float64(math.MaxInt64) {
		return time.Duration(math.MaxInt64)
	}
	jittered := time.Duration(scaled)
	if jittered < time.Millisecond {
		return time.Millisecond
	}
	return jittered
}

// JitterDuration applies ±20% stochastic jitter around base.
func JitterDuration(base time.Duration) time.Duration {
	return ApplyJitter(base, defaultJitterRatio)
}

func resolveTimeout(targetTimeout, defaultTimeout time.Duration) time.Duration {
	if targetTimeout > 0 {
		return targetTimeout
	}
	if defaultTimeout > 0 {
		return defaultTimeout
	}
	return defaultCheckTimeout
}

func ResolveInterval(targetInterval, defaultInterval time.Duration) time.Duration {
	if targetInterval > 0 {
		return targetInterval
	}
	if defaultInterval > 0 {
		return defaultInterval
	}
	return defaultMonitorPeriod
}

func resolveMethod(raw string) string {
	method := strings.ToUpper(strings.TrimSpace(raw))
	if method == "" {
		return http.MethodGet
	}
	return method
}

func methodAllowsBody(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
}

func CheckTarget(
	ctx context.Context,
	client *http.Client,
	target domain.Target,
	defaultTimeout time.Duration,
) (domain.CheckResult, domain.CheckLog) {
	if client == nil {
		client = defaultHTTPClient
	}

	timeout := resolveTimeout(target.Timeout, defaultTimeout)
	method := resolveMethod(target.Method)
	checkedAt := time.Now().UTC()

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var bodyReader io.Reader
	hasJSONBody := false
	if len(target.Body) > 0 && methodAllowsBody(method) {
		payload, err := json.Marshal(target.Body)
		if err != nil {
			errMsg := fmt.Sprintf("%s (more info: %s)", domain.ErrServiceDown.Error(), err.Error())
			res := domain.CheckResult{
				Name:  target.Name,
				URL:   target.URL,
				IsUp:  false,
				Error: errMsg,
			}
			return res, domain.CheckLog{
				TargetID:  target.ID,
				Name:      target.Name,
				URL:       target.URL,
				IsUp:      false,
				Error:     errMsg,
				CheckedAt: checkedAt,
			}
		}
		bodyReader = bytes.NewReader(payload)
		hasJSONBody = true
	}

	req, err := http.NewRequestWithContext(reqCtx, method, target.URL, bodyReader)
	if err != nil {
		slog.Debug(fmt.Sprintf("%s (more info: %s)", domain.ErrServiceDown.Error(), err.Error()))
		errMsg := fmt.Sprintf("%s (more info: %s)", domain.ErrServiceDown.Error(), err.Error())
		res := domain.CheckResult{
			Name:  target.Name,
			URL:   target.URL,
			IsUp:  false,
			Error: errMsg,
		}
		return res, domain.CheckLog{
			TargetID:  target.ID,
			Name:      target.Name,
			URL:       target.URL,
			IsUp:      false,
			Error:     errMsg,
			CheckedAt: checkedAt,
		}
	}

	if hasJSONBody {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range target.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		slog.Debug(fmt.Sprintf("%s (more info: %s)", domain.ErrServiceDown.Error(), err.Error()))
		errMsg := fmt.Sprintf("%s (more info: %s)", domain.ErrServiceDown.Error(), err.Error())
		res := domain.CheckResult{
			Name:    target.Name,
			URL:     target.URL,
			IsUp:    false,
			Latency: latency,
			Error:   errMsg,
		}
		return res, domain.CheckLog{
			TargetID:  target.ID,
			Name:      target.Name,
			URL:       target.URL,
			Latency:   latency,
			IsUp:      false,
			Error:     errMsg,
			CheckedAt: time.Now().UTC(),
		}
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	isUp := false
	if len(target.ExpectedStatus) > 0 {
		isUp = slices.Contains(target.ExpectedStatus, resp.StatusCode)
	} else {
		isUp = resp.StatusCode >= 200 && resp.StatusCode < 400
	}

	var errMsg string
	if !isUp {
		errMsg = fmt.Sprintf("status HTTP inesperado: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
		}
	} else if target.ExpectedBodyMatch != "" {
		if resp.Body == nil {
			isUp = false
			errMsg = fmt.Sprintf("corpo da resposta vazio para padrão esperado: %q", target.ExpectedBodyMatch)
		} else {
			bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
			if readErr != nil {
				isUp = false
				errMsg = fmt.Sprintf("falha ao ler corpo da resposta: %v", readErr)
			} else {
				re, regexErr := regexp.Compile(target.ExpectedBodyMatch)
				if regexErr != nil {
					isUp = false
					errMsg = fmt.Sprintf("regex inválida em expected_body_match: %v", regexErr)
				} else if !re.Match(bodyBytes) && !strings.Contains(string(bodyBytes), target.ExpectedBodyMatch) {
					isUp = false
					errMsg = fmt.Sprintf("corpo da resposta não corresponde ao padrão esperado: %q", target.ExpectedBodyMatch)
				}
			}
		}
	} else if resp.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
	}

	res := domain.CheckResult{
		Name:       target.Name,
		URL:        target.URL,
		IsUp:       isUp,
		StatusCode: resp.StatusCode,
		Latency:    latency,
		Error:      errMsg,
	}
	logEntry := domain.CheckLog{
		TargetID:   target.ID,
		Name:       target.Name,
		URL:        target.URL,
		StatusCode: resp.StatusCode,
		Latency:    latency,
		IsUp:       isUp,
		Error:      errMsg,
		CheckedAt:  time.Now().UTC(),
	}

	return res, logEntry
}

func CheckTargets(
	ctx context.Context,
	targets []domain.Target,
	defaultTimeout time.Duration,
) ([]domain.CheckResult, []domain.CheckLog) {
	enabled := make([]domain.Target, 0, len(targets))
	for _, t := range targets {
		if t.Enabled {
			enabled = append(enabled, t)
		}
	}

	if len(enabled) == 0 {
		return make([]domain.CheckResult, 0), make([]domain.CheckLog, 0)
	}

	var wg sync.WaitGroup
	outcomesChan := make(chan checkOutcome, len(enabled))
	shouldStagger := len(enabled) > 1

	for _, target := range enabled {
		var staggerDelay time.Duration
		if shouldStagger {
			staggerDelay = time.Duration(rand.Int64N(int64(maxTargetStagger)))
		}

		wg.Go(func() {
			if staggerDelay > 0 {
				timer := time.NewTimer(staggerDelay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			} else if ctx.Err() != nil {
				return
			}

			res, logEntry := CheckTarget(ctx, defaultHTTPClient, target, defaultTimeout)
			outcomesChan <- checkOutcome{
				result: res,
				log:    logEntry,
			}
		})
	}

	wg.Wait()
	close(outcomesChan)

	results := make([]domain.CheckResult, 0, len(enabled))
	logs := make([]domain.CheckLog, 0, len(enabled))
	for out := range outcomesChan {
		results = append(results, out.result)
		logs = append(logs, out.log)
	}

	return results, logs
}

func CheckAll(ctx context.Context, targets []domain.Target) []domain.CheckResult {
	results, _ := CheckTargets(ctx, targets, defaultCheckTimeout)
	return results
}

// SchedulerHooks defines optional callbacks invoked by RunTargetScheduler.
type SchedulerHooks struct {
	OnResult   func(result domain.CheckResult, snapshot []domain.CheckResult)
	OnSnapshot func(snapshot []domain.CheckResult)
}

type scheduledWorker struct {
	target domain.Target
	cancel context.CancelFunc
}

func targetWorkerKey(t domain.Target) string {
	if !t.ID.IsZero() {
		return t.ID.Hex()
	}
	return t.URL
}

func sameTargetScheduleConfig(a, b domain.Target) bool {
	return a.Name == b.Name &&
		a.URL == b.URL &&
		a.Method == b.Method &&
		a.Enabled == b.Enabled &&
		a.Timeout == b.Timeout &&
		a.Interval == b.Interval &&
		a.ExpectedBodyMatch == b.ExpectedBodyMatch &&
		slices.Equal(a.ExpectedStatus, b.ExpectedStatus) &&
		reflect.DeepEqual(a.Headers, b.Headers) &&
		reflect.DeepEqual(a.Body, b.Body)
}

func resolveReconcilePeriod(defaultInterval time.Duration) time.Duration {
	const maxReconcile = 5 * time.Second
	if defaultInterval <= 0 {
		return maxReconcile
	}
	if defaultInterval < maxReconcile {
		return defaultInterval
	}
	return maxReconcile
}

// RunTargetScheduler runs an independent jittered loop for each enabled target,
// using each target's own Interval (falling back to cfg.Interval) and Timeout
// (falling back to cfg.DefaultTimeout).
func RunTargetScheduler(
	ctx context.Context,
	targetRepo database.TargetRepository,
	checkLogRepo database.CheckLogRepository,
	triggerChan <-chan struct{},
	cfg config.MonitorConfig,
	hooks SchedulerHooks,
) {
	defaultInterval := cfg.Interval
	if defaultInterval <= 0 {
		defaultInterval = defaultMonitorPeriod
	}

	defaultTimeout := cfg.DefaultTimeout
	if defaultTimeout <= 0 {
		defaultTimeout = defaultCheckTimeout
	}

	var (
		mu       sync.Mutex
		workers  = make(map[string]*scheduledWorker)
		latest   = make(map[string]domain.CheckResult)
		workerWG sync.WaitGroup
	)

	buildSnapshotLocked := func() []domain.CheckResult {
		if len(latest) == 0 {
			return []domain.CheckResult{}
		}
		keys := make([]string, 0, len(latest))
		for k := range latest {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]domain.CheckResult, 0, len(keys))
		for _, k := range keys {
			out = append(out, latest[k])
		}
		return out
	}

	startWorker := func(key string, target domain.Target, initialStagger time.Duration) *scheduledWorker {
		wCtx, cancel := context.WithCancel(ctx)
		w := &scheduledWorker{
			target: target,
			cancel: cancel,
		}
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			if initialStagger > 0 {
				staggerTimer := time.NewTimer(initialStagger)
				select {
				case <-wCtx.Done():
					staggerTimer.Stop()
					return
				case <-staggerTimer.C:
				}
			}

			effectiveInterval := ResolveInterval(target.Interval, defaultInterval)

			runOnce := func() {
				if wCtx.Err() != nil {
					return
				}
				res, logEntry := CheckTarget(wCtx, defaultHTTPClient, target, defaultTimeout)
				if wCtx.Err() != nil {
					return
				}

				if checkLogRepo != nil {
					if err := checkLogRepo.Create(wCtx, &logEntry); err != nil && wCtx.Err() == nil {
						slog.Error("Falha ao persistir check_log", "target", target.Name, "error", err)
					}
				}

				mu.Lock()
				if cur, ok := workers[key]; !ok || cur != w {
					mu.Unlock()
					return
				}
				latest[key] = res
				snapshot := buildSnapshotLocked()
				mu.Unlock()

				if hooks.OnResult != nil {
					hooks.OnResult(res, snapshot)
				}
			}

			runOnce()

			timer := time.NewTimer(JitterDuration(effectiveInterval))
			defer timer.Stop()

			for {
				select {
				case <-wCtx.Done():
					return
				case <-timer.C:
					runOnce()
					timer.Reset(JitterDuration(effectiveInterval))
				}
			}
		}()
		return w
	}

	reconcile := func() {
		if targetRepo == nil {
			slog.Warn("TargetRepository não configurado; ciclo de monitoramento ignorado")
			return
		}

		targetPtrs, err := targetRepo.FindEnabled(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("Falha ao buscar alvos habilitados no repositório", "error", err)
			}
			return
		}

		desired := make(map[string]domain.Target, len(targetPtrs))
		for _, t := range targetPtrs {
			if t != nil && t.Enabled {
				desired[targetWorkerKey(*t)] = *t
			}
		}

		var (
			snapshotToEmit []domain.CheckResult
			emitSnapshot   bool
		)

		mu.Lock()
		// Stop workers for removed or disabled targets
		for key, w := range workers {
			if _, ok := desired[key]; !ok {
				w.cancel()
				delete(workers, key)
				if _, hadLatest := latest[key]; hadLatest {
					delete(latest, key)
					emitSnapshot = true
				}
			}
		}

		// Start new workers or restart modified ones
		newCount := 0
		for key, target := range desired {
			existing, ok := workers[key]
			if ok && sameTargetScheduleConfig(existing.target, target) {
				continue
			}
			if ok {
				existing.cancel()
			}
			var stagger time.Duration
			if len(desired) > 1 && newCount > 0 {
				stagger = time.Duration(rand.Int64N(int64(maxTargetStagger)))
			}
			workers[key] = startWorker(key, target, stagger)
			newCount++
		}

		if len(desired) == 0 && len(latest) == 0 && emitSnapshot {
			snapshotToEmit = []domain.CheckResult{}
		} else if emitSnapshot {
			snapshotToEmit = buildSnapshotLocked()
		}
		mu.Unlock()

		if emitSnapshot && hooks.OnSnapshot != nil {
			hooks.OnSnapshot(snapshotToEmit)
		}
	}

	reconcile()

	reconcilePeriod := resolveReconcilePeriod(defaultInterval)
	reconcileTicker := time.NewTicker(reconcilePeriod)
	defer reconcileTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, w := range workers {
				w.cancel()
			}
			mu.Unlock()
			workerWG.Wait()
			return
		case <-reconcileTicker.C:
			reconcile()
		case <-triggerChan:
			reconcile()
		}
	}
}

func StartMonitoring(
	ctx context.Context,
	targetRepo database.TargetRepository,
	checkLogRepo database.CheckLogRepository,
	ntf Notifier,
	triggerChan <-chan struct{},
	cfg config.MonitorConfig,
) {
	RunTargetScheduler(ctx, targetRepo, checkLogRepo, triggerChan, cfg, SchedulerHooks{
		OnResult: func(result domain.CheckResult, _ []domain.CheckResult) {
			results := []domain.CheckResult{result}
			printResults(results)
			notifierProcess(ntf, results)
		},
	})
	slog.Info("🛑 Encerrando monitoramento de forma segura")
}

func notifierProcess(ntf Notifier, results []domain.CheckResult) {
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

func printResults(results []domain.CheckResult) {
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

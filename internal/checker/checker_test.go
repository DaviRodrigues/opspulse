package checker

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type mockNotifier struct {
	mu     sync.Mutex
	alerts []domain.CheckResult
}

func (m *mockNotifier) SendAlert(result domain.CheckResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts = append(m.alerts, result)
	return nil
}

func (m *mockNotifier) Alerts() []domain.CheckResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.CheckResult, len(m.alerts))
	copy(out, m.alerts)
	return out
}

func TestApplyJitterAndJitterDuration(t *testing.T) {
	if got := ApplyJitter(0, 0.20); got != 0 {
		t.Errorf("expected 0 for base=0, got %v", got)
	}
	if got := ApplyJitter(-5*time.Second, 0.20); got != 0 {
		t.Errorf("expected 0 for negative base, got %v", got)
	}
	if got := JitterDuration(0); got != 0 {
		t.Errorf("expected 0 for JitterDuration(0), got %v", got)
	}

	// Tiny positive base respects 1ms floor
	tiny := ApplyJitter(500*time.Microsecond, 0.20)
	if tiny < time.Millisecond {
		t.Errorf("expected at least 1ms floor for positive base, got %v", tiny)
	}

	base := 10 * time.Second
	minAllowed := time.Duration(float64(base) * 0.80)
	maxAllowed := time.Duration(float64(base) * 1.20)

	seen := make(map[time.Duration]struct{})
	for i := 0; i < 100; i++ {
		d := JitterDuration(base)
		if d < minAllowed || d > maxAllowed {
			t.Fatalf("JitterDuration(%v) = %v out of bounds [%v, %v]", base, d, minAllowed, maxAllowed)
		}
		seen[d] = struct{}{}
	}
	if len(seen) < 5 {
		t.Errorf("expected stochastic variance across 100 calls, only got %d unique durations", len(seen))
	}
}

func TestApplyJitter_ExtremeDurationAndNaNRatio(t *testing.T) {
	maxBase := time.Duration(math.MaxInt64)
	minExpected := time.Duration(float64(maxBase) * 0.80)

	for i := 0; i < 100; i++ {
		d := ApplyJitter(maxBase, 0.20)
		if d < minExpected || d > maxBase {
			t.Fatalf("ApplyJitter(MaxInt64, 0.20) = %v out of bounds [%v, %v]", d, minExpected, maxBase)
		}
	}

	if got := ApplyJitter(time.Second, math.NaN()); got != time.Second {
		t.Errorf("expected ApplyJitter(1s, NaN) == 1s, got %v", got)
	}

	if got := ApplyJitter(time.Second, math.Inf(-1)); got != time.Second {
		t.Errorf("expected ApplyJitter(1s, -Inf) == 1s, got %v", got)
	}

	for i := 0; i < 20; i++ {
		d := ApplyJitter(time.Second, math.Inf(1))
		if d < time.Millisecond || d > 2*time.Second {
			t.Fatalf("ApplyJitter(1s, +Inf) = %v out of expected clamped range", d)
		}
	}
}

func TestCheckTarget_KeepAliveConnectionReuseWhenBodyUnchecked(t *testing.T) {
	var newConns atomic.Int32
	chunk := strings.Repeat("A", 16*1024)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 4; i++ {
			_, _ = w.Write([]byte(chunk))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConns.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        4,
			MaxIdleConnsPerHost: 4,
		},
	}
	defer client.CloseIdleConnections()

	target := domain.Target{
		Name:    "KeepAlive Target",
		URL:     srv.URL,
		Enabled: true,
	}

	for i := 0; i < 5; i++ {
		res, _ := CheckTarget(context.Background(), client, target, 2*time.Second)
		if !res.IsUp {
			t.Fatalf("iteration %d: expected UP, got %+v", i, res)
		}
	}

	if got := newConns.Load(); got != 1 {
		t.Errorf("expected exactly 1 TCP connection reused across 5 checks, got %d new connections", got)
	}
}

func TestCheckTarget_RichHTTPMethodsHeadersBodyAndMatch(t *testing.T) {
	t.Run("POST with custom headers, JSON body, ExpectedStatus 201 and regex body match", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.Header.Get("X-OpsPulse-Key") != "secret-123" {
				t.Errorf("expected custom header X-OpsPulse-Key=secret-123, got %q", r.Header.Get("X-OpsPulse-Key"))
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected default Content-Type=application/json, got %q", r.Header.Get("Content-Type"))
			}
			rawBody, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			if err := json.Unmarshal(rawBody, &parsed); err != nil {
				t.Fatalf("expected valid JSON body, got %q (%v)", string(rawBody), err)
			}
			if parsed["probe"] != "ok" {
				t.Errorf("expected probe=ok in body, got %v", parsed["probe"])
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"healthy","version":"1.2.3"}`))
		}))
		defer srv.Close()

		targetID := bson.NewObjectID()
		target := domain.Target{
			ID:                targetID,
			Name:              "Ingest API",
			URL:               srv.URL,
			Method:            "post",
			Headers:           map[string]string{"X-OpsPulse-Key": "secret-123"},
			Body:              map[string]any{"probe": "ok"},
			Enabled:           true,
			ExpectedStatus:    []int{http.StatusCreated},
			ExpectedBodyMatch: `"status":\s*"healthy"`,
		}

		res, logEntry := CheckTarget(context.Background(), nil, target, 2*time.Second)
		if !res.IsUp || res.StatusCode != http.StatusCreated {
			t.Fatalf("expected UP with 201, got %+v", res)
		}
		if logEntry.TargetID != targetID || !logEntry.IsUp || logEntry.CheckedAt.IsZero() {
			t.Fatalf("unexpected CheckLog entry: %+v", logEntry)
		}
	})

	t.Run("ExpectedBodyMatch mismatch marks DOWN", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"degraded"}`))
		}))
		defer srv.Close()

		res, _ := CheckTarget(context.Background(), nil, domain.Target{
			Name:              "Degraded API",
			URL:               srv.URL,
			Enabled:           true,
			ExpectedBodyMatch: "healthy",
		}, 2*time.Second)

		if res.IsUp {
			t.Errorf("expected DOWN when ExpectedBodyMatch does not match")
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected StatusCode=200, got %d", res.StatusCode)
		}
		if res.Error == "" {
			t.Errorf("expected descriptive error on body mismatch")
		}
	})

	t.Run("Invalid regex in ExpectedBodyMatch fails safely without panic", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		defer srv.Close()

		for _, pat := range []string{"[unclosed-regex", "[OK"} {
			res, _ := CheckTarget(context.Background(), nil, domain.Target{
				Name:              "Invalid Regex",
				URL:               srv.URL,
				Enabled:           true,
				ExpectedBodyMatch: pat,
			}, 2*time.Second)

			if res.IsUp {
				t.Errorf("expected DOWN on invalid regex %q", pat)
			}
			if !strings.Contains(strings.ToLower(res.Error), "regex") {
				t.Errorf("expected error mentioning regex for %q, got %q", pat, res.Error)
			}
		}
	})

	t.Run("Case-insensitive regex and empty body edge cases", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/empty" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Service HEALTHY"))
		}))
		defer srv.Close()

		resOK, _ := CheckTarget(context.Background(), nil, domain.Target{
			Name:              "CaseInsensitive",
			URL:               srv.URL + "/ok",
			Enabled:           true,
			ExpectedBodyMatch: "(?i)healthy",
		}, 2*time.Second)
		if !resOK.IsUp {
			t.Errorf("expected UP for (?i)healthy matching 'Service HEALTHY', got %+v", resOK)
		}

		resEmpty, _ := CheckTarget(context.Background(), nil, domain.Target{
			Name:              "EmptyBody",
			URL:               srv.URL + "/empty",
			Enabled:           true,
			ExpectedBodyMatch: "healthy",
		}, 2*time.Second)
		if resEmpty.IsUp {
			t.Errorf("expected DOWN when response body is empty and ExpectedBodyMatch='healthy'")
		}
	})

	t.Run("Timeout respected when server hangs", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(300 * time.Millisecond):
				w.WriteHeader(http.StatusOK)
			case <-r.Context().Done():
			}
		}))
		defer srv.Close()

		res, _ := CheckTarget(context.Background(), nil, domain.Target{
			Name:    "Slow Service",
			URL:     srv.URL,
			Enabled: true,
			Timeout: 40 * time.Millisecond,
		}, 2*time.Second)

		if res.IsUp {
			t.Errorf("expected timeout to mark service DOWN")
		}
	})
}

func TestCheckAll_AndEmptyDisabledHandling(t *testing.T) {
	empty := CheckAll(context.Background(), nil)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("expected non-nil empty slice for nil targets, got %#v", empty)
	}

	disabledOnly := CheckAll(context.Background(), []domain.Target{
		{Name: "Disabled", URL: "http://127.0.0.1:1", Enabled: false},
	})
	if disabledOnly == nil || len(disabledOnly) != 0 {
		t.Fatalf("expected non-nil empty slice when all targets disabled, got %#v", disabledOnly)
	}

	srvOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srvOK.Close()

	srvErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srvErr.Close()

	targets := []domain.Target{
		{ID: bson.NewObjectID(), Name: "OK", URL: srvOK.URL, Enabled: true, Timeout: 2 * time.Second},
		{ID: bson.NewObjectID(), Name: "Err", URL: srvErr.URL, Enabled: true, Timeout: 2 * time.Second},
		{ID: bson.NewObjectID(), Name: "Skipped", URL: "http://127.0.0.1:1", Enabled: false, Timeout: 2 * time.Second},
	}

	results := CheckAll(context.Background(), targets)
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 2 enabled targets, got %d", len(results))
	}

	byURL := make(map[string]domain.CheckResult, len(results))
	for _, r := range results {
		byURL[r.URL] = r
	}
	if !byURL[srvOK.URL].IsUp || byURL[srvOK.URL].StatusCode != http.StatusOK {
		t.Errorf("expected srvOK UP/200, got %+v", byURL[srvOK.URL])
	}
	if byURL[srvErr.URL].IsUp || byURL[srvErr.URL].StatusCode != http.StatusInternalServerError {
		t.Errorf("expected srvErr DOWN/500, got %+v", byURL[srvErr.URL])
	}
}

func TestCheckTargets_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	targets := []domain.Target{
		{Name: "A", URL: srv.URL, Enabled: true},
		{Name: "B", URL: srv.URL, Enabled: true},
	}

	results, logs := CheckTargets(ctx, targets, 2*time.Second)
	if results == nil || logs == nil {
		t.Fatalf("expected non-nil slices even on canceled context")
	}
}

func TestStartMonitoring_WithMocksAndGracefulShutdown(t *testing.T) {
	var hits atomic.Int32
	srvUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srvUp.Close()

	srvDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srvDown.Close()

	targetRepo := database.NewMockTargetRepository()
	logRepo := database.NewMockCheckLogRepository()
	ntf := &mockNotifier{}

	ctx := context.Background()
	_ = targetRepo.Create(ctx, &domain.Target{Name: "UpSvc", URL: srvUp.URL, Enabled: true})
	_ = targetRepo.Create(ctx, &domain.Target{Name: "DownSvc", URL: srvDown.URL, Enabled: true})
	_ = targetRepo.Create(ctx, &domain.Target{Name: "OffSvc", URL: "http://127.0.0.1:1", Enabled: false})

	monCtx, cancel := context.WithCancel(ctx)
	triggerChan := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		StartMonitoring(monCtx, targetRepo, logRepo, ntf, triggerChan, config.MonitorConfig{
			Interval:       60 * time.Millisecond,
			DefaultTimeout: 2 * time.Second,
		})
	}()

	// Wait until at least 2 logs are persisted from the initial cycle
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, total, _ := logRepo.FindAll(ctx, database.CheckLogFilter{}, 10, 0)
		if total >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	triggerChan <- struct{}{}
	time.Sleep(20 * time.Millisecond)
	cancel()
	wg.Wait()

	logs, total, err := logRepo.FindAll(ctx, database.CheckLogFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll logs error: %v", err)
	}
	if total < 2 || len(logs) < 2 {
		t.Fatalf("expected at least 2 persisted logs, got total=%d len=%d", total, len(logs))
	}
	if len(ntf.Alerts()) == 0 {
		t.Errorf("expected alert sent for DownSvc")
	}
	if hits.Load() < 2 {
		t.Errorf("expected HTTP servers to be hit, got %d", hits.Load())
	}
}

func TestResolveInterval(t *testing.T) {
	if got := ResolveInterval(15*time.Second, 30*time.Second); got != 15*time.Second {
		t.Errorf("expected target interval 15s, got %v", got)
	}
	if got := ResolveInterval(0, 45*time.Second); got != 45*time.Second {
		t.Errorf("expected fallback default interval 45s, got %v", got)
	}
	if got := ResolveInterval(0, 0); got != defaultMonitorPeriod {
		t.Errorf("expected built-in fallback %v, got %v", defaultMonitorPeriod, got)
	}
}

func TestRunTargetScheduler_IndependentPerTargetIntervals(t *testing.T) {
	var fastHits, slowHits atomic.Int32

	fastSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fastHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer fastSrv.Close()

	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slowHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowSrv.Close()

	targetRepo := database.NewMockTargetRepository()
	logRepo := database.NewMockCheckLogRepository()
	ctx := context.Background()

	fastTarget := &domain.Target{
		Name:     "FastTarget",
		URL:      fastSrv.URL,
		Enabled:  true,
		Interval: 30 * time.Millisecond,
	}
	slowTarget := &domain.Target{
		Name:     "SlowTarget",
		URL:      slowSrv.URL,
		Enabled:  true,
		Interval: 0, // falls back to cfg.Interval (500ms)
	}
	_ = targetRepo.Create(ctx, fastTarget)
	_ = targetRepo.Create(ctx, slowTarget)

	schedCtx, cancel := context.WithCancel(ctx)
	triggerChan := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunTargetScheduler(schedCtx, targetRepo, logRepo, triggerChan, config.MonitorConfig{
			Interval:       500 * time.Millisecond,
			DefaultTimeout: 2 * time.Second,
		}, SchedulerHooks{})
	}()

	time.Sleep(220 * time.Millisecond)

	// Disable fastTarget and signal reconcile; ensure it stops cleanly
	fastTarget.Enabled = false
	_ = targetRepo.Update(ctx, fastTarget.ID, fastTarget)
	triggerChan <- struct{}{}
	time.Sleep(60 * time.Millisecond)

	cancel()
	wg.Wait()

	fHits := fastHits.Load()
	sHits := slowHits.Load()
	if fHits < 3 {
		t.Errorf("expected fast target (30ms) to run at least 3 times, got %d", fHits)
	}
	if sHits < 1 || sHits > 2 {
		t.Errorf("expected slow target (500ms fallback) to run 1-2 times, got %d", sHits)
	}
	if fHits <= sHits {
		t.Errorf("expected fast target hits (%d) > slow target hits (%d)", fHits, sHits)
	}
}

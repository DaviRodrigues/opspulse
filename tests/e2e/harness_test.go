//go:build e2e

package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/tests/e2e/mongofake"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	repoRoot         string
	binTempDir       string
	apiBinPath       string
	opspulseBinPath  string
	apiBuildErr      error
	apiBuildOut      string
	opspulseBuildErr error
	opspulseBuildOut string
)

func TestMain(m *testing.M) {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e TestMain: %v\n", err)
		os.Exit(1)
	}
	repoRoot = root

	tmpDir, err := os.MkdirTemp("", "opspulse-e2e-bins-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e TestMain MkdirTemp: %v\n", err)
		os.Exit(1)
	}
	binTempDir = tmpDir

	goBin := resolveGoBinary()

	apiBinPath = filepath.Join(binTempDir, "opspulse-api")
	apiBuildOut, apiBuildErr = buildBinary(goBin, repoRoot, "./cmd/api", apiBinPath)

	opspulseBinPath = filepath.Join(binTempDir, "opspulse-worker")
	opspulseBuildOut, opspulseBuildErr = buildBinary(goBin, repoRoot, "./cmd/opspulse", opspulseBinPath)

	code := m.Run()
	_ = os.RemoveAll(binTempDir)
	os.Exit(code)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find go.mod walking up from working directory")
		}
		dir = parent
	}
}

func resolveGoBinary() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	const fallback = "/usr/local/go/bin/go"
	if _, err := os.Stat(fallback); err == nil {
		return fallback
	}
	return "go"
}

func buildBinary(goBin, dir, pkg, outPath string) (string, error) {
	cmd := exec.Command(goBin, "build", "-o", outPath, pkg)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func requireAPIBinary(t *testing.T) string {
	t.Helper()
	if apiBuildErr != nil {
		t.Fatalf("failed to build ./cmd/api: %v\noutput:\n%s", apiBuildErr, apiBuildOut)
	}
	return apiBinPath
}

func requireOpsPulseBinary(t *testing.T) string {
	t.Helper()
	if opspulseBuildErr != nil {
		t.Fatalf("failed to build ./cmd/opspulse: %v\noutput:\n%s", opspulseBuildErr, opspulseBuildOut)
	}
	return opspulseBinPath
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeTCPPort listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	_ = ln.Close()
	return fmt.Sprintf("%d", addr.Port)
}

// --- Sync Buffer for Child Process Output ---

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// --- API Server Process Handle ---

type apiServerOpts struct {
	MongoServer        *mongofake.Server
	DBURI              string
	DBName             string
	DBTimeout          string
	CORSAllowedOrigins string
	MonitorInterval    string
	DefaultTimeout     string
	TargetsFile        string
	ExtraEnv           map[string]string
	SkipReadyWait      bool
}

type apiProcess struct {
	BaseURL string
	Port    string
	Mongo   *mongofake.Server
	DBName  string

	cmd     *exec.Cmd
	logs    *syncBuffer
	doneCh  chan error
	stopped atomic.Bool
}

func startMongoFake(t *testing.T) *mongofake.Server {
	t.Helper()
	srv, err := mongofake.NewServer()
	if err != nil {
		t.Fatalf("mongofake.NewServer: %v", err)
	}
	t.Cleanup(func() {
		_ = srv.Close()
	})
	return srv
}

func startAPIServer(t *testing.T, opts apiServerOpts) *apiProcess {
	t.Helper()
	bin := requireAPIBinary(t)

	mongoSrv := opts.MongoServer
	dbURI := opts.DBURI
	if mongoSrv == nil && dbURI == "" {
		mongoSrv = startMongoFake(t)
	}
	if dbURI == "" && mongoSrv != nil {
		dbURI = mongoSrv.URI()
	}

	dbName := opts.DBName
	if dbName == "" {
		dbName = "opspulse_e2e"
	}
	dbTimeout := opts.DBTimeout
	if dbTimeout == "" {
		dbTimeout = "3s"
	}
	corsOrigins := opts.CORSAllowedOrigins
	if corsOrigins == "" {
		corsOrigins = "http://localhost:5173,http://localhost:5174"
	}
	monInterval := opts.MonitorInterval
	if monInterval == "" {
		monInterval = "200ms"
	}
	defTimeout := opts.DefaultTimeout
	if defTimeout == "" {
		defTimeout = "2s"
	}
	targetsFile := opts.TargetsFile
	if targetsFile == "" {
		targetsFile = "/nonexistent/e2e-missing-targets.json"
	}

	port := freeTCPPort(t)
	logDir := t.TempDir()

	envMap := map[string]string{
		"APP_ENV":                 "development",
		"APP_NAME":                "opspulse-e2e",
		"SERVER_PORT":             port,
		"SERVER_READ_TIMEOUT":     "5s",
		"SERVER_WRITE_TIMEOUT":    "0",
		"SERVER_SHUTDOWN_TIMEOUT": "5s",
		"CORS_ALLOWED_ORIGINS":    corsOrigins,
		"LOG_LEVEL":               "warn",
		"LOG_FORMAT":              "json",
		"LOG_OUTPUT_DIR_API":      logDir,
		"MONITOR_INTERVAL":        monInterval,
		"MONITOR_DEFAULT_TIMEOUT": defTimeout,
		"MONITOR_TARGETS_FILE":    targetsFile,
		"DB_URI":                  dbURI,
		"DB_NAME":                 dbName,
		"DB_TIMEOUT":              dbTimeout,
	}
	for k, v := range opts.ExtraEnv {
		envMap[k] = v
	}

	cmd := exec.Command(bin)
	cmd.Dir = repoRoot
	cmd.Env = buildEnvSlice(envMap)
	logs := &syncBuffer{}
	cmd.Stdout = logs
	cmd.Stderr = logs

	if err := cmd.Start(); err != nil {
		t.Fatalf("start api binary: %v", err)
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	proc := &apiProcess{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%s", port),
		Port:    port,
		Mongo:   mongoSrv,
		DBName:  dbName,
		cmd:     cmd,
		logs:    logs,
		doneCh:  doneCh,
	}

	t.Cleanup(func() {
		_ = proc.Stop(5 * time.Second)
		if t.Failed() {
			t.Logf("=== api process logs (port %s) ===\n%s", port, logs.String())
		}
	})

	if !opts.SkipReadyWait {
		waitForHealth(t, proc, 10*time.Second)
	}

	return proc
}

func waitForHealth(t *testing.T, proc *apiProcess, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}

	for time.Now().Before(deadline) {
		select {
		case err := <-proc.doneCh:
			// Put error back for Stop()
			proc.doneCh <- err
			t.Fatalf("api process exited prematurely (%v) before health ready\nlogs:\n%s", err, proc.logs.String())
		default:
		}

		resp, err := client.Get(proc.BaseURL + "/api/v1/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("api server did not become healthy within %s\nlogs:\n%s", timeout, proc.logs.String())
}

func (p *apiProcess) Stop(timeout time.Duration) error {
	if p.stopped.Swap(true) {
		return nil
	}
	if p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)

	select {
	case err := <-p.doneCh:
		return err
	case <-time.After(timeout):
		_ = p.cmd.Process.Kill()
		return <-p.doneCh
	}
}

func (p *apiProcess) Logs() string {
	return p.logs.String()
}

// --- OpsPulse Worker Process Handle ---

type workerOpts struct {
	MongoServer     *mongofake.Server
	DBURI           string
	DBName          string
	DBTimeout       string
	MonitorInterval string
	DefaultTimeout  string
	TargetsFile     string
}

type workerProcess struct {
	cmd     *exec.Cmd
	logs    *syncBuffer
	doneCh  chan error
	stopped atomic.Bool
}

func startOpsPulseWorker(t *testing.T, opts workerOpts) *workerProcess {
	t.Helper()
	bin := requireOpsPulseBinary(t)

	dbURI := opts.DBURI
	if dbURI == "" && opts.MongoServer != nil {
		dbURI = opts.MongoServer.URI()
	}
	dbName := opts.DBName
	if dbName == "" {
		dbName = "opspulse_e2e"
	}
	dbTimeout := opts.DBTimeout
	if dbTimeout == "" {
		dbTimeout = "3s"
	}
	monInterval := opts.MonitorInterval
	if monInterval == "" {
		monInterval = "200ms"
	}
	defTimeout := opts.DefaultTimeout
	if defTimeout == "" {
		defTimeout = "2s"
	}
	targetsFile := opts.TargetsFile
	if targetsFile == "" {
		targetsFile = "/nonexistent/e2e-missing-targets.json"
	}

	envMap := map[string]string{
		"APP_ENV":                 "development",
		"APP_NAME":                "opspulse-worker-e2e",
		"LOG_LEVEL":               "warn",
		"LOG_FORMAT":              "json",
		"LOG_OUTPUT_DIR_APP":      t.TempDir(),
		"MONITOR_INTERVAL":        monInterval,
		"MONITOR_DEFAULT_TIMEOUT": defTimeout,
		"MONITOR_TARGETS_FILE":    targetsFile,
		"DB_URI":                  dbURI,
		"DB_NAME":                 dbName,
		"DB_TIMEOUT":              dbTimeout,
		"DISCORD_TOKEN":           "e2e-invalid-token",
		"DISCORD_CHANNEL_ID":      "0000000000",
		"DISCORD_GUILD_ID":        "0000000000",
	}

	cmd := exec.Command(bin)
	cmd.Dir = repoRoot
	cmd.Env = buildEnvSlice(envMap)
	logs := &syncBuffer{}
	cmd.Stdout = logs
	cmd.Stderr = logs

	if err := cmd.Start(); err != nil {
		t.Fatalf("start opspulse binary: %v", err)
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	wp := &workerProcess{
		cmd:    cmd,
		logs:   logs,
		doneCh: doneCh,
	}

	t.Cleanup(func() {
		_ = wp.Stop(5 * time.Second)
		if t.Failed() {
			t.Logf("=== opspulse worker logs ===\n%s", logs.String())
		}
	})

	return wp
}

func (w *workerProcess) Stop(timeout time.Duration) error {
	if w.stopped.Swap(true) {
		return nil
	}
	if w.cmd.Process == nil {
		return nil
	}
	_ = w.cmd.Process.Signal(syscall.SIGTERM)

	select {
	case err := <-w.doneCh:
		return err
	case <-time.After(timeout):
		_ = w.cmd.Process.Kill()
		return <-w.doneCh
	}
}

func (w *workerProcess) Alive() bool {
	select {
	case err := <-w.doneCh:
		w.doneCh <- err
		return false
	default:
		return true
	}
}

func buildEnvSlice(overrides map[string]string) []string {
	merged := make(map[string]string)
	for _, entry := range os.Environ() {
		if k, v, ok := strings.Cut(entry, "="); ok {
			merged[k] = v
		}
	}
	for k, v := range overrides {
		merged[k] = v
	}
	out := make([]string, 0, len(merged))
	for k, v := range merged {
		out = append(out, k+"="+v)
	}
	return out
}

// --- Controllable Mock HTTP Target ---

type recordedRequest struct {
	Timestamp time.Time
	Method    string
	Headers   http.Header
	Body      string
}

type mockTarget struct {
	Server *httptest.Server

	statusCode atomic.Int32
	delayNs    atomic.Int64

	mu       sync.Mutex
	body     string
	requests []recordedRequest
}

func newMockTarget(t *testing.T, customHandler func(w http.ResponseWriter, r *http.Request)) *mockTarget {
	t.Helper()
	mt := &mockTarget{
		body: `{"ok":true}`,
	}
	mt.statusCode.Store(http.StatusOK)

	mt.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawBody, _ := io.ReadAll(r.Body)
		mt.mu.Lock()
		mt.requests = append(mt.requests, recordedRequest{
			Timestamp: time.Now(),
			Method:    r.Method,
			Headers:   r.Header.Clone(),
			Body:      string(rawBody),
		})
		respBody := mt.body
		mt.mu.Unlock()

		if d := time.Duration(mt.delayNs.Load()); d > 0 {
			select {
			case <-time.After(d):
			case <-r.Context().Done():
				return
			}
		}

		if customHandler != nil {
			customHandler(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(mt.statusCode.Load()))
		_, _ = w.Write([]byte(respBody))
	}))

	t.Cleanup(func() {
		mt.Server.Close()
	})
	return mt
}

func (mt *mockTarget) URL() string {
	return mt.Server.URL
}

func (mt *mockTarget) SetResponse(status int, body string) {
	mt.statusCode.Store(int32(status))
	mt.mu.Lock()
	mt.body = body
	mt.mu.Unlock()
}

func (mt *mockTarget) SetDelay(d time.Duration) {
	mt.delayNs.Store(int64(d))
}

func (mt *mockTarget) RequestCount() int {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	return len(mt.requests)
}

func (mt *mockTarget) Requests() []recordedRequest {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	cp := make([]recordedRequest, len(mt.requests))
	copy(cp, mt.requests)
	return cp
}

func (mt *mockTarget) Timestamps() []time.Time {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	ts := make([]time.Time, len(mt.requests))
	for i, r := range mt.requests {
		ts[i] = r.Timestamp
	}
	return ts
}

func (mt *mockTarget) ResetRequests() {
	mt.mu.Lock()
	mt.requests = nil
	mt.mu.Unlock()
}

// --- API Domain DTOs (Opaque-box; zero internal/* imports) ---

type apiTarget struct {
	ID                string            `json:"id,omitempty"`
	Name              string            `json:"name"`
	URL               string            `json:"url"`
	Method            string            `json:"method,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	Enabled           bool              `json:"enabled"`
	Tags              []string          `json:"tags,omitempty"`
	Body              map[string]any    `json:"body,omitempty"`
	Timeout           int64             `json:"timeout,omitempty"`
	ExpectedStatus    []int             `json:"expected_status,omitempty"`
	ExpectedBodyMatch string            `json:"expected_body_match,omitempty"`
	CreatedAt         time.Time         `json:"created_at,omitempty"`
	UpdatedAt         time.Time         `json:"updated_at,omitempty"`
}

type apiCheckLog struct {
	ID         string    `json:"id,omitempty"`
	TargetID   string    `json:"target_id,omitempty"`
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	StatusCode int       `json:"status_code"`
	Latency    int64     `json:"latency"`
	IsUp       bool      `json:"is_up"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type apiCheckResult struct {
	Name       string `json:"Name"`
	URL        string `json:"URL"`
	StatusCode int    `json:"StatusCode"`
	Latency    int64  `json:"Latency"`
	IsUp       bool   `json:"IsUp"`
	Error      string `json:"Error,omitempty"`
}

type apiPagination struct {
	Total  int64 `json:"total"`
	Limit  int64 `json:"limit"`
	Offset int64 `json:"offset"`
}

// --- Flexible JSON & HTTP Helpers ---

func doJSONRequest(t *testing.T, method, url string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			bodyReader = strings.NewReader(v)
		case []byte:
			bodyReader = bytes.NewReader(v)
		default:
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("json.Marshal request: %v", err)
			}
			bodyReader = bytes.NewReader(b)
		}
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("http.NewRequest(%s, %s): %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HTTP %s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body for %s %s: %v", method, url, err)
	}
	return resp, respBytes
}

// decodeList handles either a JSON array `[...]` or an envelope `{"data": [...]}`, `{"targets": [...]}`,
// `{"check_logs": [...]}`, or `{"items": [...]}`.
func decodeList[T any](t *testing.T, raw []byte) ([]T, *apiPagination) {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var items []T
		if err := json.Unmarshal(trimmed, &items); err != nil {
			t.Fatalf("decodeList array failed: %v\nbody: %s", err, string(raw))
		}
		return items, nil
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		t.Fatalf("decodeList envelope failed: %v\nbody: %s", err, string(raw))
	}

	var pag *apiPagination
	if pagRaw, ok := envelope["pagination"]; ok && len(pagRaw) > 0 && string(pagRaw) != "null" {
		var p apiPagination
		if err := json.Unmarshal(pagRaw, &p); err == nil {
			pag = &p
		}
	}

	for _, key := range []string{"data", "targets", "check_logs", "items"} {
		if listRaw, ok := envelope[key]; ok {
			if string(bytes.TrimSpace(listRaw)) == "null" {
				return []T{}, pag
			}
			var items []T
			if err := json.Unmarshal(listRaw, &items); err != nil {
				t.Fatalf("decodeList key %q failed: %v\nbody: %s", key, err, string(raw))
			}
			return items, pag
		}
	}

	t.Fatalf("decodeList: unexpected JSON shape (no array or known list key)\nbody: %s", string(raw))
	return nil, nil
}

// decodeItem handles either a direct object `{...}` or `{"data": {...}}`.
func decodeItem[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err == nil {
		if inner, ok := probe["data"]; ok && len(bytes.TrimSpace(inner)) > 0 && bytes.TrimSpace(inner)[0] == '{' {
			var out T
			if err := json.Unmarshal(inner, &out); err != nil {
				t.Fatalf("decodeItem envelope data failed: %v\nbody: %s", err, string(raw))
			}
			return out
		}
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decodeItem failed: %v\nbody: %s", err, string(raw))
	}
	return out
}

func createTargetViaAPI(t *testing.T, baseURL string, payload apiTarget) apiTarget {
	t.Helper()
	resp, raw := doJSONRequest(t, http.MethodPost, baseURL+"/api/v1/targets", payload, nil)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/targets expected 201/200, got %d: %s", resp.StatusCode, string(raw))
	}
	created := decodeItem[apiTarget](t, raw)
	if created.ID == "" {
		t.Fatalf("POST /api/v1/targets returned empty id: %s", string(raw))
	}
	return created
}

func waitForCondition(t *testing.T, timeout, interval time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatalf("timed out after %s waiting for: %s", timeout, desc)
}

// --- SSE Stream Reader ---

type sseEvent struct {
	ID    string
	Event string
	Data  string
}

type sseClient struct {
	Events <-chan sseEvent
	ErrCh  <-chan error
	cancel context.CancelFunc
}

func openSSEStream(t *testing.T, url string, headers map[string]string) *sseClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		t.Fatalf("openSSEStream NewRequest: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("openSSEStream Do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("openSSEStream expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(ct), "text/event-stream") {
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("openSSEStream expected Content-Type text/event-stream, got %q", ct)
	}

	events := make(chan sseEvent, 32)
	errCh := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var cur sseEvent
		var dataLines []string
		flush := func() {
			if cur.Event != "" || len(dataLines) > 0 || cur.ID != "" {
				cur.Data = strings.Join(dataLines, "\n")
				select {
				case events <- cur:
				case <-ctx.Done():
					return
				}
			}
			cur = sseEvent{}
			dataLines = nil
		}

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				flush()
				continue
			}
			if strings.HasPrefix(line, ":") {
				continue // heartbeat comment
			}
			field, val, hasColon := strings.Cut(line, ":")
			if hasColon && strings.HasPrefix(val, " ") {
				val = val[1:]
			}
			switch field {
			case "event":
				cur.Event = val
			case "data":
				dataLines = append(dataLines, val)
			case "id":
				cur.ID = val
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
		close(errCh)
	}()

	sc := &sseClient{
		Events: events,
		ErrCh:  errCh,
		cancel: cancel,
	}
	t.Cleanup(func() {
		sc.Close()
	})
	return sc
}

func (s *sseClient) Close() {
	s.cancel()
}

func (s *sseClient) WaitEvent(t *testing.T, timeout time.Duration, pred func(sseEvent) bool, desc string) sseEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case ev, ok := <-s.Events:
			if !ok {
				t.Fatalf("SSE stream closed before receiving %s", desc)
			}
			if pred(ev) {
				return ev
			}
		case <-timer.C:
			t.Fatalf("timed out after %s waiting for SSE event: %s", timeout, desc)
		}
	}
}

// --- Direct MongoDB Seed Helper (for cmd/opspulse tests) ---

func seedTargetInMongo(t *testing.T, uri, dbName, name, url string, enabled bool) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("seedTargetInMongo connect: %v", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	id := bson.NewObjectID()
	now := time.Now().UTC()
	doc := bson.D{
		{Key: "_id", Value: id},
		{Key: "name", Value: name},
		{Key: "url", Value: url},
		{Key: "method", Value: "GET"},
		{Key: "enabled", Value: enabled},
		{Key: "tags", Value: bson.A{"e2e"}},
		{Key: "timeout", Value: int64(2 * time.Second)},
		{Key: "expected_status", Value: bson.A{200}},
		{Key: "created_at", Value: now},
		{Key: "updated_at", Value: now},
	}
	if _, err := client.Database(dbName).Collection("targets").InsertOne(ctx, doc); err != nil {
		t.Fatalf("seedTargetInMongo InsertOne: %v", err)
	}
	return id.Hex()
}

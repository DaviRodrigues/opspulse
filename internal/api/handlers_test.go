package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/domain"
	"github.com/DaviRodrigues/opspulse/internal/file"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.yaml.in/yaml/v4"
)

func newTestServer(t *testing.T, allowedOrigins []string) (*Server, *database.MockTargetRepository, *database.MockCheckLogRepository) {
	t.Helper()
	targetRepo := database.NewMockTargetRepository()
	checkLogRepo := database.NewMockCheckLogRepository()

	serverCfg := config.ServerConfig{
		Port:               "0",
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       0,
		ShutdownTimeout:    5 * time.Second,
		CORSAllowedOrigins: allowedOrigins,
	}
	monitorCfg := config.MonitorConfig{
		Interval:       30 * time.Second,
		DefaultTimeout: 2 * time.Second,
	}

	srv, err := NewServerWithRepos(context.Background(), nil, targetRepo, checkLogRepo, serverCfg, monitorCfg)
	if err != nil {
		t.Fatalf("NewServerWithRepos failed: %v", err)
	}
	srv.SetConfigures(nil, monitorCfg)
	return srv, targetRepo, checkLogRepo
}

func TestCORSMiddleware_AllowedDisallowedMissingAndPreflight(t *testing.T) {
	srv, _, _ := newTestServer(t, []string{" http://localhost:5173 ", "", "https://app.opspulse.local"})

	t.Run("allowed origin gets echoed with Vary Origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		rec := httptest.NewRecorder()

		srv.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
			t.Fatalf("expected Access-Control-Allow-Origin http://localhost:5173, got %q", got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("expected Vary: Origin, got %q", got)
		}
	})

	t.Run("disallowed origin gets no ACAO header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		rec := httptest.NewRecorder()

		srv.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("expected empty Access-Control-Allow-Origin, got %q", got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("expected Vary: Origin, got %q", got)
		}
	})

	t.Run("missing origin gets no ACAO header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		rec := httptest.NewRecorder()

		srv.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("expected empty Access-Control-Allow-Origin, got %q", got)
		}
	})

	t.Run("OPTIONS preflight allowed returns 204 with PATCH", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/targets", nil)
		req.Header.Set("Origin", "https://app.opspulse.local")
		rec := httptest.NewRecorder()

		srv.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.opspulse.local" {
			t.Fatalf("expected echoed origin, got %q", got)
		}
		methods := rec.Header().Get("Access-Control-Allow-Methods")
		for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
			if !strings.Contains(methods, m) {
				t.Fatalf("expected %s in Access-Control-Allow-Methods (%q)", m, methods)
			}
		}
	})

	t.Run("OPTIONS preflight disallowed returns 403 without ACAO", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/targets", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		rec := httptest.NewRecorder()

		srv.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("expected no Access-Control-Allow-Origin on 403 preflight, got %q", got)
		}
	})

	t.Run("explicit wildcard in env config allows origin without literal *", func(t *testing.T) {
		wildSrv, _, _ := newTestServer(t, []string{"*"})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set("Origin", "https://custom.frontend.dev")
		rec := httptest.NewRecorder()

		wildSrv.Router.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://custom.frontend.dev" {
			t.Fatalf("expected echoed origin when wildcard configured, got %q", got)
		}
	})
}

func TestPrepareSSE_DoesNotSetWildcardCORS(t *testing.T) {
	rec := httptest.NewRecorder()
	flusher, ok := PrepareSSE(rec)
	if !ok || flusher == nil {
		t.Fatal("expected httptest.ResponseRecorder to support http.Flusher")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("PrepareSSE must not set Access-Control-Allow-Origin, got %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}
}

func TestHealthEndpoint(t *testing.T) {
	srv, _, _ := newTestServer(t, []string{"http://localhost:5173"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["status"] != "OK" {
		t.Fatalf("expected status OK, got %+v", body)
	}
}

func TestGetStatus_EmptyReturnsArrayAndEnabledChecksPersistLogs(t *testing.T) {
	srv, targetRepo, checkLogRepo := newTestServer(t, []string{"http://localhost:5173"})

	// Empty targets must return [] not null
	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/status", nil)
	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("expected empty JSON array [], got %q", rec.Body.String())
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if err := targetRepo.Create(context.Background(), &domain.Target{
		Name:    "Active",
		URL:     ts.URL + "/ok",
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := targetRepo.Create(context.Background(), &domain.Target{
		Name:    "Disabled",
		URL:     ts.URL + "/off",
		Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/targets/status", nil)
	rec2 := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}

	var results []domain.CheckResult
	if err := json.Unmarshal(rec2.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if len(results) != 1 || results[0].Name != "Active" || !results[0].IsUp {
		t.Fatalf("expected 1 UP result for Active target, got %+v", results)
	}

	logs, total, err := checkLogRepo.FindAll(context.Background(), database.CheckLogFilter{}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 || logs[0].Name != "Active" {
		t.Fatalf("expected 1 persisted check log, got total=%d logs=%+v", total, logs)
	}
}

func TestHandleSSE_And_RunMonitorCycle(t *testing.T) {
	srv, targetRepo, checkLogRepo := newTestServer(t, []string{"http://localhost:5173"})

	upServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upServer.Close()

	downServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer downServer.Close()

	_ = targetRepo.Create(context.Background(), &domain.Target{Name: "UpSvc", URL: upServer.URL, Enabled: true})
	_ = targetRepo.Create(context.Background(), &domain.Target{Name: "DownSvc", URL: downServer.URL, Enabled: true})

	httpTestSrv := httptest.NewServer(srv.Router)
	defer httpTestSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpTestSrv.URL+"/api/v1/targets/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE connect failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected CORS origin on SSE, got %q", got)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}

	// Trigger monitor cycle after SSE client connected
	srv.runMonitorCycle(context.Background(), 2*time.Second)

	scanner := bufio.NewScanner(resp.Body)
	var seenStatus, seenAlert bool
	for scanner.Scan() {
		line := scanner.Text()
		if line == "event: status" {
			seenStatus = true
		}
		if line == "event: alert" {
			seenAlert = true
		}
		if seenStatus && seenAlert {
			break
		}
	}
	if !seenStatus || !seenAlert {
		t.Fatalf("expected both status and alert SSE events, got seenStatus=%v seenAlert=%v (err=%v)", seenStatus, seenAlert, scanner.Err())
	}

	_, total, err := checkLogRepo.FindAll(context.Background(), database.CheckLogFilter{}, 50, 0)
	if err != nil || total != 2 {
		t.Fatalf("expected 2 persisted logs from monitor cycle, got total=%d err=%v", total, err)
	}
}

func TestTargetsCRUD(t *testing.T) {
	srv, _, _ := newTestServer(t, []string{"http://localhost:5173"})

	// 1. Create with defaults (Method GET, Timeout 5s, ExpectedStatus [200,201,204], Enabled true)
	createBody := `{"name":"Payments API","url":"https://payments.example.com/health","tags":["core","payments"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created domain.Target
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID.IsZero() || created.Method != "GET" || created.Timeout != 5*time.Second || !created.Enabled || len(created.ExpectedStatus) != 3 {
		t.Fatalf("unexpected defaults on created target: %+v", created)
	}

	// 2. Create preserving explicit enabled=false
	createDisabled := `{"name":"Staging API","url":"http://staging.example.com/health","enabled":false,"method":"post"}`
	reqDisabled := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(createDisabled))
	recDisabled := httptest.NewRecorder()
	srv.Router.ServeHTTP(recDisabled, reqDisabled)
	if recDisabled.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recDisabled.Code, recDisabled.Body.String())
	}
	var disabledTarget domain.Target
	if err := json.Unmarshal(recDisabled.Body.Bytes(), &disabledTarget); err != nil {
		t.Fatal(err)
	}
	if disabledTarget.Enabled {
		t.Fatal("expected enabled=false to be preserved")
	}
	if disabledTarget.Method != "POST" {
		t.Fatalf("expected method POST, got %q", disabledTarget.Method)
	}

	// 3. Duplicate URL -> 409
	reqDup := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(createBody))
	recDup := httptest.NewRecorder()
	srv.Router.ServeHTTP(recDup, reqDup)
	if recDup.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate URL, got %d: %s", recDup.Code, recDup.Body.String())
	}

	// 4. Invalid payloads -> 400
	invalidPayloads := []struct {
		name string
		body string
	}{
		{"invalid json", `{`},
		{"missing name", `{"url":"https://a.example.com"}`},
		{"invalid url scheme", `{"name":"FTP","url":"ftp://a.example.com"}`},
		{"invalid method", `{"name":"BadMethod","url":"https://b.example.com","method":"TRACE"}`},
		{"negative timeout", `{"name":"BadTimeout","url":"https://c.example.com","timeout":-1}`},
		{"invalid expected_status", `{"name":"BadStatus","url":"https://d.example.com","expected_status":[99]}`},
		{"expected_status > 599", `{"name":"BadStatus2","url":"https://e.example.com","expected_status":[600]}`},
	}
	for _, tc := range invalidPayloads {
		t.Run("POST invalid "+tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			srv.Router.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d: %s", tc.name, w.Code, w.Body.String())
			}
		})
	}

	// 5. List targets with pagination & filters
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/targets?enabled=true&tag=core&search=payments&limit=10&offset=0", nil)
	listRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}
	var listResp PaginatedTargetsResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if listResp.Pagination.Total != 1 || listResp.Pagination.Limit != 10 || listResp.Pagination.Offset != 0 || len(listResp.Data) != 1 {
		t.Fatalf("unexpected paginated targets response: %+v", listResp)
	}

	// Empty list must return data: [] not null
	emptyReq := httptest.NewRequest(http.MethodGet, "/api/v1/targets?search=nonexistent", nil)
	emptyRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(emptyRec, emptyReq)
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", emptyRec.Code)
	}
	if !strings.Contains(emptyRec.Body.String(), `"data":[]`) {
		t.Fatalf("expected non-nil empty data array, got %s", emptyRec.Body.String())
	}

	// Bad query params -> 400
	for _, qs := range []string{"enabled=notabool", "limit=0", "limit=201", "offset=-1", "limit=abc"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/targets?"+qs, nil)
		w := httptest.NewRecorder()
		srv.Router.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for query %q, got %d", qs, w.Code)
		}
	}

	// 6. Get by ID (200, 400, 404)
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/targets/"+created.ID.Hex(), nil)
	getRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	badIDReq := httptest.NewRequest(http.MethodGet, "/api/v1/targets/not-an-objectid", nil)
	badIDRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(badIDRec, badIDReq)
	if badIDRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", badIDRec.Code)
	}

	missingID := bson.NewObjectID().Hex()
	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/targets/"+missingID, nil)
	missingRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", missingRec.Code)
	}

	// 7. PUT full update preserving CreatedAt & conflict on duplicate URL
	putPayload := map[string]any{
		"name":                "Payments API v2",
		"url":                 "https://payments.example.com/v2/health",
		"method":              "PUT",
		"headers":             map[string]string{"X-Api-Key": "secret"},
		"enabled":             true,
		"tags":                []string{"v2"},
		"body":                map[string]any{"ping": true},
		"timeout":             int64(3 * time.Second),
		"expected_status":     []int{200, 202},
		"expected_body_match": "ok",
	}
	putBytes, _ := json.Marshal(putPayload)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/targets/"+created.ID.Hex(), bytes.NewReader(putBytes))
	putRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on PUT, got %d: %s", putRec.Code, putRec.Body.String())
	}
	var updated domain.Target
	if err := json.Unmarshal(putRec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("expected CreatedAt preserved (%v), got %v", created.CreatedAt, updated.CreatedAt)
	}
	if updated.Method != "PUT" || updated.Headers["X-Api-Key"] != "secret" || updated.ExpectedBodyMatch != "ok" {
		t.Fatalf("unexpected PUT result: %+v", updated)
	}

	// PUT conflict with disabledTarget URL -> 409
	putConflict := `{"name":"Conflict","url":"http://staging.example.com/health","enabled":true}`
	putConfReq := httptest.NewRequest(http.MethodPut, "/api/v1/targets/"+created.ID.Hex(), strings.NewReader(putConflict))
	putConfRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(putConfRec, putConfReq)
	if putConfRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on PUT duplicate URL, got %d", putConfRec.Code)
	}

	// 8. PATCH partial update preserving omitted fields
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+created.ID.Hex(), strings.NewReader(`{"enabled":false}`))
	patchRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on PATCH, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
	var patched domain.Target
	if err := json.Unmarshal(patchRec.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Enabled {
		t.Fatal("expected enabled=false after PATCH")
	}
	if patched.Method != "PUT" || patched.Headers["X-Api-Key"] != "secret" || patched.Body["ping"] != true || patched.ExpectedBodyMatch != "ok" {
		t.Fatalf("PATCH overwrote omitted fields: %+v", patched)
	}

	// 9. DELETE (204, then 404)
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/targets/"+created.ID.Hex(), nil)
	delRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delRec.Code)
	}

	delAgainReq := httptest.NewRequest(http.MethodDelete, "/api/v1/targets/"+created.ID.Hex(), nil)
	delAgainRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(delAgainRec, delAgainReq)
	if delAgainRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", delAgainRec.Code)
	}
}

func TestCheckLogsEndpoints(t *testing.T) {
	srv, _, checkLogRepo := newTestServer(t, []string{"http://localhost:5173"})

	// Empty check-logs returns {"data":[],"pagination":...}
	emptyReq := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs", nil)
	emptyRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(emptyRec, emptyReq)
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", emptyRec.Code)
	}
	if !strings.Contains(emptyRec.Body.String(), `"data":[]`) {
		t.Fatalf("expected non-nil empty data array, got %s", emptyRec.Body.String())
	}

	targetA := bson.NewObjectID()
	targetB := bson.NewObjectID()
	t1 := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 25, 11, 0, 0, 123456000, time.UTC)

	log1 := domain.CheckLog{
		ID:         bson.NewObjectID(),
		TargetID:   targetA,
		Name:       "Auth",
		URL:        "https://auth.example.com",
		StatusCode: 200,
		Latency:    45 * time.Millisecond,
		IsUp:       true,
		CheckedAt:  t1,
	}
	log2 := domain.CheckLog{
		ID:         bson.NewObjectID(),
		TargetID:   targetB,
		Name:       "Billing",
		URL:        "https://billing.example.com",
		StatusCode: 502,
		Latency:    120 * time.Millisecond,
		IsUp:       false,
		Error:      "bad gateway",
		CheckedAt:  t2,
	}
	if err := checkLogRepo.CreateBatch(context.Background(), []domain.CheckLog{log1, log2}); err != nil {
		t.Fatal(err)
	}

	// Filter by target_id, is_up, status_code, search, from, to
	qs := "target_id=" + targetB.Hex() +
		"&is_up=false&status_code=502&search=gateway" +
		"&from=2026-11-25T10:30:00Z&to=2026-11-25T11:30:00.500Z&limit=20&offset=0"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs?"+qs, nil)
	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp PaginatedCheckLogsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Pagination.Total != 1 || resp.Pagination.Limit != 20 || len(resp.Data) != 1 || resp.Data[0].ID != log2.ID {
		t.Fatalf("unexpected check-logs response: %+v", resp)
	}

	// Bad query params -> 400
	badQueries := []string{
		"target_id=bad",
		"is_up=maybe",
		"status_code=abc",
		"from=not-a-date",
		"to=not-a-date",
		"from=2026-11-25T12:00:00Z&to=2026-11-25T10:00:00Z",
		"limit=0",
		"limit=501",
		"offset=-1",
	}
	for _, bq := range badQueries {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs?"+bq, nil)
		w := httptest.NewRecorder()
		srv.Router.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for query %q, got %d", bq, w.Code)
		}
	}

	// Get by ID (200, 400, 404)
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs/"+log1.ID.Hex(), nil)
	getRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	badIDReq := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs/bad-id", nil)
	badIDRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(badIDRec, badIDReq)
	if badIDRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", badIDRec.Code)
	}

	notFoundReq := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs/"+bson.NewObjectID().Hex(), nil)
	notFoundRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(notFoundRec, notFoundReq)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", notFoundRec.Code)
	}

	// status_code=0 is valid (network error), while -1 and 600 are invalid
	zeroReq := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs?status_code=0", nil)
	zeroRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(zeroRec, zeroReq)
	if zeroRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for status_code=0, got %d", zeroRec.Code)
	}
	for _, invalidSC := range []string{"status_code=-1", "status_code=600"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs?"+invalidSC, nil)
		w := httptest.NewRecorder()
		srv.Router.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", invalidSC, w.Code)
		}
	}
}

func TestTargetsEdgeCases_BodyLimitsTrailingJSONAndErrors(t *testing.T) {
	srv, _, _ := newTestServer(t, []string{"http://localhost:5173"})

	// Empty body -> 400
	emptyReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(""))
	emptyRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(emptyRec, emptyReq)
	if emptyRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on empty body, got %d", emptyRec.Code)
	}

	// Trailing JSON -> 400
	trailingReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{"name":"A","url":"https://a.example.com"}{"extra":1}`))
	trailingRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(trailingRec, trailingReq)
	if trailingRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on trailing JSON, got %d", trailingRec.Code)
	}

	// Oversized body (> 1MB) -> 400
	huge := `{"name":"Big","url":"https://big.example.com","expected_body_match":"` + strings.Repeat("a", maxRequestBodyBytes+100) + `"}`
	hugeReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(huge))
	hugeRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(hugeRec, hugeReq)
	if hugeRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on oversized body, got %d", hugeRec.Code)
	}

	// Create two targets for PATCH/PUT/DELETE error coverage
	r1 := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{"name":"One","url":"https://one.example.com"}`))
	w1 := httptest.NewRecorder()
	srv.Router.ServeHTTP(w1, r1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w1.Code)
	}
	var t1 domain.Target
	_ = json.Unmarshal(w1.Body.Bytes(), &t1)

	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{"name":"Two","url":"https://two.example.com"}`))
	w2 := httptest.NewRecorder()
	srv.Router.ServeHTTP(w2, r2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w2.Code)
	}

	// PATCH 400, 404, 409
	patchBadID := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/bad-id", strings.NewReader(`{"enabled":false}`))
	patchBadIDRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(patchBadIDRec, patchBadID)
	if patchBadIDRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on PATCH bad id, got %d", patchBadIDRec.Code)
	}

	patchNotFound := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+bson.NewObjectID().Hex(), strings.NewReader(`{"enabled":false}`))
	patchNotFoundRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(patchNotFoundRec, patchNotFound)
	if patchNotFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on PATCH missing id, got %d", patchNotFoundRec.Code)
	}

	patchInvalid := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+t1.ID.Hex(), strings.NewReader(`{"url":"ftp://invalid"}`))
	patchInvalidRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(patchInvalidRec, patchInvalid)
	if patchInvalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on PATCH invalid url, got %d", patchInvalidRec.Code)
	}

	patchConflict := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+t1.ID.Hex(), strings.NewReader(`{"url":"https://two.example.com"}`))
	patchConflictRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(patchConflictRec, patchConflict)
	if patchConflictRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on PATCH duplicate url, got %d", patchConflictRec.Code)
	}

	// PUT 400 & 404
	putNotFound := httptest.NewRequest(http.MethodPut, "/api/v1/targets/"+bson.NewObjectID().Hex(), strings.NewReader(`{"name":"Missing","url":"https://missing.example.com"}`))
	putNotFoundRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(putNotFoundRec, putNotFound)
	if putNotFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on PUT missing id, got %d", putNotFoundRec.Code)
	}

	// DELETE 400
	delBadID := httptest.NewRequest(http.MethodDelete, "/api/v1/targets/bad-id", nil)
	delBadIDRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(delBadIDRec, delBadID)
	if delBadIDRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on DELETE bad id, got %d", delBadIDRec.Code)
	}

	// GET /docs
	docsReq := httptest.NewRequest(http.MethodGet, "/docs", nil)
	docsRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(docsRec, docsReq)
	if docsRec.Code != http.StatusOK || !strings.Contains(docsRec.Body.String(), "/docs/openapi.yaml") {
		t.Fatalf("expected /docs scalar HTML, got code=%d", docsRec.Code)
	}
}

func TestTargetsValidation_URLAndNullPayload(t *testing.T) {
	srv, _, _ := newTestServer(t, []string{"http://localhost:5173"})

	// Create a valid target for PATCH tests
	seedReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{"name":"Seed","url":"https://seed.example.com"}`))
	seedRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(seedRec, seedReq)
	if seedRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 seeding target, got %d: %s", seedRec.Code, seedRec.Body.String())
	}
	var seeded domain.Target
	if err := json.Unmarshal(seedRec.Body.Bytes(), &seeded); err != nil {
		t.Fatal(err)
	}

	t.Run("hostless port URLs return 400", func(t *testing.T) {
		for _, badURL := range []string{"http://:8080", "https://:8443/health"} {
			body := `{"name":"Hostless","url":"` + badURL + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.Router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for URL %q, got %d: %s", badURL, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("IPv6 loopback accepted while javascript and relative rejected", func(t *testing.T) {
		reqOK := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{"name":"IPv6","url":"http://[::1]:8080"}`))
		recOK := httptest.NewRecorder()
		srv.Router.ServeHTTP(recOK, reqOK)
		if recOK.Code != http.StatusCreated {
			t.Fatalf("expected 201 for http://[::1]:8080, got %d: %s", recOK.Code, recOK.Body.String())
		}

		for _, badURL := range []string{"javascript:alert(1)", "/relative"} {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{"name":"Bad","url":"`+badURL+`"}`))
			rec := httptest.NewRecorder()
			srv.Router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for URL %q, got %d: %s", badURL, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("trailing JSON null or primitive rejected with 400", func(t *testing.T) {
		for _, payload := range []string{
			`{"name":"A","url":"https://a1.example.com"} null`,
			`{"name":"A","url":"https://a2.example.com"} 123`,
		} {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(payload))
			rec := httptest.NewRecorder()
			srv.Router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for trailing payload %q, got %d: %s", payload, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("PATCH with top-level null or non-object JSON returns 400", func(t *testing.T) {
		for _, payload := range []string{`null`, `[]`, `123`} {
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+seeded.ID.Hex(), strings.NewReader(payload))
			rec := httptest.NewRecorder()
			srv.Router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for PATCH payload %s, got %d: %s", payload, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("invalid and empty URL ports return 400 while valid ports and bare IPv6 return 201", func(t *testing.T) {
		portSrv, _, _ := newTestServer(t, []string{"http://localhost:5173"})

		invalidPortURLs := []string{
			"http://example.com:99999",
			"http://example.com:0",
			"http://example.com:",
			"http://[::1]:99999",
			"http://[::1]:0",
			"http://[::1]:",
		}
		for _, badURL := range invalidPortURLs {
			body := `{"name":"BadPort","url":"` + badURL + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(body))
			rec := httptest.NewRecorder()
			portSrv.Router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid port URL %q, got %d: %s", badURL, rec.Code, rec.Body.String())
			}
		}

		validPortURLs := []string{
			"http://example.com:65535",
			"http://[::1]:8080",
			"http://[::1]",
		}
		for _, goodURL := range validPortURLs {
			body := `{"name":"ValidPort","url":"` + goodURL + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(body))
			rec := httptest.NewRecorder()
			portSrv.Router.ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201 for valid URL %q, got %d: %s", goodURL, rec.Code, rec.Body.String())
			}
		}
	})
}

func TestDocsEndpoints_OpenAPISchemaAndContentType(t *testing.T) {
	t.Chdir("../..")
	srv, _, _ := newTestServer(t, []string{"http://localhost:5173"})

	// GET /docs -> 200 text/html
	docsReq := httptest.NewRequest(http.MethodGet, "/docs", nil)
	docsRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(docsRec, docsReq)
	if docsRec.Code != http.StatusOK {
		t.Fatalf("expected /docs 200, got %d", docsRec.Code)
	}
	if ct := docsRec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("expected /docs Content-Type text/html, got %q", ct)
	}

	// GET /docs/openapi.yaml -> 200 yaml
	specReq := httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil)
	specRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(specRec, specReq)
	if specRec.Code != http.StatusOK {
		t.Fatalf("expected /docs/openapi.yaml 200, got %d: %s", specRec.Code, specRec.Body.String())
	}
	if ct := specRec.Header().Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Fatalf("expected /docs/openapi.yaml Content-Type containing yaml, got %q", ct)
	}
	rawBody := specRec.Body.String()
	if !strings.Contains(rawBody, "openapi:") {
		t.Fatalf("expected openapi: in spec body")
	}
	if strings.Contains(rawBody, "Ehab Terra") || strings.Contains(rawBody, "Default response") {
		t.Fatalf("unexpected boilerplate or Default response in /docs/openapi.yaml")
	}

	var doc map[string]any
	if err := yaml.Unmarshal(specRec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	if schemas == nil {
		t.Fatalf("missing components.schemas in openapi.yaml")
	}
	if _, hasBrokenDuration := schemas["time_Duration"]; hasBrokenDuration {
		t.Fatalf("time_Duration schema must not exist in openapi.yaml")
	}

	findSchema := func(suffix string) map[string]any {
		if s, ok := schemas[suffix].(map[string]any); ok {
			return s
		}
		for k, v := range schemas {
			if strings.HasSuffix(k, "_"+suffix) {
				if s, ok := v.(map[string]any); ok {
					return s
				}
			}
		}
		return nil
	}

	targetReqSchema := findSchema("TargetRequest")
	if targetReqSchema == nil {
		t.Fatalf("TargetRequest schema not found in components.schemas")
	}
	reqFieldsRaw, _ := targetReqSchema["required"].([]any)
	var reqFields []string
	for _, item := range reqFieldsRaw {
		if s, ok := item.(string); ok {
			reqFields = append(reqFields, s)
		}
	}
	if !slices.Contains(reqFields, "name") || !slices.Contains(reqFields, "url") {
		t.Fatalf("TargetRequest required must contain [name, url], got %v", reqFields)
	}

	assertInt64Prop := func(schemaName, propName string) {
		t.Helper()
		s := findSchema(schemaName)
		if s == nil {
			t.Fatalf("schema %s not found", schemaName)
		}
		props, _ := s["properties"].(map[string]any)
		prop, _ := props[propName].(map[string]any)
		if prop == nil {
			t.Fatalf("%s.%s property missing", schemaName, propName)
		}
		if prop["type"] != "integer" || prop["format"] != "int64" {
			t.Fatalf("expected %s.%s to be integer/int64, got %#v", schemaName, propName, prop)
		}
	}
	assertInt64Prop("TargetRequest", "timeout")
	assertInt64Prop("PatchTargetRequest", "timeout")
	assertInt64Prop("Target", "timeout")
	assertInt64Prop("CheckLogs", "latency")
	assertInt64Prop("CheckResult", "Latency")

	paths, _ := doc["paths"].(map[string]any)
	openAPIPath, _ := paths["/docs/openapi.yaml"].(map[string]any)
	getOp, _ := openAPIPath["get"].(map[string]any)
	responses, _ := getOp["responses"].(map[string]any)
	resp200, ok := responses["200"].(map[string]any)
	if !ok {
		t.Fatalf("expected 200 response on /docs/openapi.yaml, got %#v", responses)
	}
	content200, _ := resp200["content"].(map[string]any)
	if _, hasYAML := content200["application/yaml"]; !hasYAML {
		t.Fatalf("expected application/yaml media type on /docs/openapi.yaml 200 response, got %#v", content200)
	}
}

func TestTargetsPatchPutDelete_ClearingAndDefaults(t *testing.T) {
	srv, _, _ := newTestServer(t, []string{"http://localhost:5173"})

	createBody := `{
		"name":"Clearing Target",
		"url":"https://clear.example.com/health",
		"tags":["alpha","beta"],
		"headers":{"X-Custom":"val"},
		"body":{"hello":"world"},
		"expected_body_match":"pong",
		"enabled":true
	}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(createBody))
	createRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created domain.Target
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	// PATCH with null fields preserves existing values
	patchNullReq := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+created.ID.Hex(), strings.NewReader(`{"tags":null,"headers":null,"body":null,"expected_body_match":null}`))
	patchNullRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(patchNullRec, patchNullReq)
	if patchNullRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on PATCH null fields, got %d: %s", patchNullRec.Code, patchNullRec.Body.String())
	}
	var afterNull domain.Target
	if err := json.Unmarshal(patchNullRec.Body.Bytes(), &afterNull); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(afterNull.Tags, []string{"alpha", "beta"}) ||
		afterNull.Headers["X-Custom"] != "val" ||
		afterNull.Body["hello"] != "world" ||
		afterNull.ExpectedBodyMatch != "pong" {
		t.Fatalf("PATCH null fields should preserve existing values, got %+v", afterNull)
	}

	// PATCH with empty values clears fields
	patchClearReq := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+created.ID.Hex(), strings.NewReader(`{"tags":[],"headers":{},"body":{},"expected_body_match":""}`))
	patchClearRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(patchClearRec, patchClearReq)
	if patchClearRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on PATCH clear fields, got %d: %s", patchClearRec.Code, patchClearRec.Body.String())
	}
	var afterClear domain.Target
	if err := json.Unmarshal(patchClearRec.Body.Bytes(), &afterClear); err != nil {
		t.Fatal(err)
	}
	if len(afterClear.Tags) != 0 || len(afterClear.Headers) != 0 || len(afterClear.Body) != 0 || afterClear.ExpectedBodyMatch != "" {
		t.Fatalf("PATCH empty fields should clear existing values, got %+v", afterClear)
	}

	// PUT with omitted enabled sets enabled=false
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/targets/"+created.ID.Hex(), strings.NewReader(`{"name":"Replaced","url":"https://clear.example.com/v2"}`))
	putRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on PUT, got %d: %s", putRec.Code, putRec.Body.String())
	}
	var afterPut domain.Target
	if err := json.Unmarshal(putRec.Body.Bytes(), &afterPut); err != nil {
		t.Fatal(err)
	}
	if afterPut.Enabled {
		t.Fatalf("expected PUT with omitted enabled to set enabled=false, got true")
	}

	// DELETE returns 204 with 0-byte empty body
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/targets/"+created.ID.Hex(), nil)
	delRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delRec.Code)
	}
	if delRec.Body.Len() != 0 {
		t.Fatalf("expected 0-byte body on 204 DELETE, got %d bytes: %q", delRec.Body.Len(), delRec.Body.String())
	}
}

func TestHandleSSE_WithSlogChiLoggerAndImmediateFlush(t *testing.T) {
	targetRepo := database.NewMockTargetRepository()
	checkLogRepo := database.NewMockCheckLogRepository()
	serverCfg := config.ServerConfig{
		Port:               "0",
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       0,
		ShutdownTimeout:    5 * time.Second,
		CORSAllowedOrigins: []string{"http://localhost:5173"},
	}
	monitorCfg := config.MonitorConfig{
		Interval:       30 * time.Second,
		DefaultTimeout: 2 * time.Second,
	}

	srv, err := NewServerWithRepos(context.Background(), nil, targetRepo, checkLogRepo, serverCfg, monitorCfg)
	if err != nil {
		t.Fatalf("NewServerWithRepos failed: %v", err)
	}
	defer srv.Broker.Close()
	srv.SetConfigures(slog.New(slog.NewJSONHandler(io.Discard, nil)), monitorCfg)

	httpSrv := httptest.NewServer(srv.Router)
	defer httpSrv.Close()

	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, httpSrv.URL+"/api/v1/targets/events", nil)
	if err != nil {
		t.Fatal(err)
	}

	type sseResp struct {
		resp *http.Response
		err  error
	}
	ch := make(chan sseResp, 1)
	go func() {
		resp, doErr := http.DefaultClient.Do(req)
		ch <- sseResp{resp: resp, err: doErr}
	}()

	var resp *http.Response
	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("SSE request failed: %v", res.err)
		}
		resp = res.resp
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for immediate SSE headers before any Publish")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	// Cancel client context and verify clean disconnect
	cancel()
	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 64)
		_, readErr := resp.Body.Read(buf)
		readDone <- readErr
	}()

	select {
	case readErr := <-readDone:
		if readErr == nil {
			t.Fatal("expected read error after SSE context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE read to terminate after context cancellation")
	}
}

func TestCORS_AdversarialOriginsAndSSEPreflight(t *testing.T) {
	srv, _, _ := newTestServer(t, []string{"http://localhost:5173"})

	adversarialOrigins := []string{
		"http://localhost:5173.evil.com",
		"http://localhost:51730",
		"HTTP://LOCALHOST:5173",
		"http://localhost:5173/",
	}
	for _, origin := range adversarialOrigins {
		t.Run("GET rejects "+origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()
			srv.Router.ServeHTTP(rec, req)
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("expected no ACAO for adversarial origin %q, got %q", origin, got)
			}
		})

		t.Run("OPTIONS rejects "+origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/api/v1/targets/events", nil)
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()
			srv.Router.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for adversarial preflight %q, got %d", origin, rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("expected no ACAO for adversarial preflight %q, got %q", origin, got)
			}
		})
	}

	t.Run("OPTIONS without Origin returns 204 without ACAO", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/targets", nil)
		rec := httptest.NewRecorder()
		srv.Router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("expected no ACAO, got %q", got)
		}
	})

	t.Run("LoadServerConfig whitespace and empty CORS_ALLOWED_ORIGINS fallback", func(t *testing.T) {
		expectedFallback := []string{"http://localhost:5173", "http://localhost:5174"}
		for _, rawVal := range []string{"", "   ", " , , "} {
			t.Setenv("CORS_ALLOWED_ORIGINS", rawVal)
			cfg, err := config.LoadServerConfig(file.NewEnvFile())
			if err != nil {
				t.Fatalf("LoadServerConfig failed for %q: %v", rawVal, err)
			}
			if !slices.Equal(cfg.CORSAllowedOrigins, expectedFallback) {
				t.Fatalf("for %q expected fallback %v, got %v", rawVal, expectedFallback, cfg.CORSAllowedOrigins)
			}
		}
	})
}

func TestPaginationAndFiltering_Boundaries(t *testing.T) {
	srv, _, checkLogRepo := newTestServer(t, []string{"http://localhost:5173"})

	// limit=200 on /targets -> 200
	reqT := httptest.NewRequest(http.MethodGet, "/api/v1/targets?limit=200", nil)
	recT := httptest.NewRecorder()
	srv.Router.ServeHTTP(recT, reqT)
	if recT.Code != http.StatusOK {
		t.Fatalf("expected 200 for /targets?limit=200, got %d", recT.Code)
	}
	var respT PaginatedTargetsResponse
	if err := json.Unmarshal(recT.Body.Bytes(), &respT); err != nil {
		t.Fatal(err)
	}
	if respT.Pagination.Limit != 200 {
		t.Fatalf("expected limit=200, got %d", respT.Pagination.Limit)
	}

	// limit=500 on /check-logs -> 200
	reqC := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs?limit=500", nil)
	recC := httptest.NewRecorder()
	srv.Router.ServeHTTP(recC, reqC)
	if recC.Code != http.StatusOK {
		t.Fatalf("expected 200 for /check-logs?limit=500, got %d", recC.Code)
	}
	var respC PaginatedCheckLogsResponse
	if err := json.Unmarshal(recC.Body.Bytes(), &respC); err != nil {
		t.Fatal(err)
	}
	if respC.Pagination.Limit != 500 {
		t.Fatalf("expected limit=500, got %d", respC.Pagination.Limit)
	}

	// from == to inclusive match
	exactTS := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	if err := checkLogRepo.Create(context.Background(), &domain.CheckLog{
		ID:         bson.NewObjectID(),
		TargetID:   bson.NewObjectID(),
		Name:       "ExactTS",
		URL:        "https://exact.example.com",
		StatusCode: 200,
		Latency:    10 * time.Millisecond,
		IsUp:       true,
		CheckedAt:  exactTS,
	}); err != nil {
		t.Fatal(err)
	}
	tsStr := exactTS.Format(time.RFC3339)
	reqEq := httptest.NewRequest(http.MethodGet, "/api/v1/check-logs?from="+tsStr+"&to="+tsStr, nil)
	recEq := httptest.NewRecorder()
	srv.Router.ServeHTTP(recEq, reqEq)
	if recEq.Code != http.StatusOK {
		t.Fatalf("expected 200 for from==to, got %d: %s", recEq.Code, recEq.Body.String())
	}
	var respEq PaginatedCheckLogsResponse
	if err := json.Unmarshal(recEq.Body.Bytes(), &respEq); err != nil {
		t.Fatal(err)
	}
	if respEq.Pagination.Total != 1 || len(respEq.Data) != 1 || respEq.Data[0].Name != "ExactTS" {
		t.Fatalf("expected inclusive match for from==to, got %+v", respEq)
	}

	// int64 overflow strings -> 400
	overflowQueries := []string{
		"limit=9223372036854775808",
		"offset=9223372036854775808",
		"offset=-9223372036854775809",
	}
	for _, endpoint := range []string{"/api/v1/targets", "/api/v1/check-logs"} {
		for _, qs := range overflowQueries {
			r := httptest.NewRequest(http.MethodGet, endpoint+"?"+qs, nil)
			w := httptest.NewRecorder()
			srv.Router.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s?%s, got %d", endpoint, qs, w.Code)
			}
		}
	}
}

func TestServerSetup_GracefulShutdownWithActiveSSEAndPortConflict(t *testing.T) {
	t.Run("graceful shutdown with active SSE connection", func(t *testing.T) {
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("failed to allocate free port: %v", err)
		}
		port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
		_ = ln.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		serverCfg := config.ServerConfig{
			Port:               port,
			ReadTimeout:        5 * time.Second,
			WriteTimeout:       0,
			ShutdownTimeout:    5 * time.Second,
			CORSAllowedOrigins: []string{"http://localhost:5173"},
		}
		monitorCfg := config.MonitorConfig{
			Interval:       30 * time.Second,
			DefaultTimeout: 2 * time.Second,
		}
		srv, err := NewServerWithRepos(ctx, nil, database.NewMockTargetRepository(), database.NewMockCheckLogRepository(), serverCfg, monitorCfg)
		if err != nil {
			t.Fatalf("NewServerWithRepos failed: %v", err)
		}
		srv.SetConfigures(slog.New(slog.NewJSONHandler(io.Discard, nil)), monitorCfg)

		setupErrCh := make(chan error, 1)
		go func() {
			setupErrCh <- srv.Setup(ctx, monitorCfg)
		}()

		baseURL := "http://127.0.0.1:" + port
		healthClient := &http.Client{
			Timeout: 500 * time.Millisecond,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		}
		defer healthClient.CloseIdleConnections()

		deadline := time.Now().Add(2 * time.Second)
		for {
			resp, err := healthClient.Get(baseURL + "/api/v1/health")
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					break
				}
			}
			if time.Now().After(deadline) {
				cancel()
				t.Fatal("server did not start listening in time")
			}
			time.Sleep(15 * time.Millisecond)
		}

		sseClient := &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		}
		defer sseClient.CloseIdleConnections()

		sseResp, err := sseClient.Get(baseURL + "/api/v1/targets/events")
		if err != nil {
			cancel()
			t.Fatalf("SSE connect failed: %v", err)
		}
		defer sseResp.Body.Close()
		if sseResp.StatusCode != http.StatusOK {
			cancel()
			t.Fatalf("expected SSE 200, got %d", sseResp.StatusCode)
		}

		cancel()
		select {
		case setupErr := <-setupErrCh:
			if setupErr != nil {
				t.Fatalf("expected Setup to return nil on graceful shutdown, got %v", setupErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Setup took too long to shut down with active SSE connection")
		}
	})

	t.Run("port conflict returns listen error without hanging", func(t *testing.T) {
		occupied, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("failed to bind test port: %v", err)
		}
		defer occupied.Close()
		port := strconv.Itoa(occupied.Addr().(*net.TCPAddr).Port)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		serverCfg := config.ServerConfig{
			Port:               port,
			ReadTimeout:        5 * time.Second,
			WriteTimeout:       0,
			ShutdownTimeout:    2 * time.Second,
			CORSAllowedOrigins: []string{"http://localhost:5173"},
		}
		monitorCfg := config.MonitorConfig{
			Interval:       30 * time.Second,
			DefaultTimeout: 2 * time.Second,
		}
		srv, err := NewServerWithRepos(ctx, nil, database.NewMockTargetRepository(), database.NewMockCheckLogRepository(), serverCfg, monitorCfg)
		if err != nil {
			t.Fatalf("NewServerWithRepos failed: %v", err)
		}
		srv.SetConfigures(nil, monitorCfg)

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.Setup(ctx, monitorCfg)
		}()

		select {
		case setupErr := <-errCh:
			if setupErr == nil {
				t.Fatal("expected non-nil listen error on port conflict")
			}
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("Setup hung on port conflict")
		}
	})
}

func TestTargetsValidation_ExpectedBodyMatchRegex(t *testing.T) {
	srv, _, _ := newTestServer(t, []string{"http://localhost:5173"})

	// Seed a valid target for PUT/PATCH tests
	seedReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{"name":"RegexSeed","url":"https://regex-seed.example.com","expected_body_match":"status=ok"}`))
	seedRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(seedRec, seedReq)
	if seedRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 seeding target, got %d: %s", seedRec.Code, seedRec.Body.String())
	}
	var seeded domain.Target
	if err := json.Unmarshal(seedRec.Body.Bytes(), &seeded); err != nil {
		t.Fatal(err)
	}

	invalidRegexes := []string{"[OK", "(unclosed", "*invalid"}
	for _, pat := range invalidRegexes {
		payload, _ := json.Marshal(map[string]any{
			"name":                "InvalidRegex",
			"url":                 "https://invalid-regex.example.com/" + pat,
			"expected_body_match": pat,
		})

		// POST -> 400
		postReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", bytes.NewReader(payload))
		postRec := httptest.NewRecorder()
		srv.Router.ServeHTTP(postRec, postReq)
		if postRec.Code != http.StatusBadRequest {
			t.Fatalf("POST expected 400 for invalid expected_body_match %q, got %d: %s", pat, postRec.Code, postRec.Body.String())
		}

		// PUT -> 400
		putPayload, _ := json.Marshal(map[string]any{
			"name":                "RegexSeed",
			"url":                 seeded.URL,
			"expected_body_match": pat,
		})
		putReq := httptest.NewRequest(http.MethodPut, "/api/v1/targets/"+seeded.ID.Hex(), bytes.NewReader(putPayload))
		putRec := httptest.NewRecorder()
		srv.Router.ServeHTTP(putRec, putReq)
		if putRec.Code != http.StatusBadRequest {
			t.Fatalf("PUT expected 400 for invalid expected_body_match %q, got %d: %s", pat, putRec.Code, putRec.Body.String())
		}

		// PATCH -> 400
		patchPayload, _ := json.Marshal(map[string]any{
			"expected_body_match": pat,
		})
		patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+seeded.ID.Hex(), bytes.NewReader(patchPayload))
		patchRec := httptest.NewRecorder()
		srv.Router.ServeHTTP(patchRec, patchReq)
		if patchRec.Code != http.StatusBadRequest {
			t.Fatalf("PATCH expected 400 for invalid expected_body_match %q, got %d: %s", pat, patchRec.Code, patchRec.Body.String())
		}
	}

	// Valid regexes accepted on POST, PUT, PATCH
	validRegexes := []string{"status=ok", "(?i)healthy", `^ok$`}
	for i, pat := range validRegexes {
		postPayload, _ := json.Marshal(map[string]any{
			"name":                "ValidRegex",
			"url":                 "https://valid-regex-" + strconv.Itoa(i) + ".example.com",
			"expected_body_match": pat,
		})
		postReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", bytes.NewReader(postPayload))
		postRec := httptest.NewRecorder()
		srv.Router.ServeHTTP(postRec, postReq)
		if postRec.Code != http.StatusCreated {
			t.Fatalf("POST expected 201 for valid expected_body_match %q, got %d: %s", pat, postRec.Code, postRec.Body.String())
		}

		patchPayload, _ := json.Marshal(map[string]any{"expected_body_match": pat})
		patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+seeded.ID.Hex(), bytes.NewReader(patchPayload))
		patchRec := httptest.NewRecorder()
		srv.Router.ServeHTTP(patchRec, patchReq)
		if patchRec.Code != http.StatusOK {
			t.Fatalf("PATCH expected 200 for valid expected_body_match %q, got %d: %s", pat, patchRec.Code, patchRec.Body.String())
		}

		putPayload, _ := json.Marshal(map[string]any{
			"name":                "RegexSeed",
			"url":                 seeded.URL,
			"expected_body_match": pat,
		})
		putReq := httptest.NewRequest(http.MethodPut, "/api/v1/targets/"+seeded.ID.Hex(), bytes.NewReader(putPayload))
		putRec := httptest.NewRecorder()
		srv.Router.ServeHTTP(putRec, putReq)
		if putRec.Code != http.StatusOK {
			t.Fatalf("PUT expected 200 for valid expected_body_match %q, got %d: %s", pat, putRec.Code, putRec.Body.String())
		}
	}
}

func TestTargetInterval_CRUDAndValidation(t *testing.T) {
	srv, _, _ := newTestServer(t, []string{"http://localhost:5173"})

	// POST with explicit interval (15s)
	postBody := `{"name":"CustomInterval","url":"https://interval.example.com","interval":15000000000}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(postBody))
	postRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", postRec.Code, postRec.Body.String())
	}
	var created domain.Target
	if err := json.Unmarshal(postRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created target: %v", err)
	}
	if created.Interval != 15*time.Second {
		t.Fatalf("expected interval 15s, got %v", created.Interval)
	}

	// PATCH interval to 45s
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+created.ID.Hex(), strings.NewReader(`{"interval":45000000000}`))
	patchRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on PATCH interval, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
	var patched domain.Target
	_ = json.Unmarshal(patchRec.Body.Bytes(), &patched)
	if patched.Interval != 45*time.Second {
		t.Fatalf("expected patched interval 45s, got %v", patched.Interval)
	}

	// PUT interval to 0 (inherits MONITOR_INTERVAL)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/targets/"+created.ID.Hex(), strings.NewReader(`{"name":"CustomInterval","url":"https://interval.example.com","enabled":true,"interval":0}`))
	putRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on PUT interval=0, got %d: %s", putRec.Code, putRec.Body.String())
	}
	var updated domain.Target
	_ = json.Unmarshal(putRec.Body.Bytes(), &updated)
	if updated.Interval != 0 {
		t.Fatalf("expected updated interval 0, got %v", updated.Interval)
	}

	// Negative interval rejected on POST and PATCH
	negPost := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{"name":"Neg","url":"https://neg.example.com","interval":-1000}`))
	negPostRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(negPostRec, negPost)
	if negPostRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on negative POST interval, got %d", negPostRec.Code)
	}

	negPatch := httptest.NewRequest(http.MethodPatch, "/api/v1/targets/"+created.ID.Hex(), strings.NewReader(`{"interval":-1}`))
	negPatchRec := httptest.NewRecorder()
	srv.Router.ServeHTTP(negPatchRec, negPatch)
	if negPatchRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on negative PATCH interval, got %d", negPatchRec.Code)
	}
}

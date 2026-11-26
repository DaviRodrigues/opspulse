//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTier1_F2_PersistenceAndBuild(t *testing.T) {
	t.Run("1_api_builds_and_boots_with_mongofake", func(t *testing.T) {
		proc := startAPIServer(t, apiServerOpts{})
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/health", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(raw))
		}
	})

	t.Run("2_opspulse_builds_and_boots_without_target_file", func(t *testing.T) {
		mongoSrv := startMongoFake(t)
		mt := newMockTarget(t, nil)
		seedTargetInMongo(t, mongoSrv.URI(), "opspulse_e2e_worker", "WorkerTarget", mt.URL(), true)

		wp := startOpsPulseWorker(t, workerOpts{
			MongoServer:     mongoSrv,
			DBName:          "opspulse_e2e_worker",
			MonitorInterval: "150ms",
			TargetsFile:     "/nonexistent/missing-worker-targets.json",
		})
		waitForCondition(t, 5*time.Second, 50*time.Millisecond, "opspulse worker checks MongoDB target", func() bool {
			return mt.RequestCount() >= 1
		})
		if !wp.Alive() {
			t.Fatal("opspulse worker exited unexpectedly")
		}
	})

	t.Run("3_api_ignores_nonexistent_targets_file", func(t *testing.T) {
		proc := startAPIServer(t, apiServerOpts{
			TargetsFile: "/nonexistent/path/does-not-exist.yaml",
		})
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(raw))
		}
	})

	t.Run("4_targets_persist_across_api_restart", func(t *testing.T) {
		mongoSrv := startMongoFake(t)
		mt := newMockTarget(t, nil)

		proc1 := startAPIServer(t, apiServerOpts{
			MongoServer: mongoSrv,
			DBName:      "persist_targets_db",
		})
		created := createTargetViaAPI(t, proc1.BaseURL, apiTarget{
			Name:           "Persistent Service",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})
		if err := proc1.Stop(5 * time.Second); err != nil {
			t.Fatalf("proc1.Stop error: %v", err)
		}

		proc2 := startAPIServer(t, apiServerOpts{
			MongoServer: mongoSrv,
			DBName:      "persist_targets_db",
		})
		resp, raw := doJSONRequest(t, http.MethodGet, proc2.BaseURL+"/api/v1/targets/"+created.ID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 after restart, got %d: %s", resp.StatusCode, string(raw))
		}
		got := decodeItem[apiTarget](t, raw)
		if got.ID != created.ID || got.Name != "Persistent Service" || got.URL != mt.URL() {
			t.Fatalf("target mismatch after restart: %+v", got)
		}
	})

	t.Run("5_check_logs_persist_across_api_restart", func(t *testing.T) {
		mongoSrv := startMongoFake(t)
		mt := newMockTarget(t, nil)

		proc1 := startAPIServer(t, apiServerOpts{
			MongoServer:     mongoSrv,
			DBName:          "persist_logs_db",
			MonitorInterval: "150ms",
		})
		created := createTargetViaAPI(t, proc1.BaseURL, apiTarget{
			Name:           "Logged Service",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})

		waitForCondition(t, 5*time.Second, 50*time.Millisecond, "at least 2 check logs persisted", func() bool {
			resp, raw := doJSONRequest(t, http.MethodGet, proc1.BaseURL+"/api/v1/check-logs?target_id="+created.ID, nil, nil)
			if resp.StatusCode != http.StatusOK {
				return false
			}
			logs, _ := decodeList[apiCheckLog](t, raw)
			return len(logs) >= 2
		})

		if err := proc1.Stop(5 * time.Second); err != nil {
			t.Fatalf("proc1.Stop error: %v", err)
		}

		// Disable monitoring noise by clearing target? Keep it enabled, just confirm existing logs remain on restart.
		proc2 := startAPIServer(t, apiServerOpts{
			MongoServer:     mongoSrv,
			DBName:          "persist_logs_db",
			MonitorInterval: "10s",
		})
		resp, raw := doJSONRequest(t, http.MethodGet, proc2.BaseURL+"/api/v1/check-logs?target_id="+created.ID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 after restart, got %d: %s", resp.StatusCode, string(raw))
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		if len(logs) < 2 {
			t.Fatalf("expected >=2 persisted check logs after restart, got %d", len(logs))
		}
	})
}

func TestTier1_F3_HealthDocsAndCORS(t *testing.T) {
	const origin1 = "http://localhost:5173"
	const origin2 = "http://opspulse.local:4173"
	proc := startAPIServer(t, apiServerOpts{
		CORSAllowedOrigins: origin1 + "," + origin2,
	})

	t.Run("1_health_returns_200_ok", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/health", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var body map[string]string
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("unmarshal health: %v", err)
		}
		if body["status"] != "OK" {
			t.Fatalf("expected status OK, got %q", body["status"])
		}
	})

	t.Run("2_docs_openapi_yaml_returns_200", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/docs/openapi.yaml", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if !strings.Contains(string(raw), "openapi:") {
			t.Fatalf("expected openapi YAML content, got: %.120s", string(raw))
		}
	})

	t.Run("3_docs_scalar_html_returns_200", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/docs", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "text/html") || !strings.Contains(strings.ToLower(string(raw)), "<html") {
			t.Fatalf("expected HTML response, ct=%q body=%.120s", ct, string(raw))
		}
	})

	t.Run("4_cors_preflight_allows_first_origin_not_wildcard", func(t *testing.T) {
		resp, _ := doJSONRequest(t, http.MethodOptions, proc.BaseURL+"/api/v1/targets", nil, map[string]string{
			"Origin":                         origin1,
			"Access-Control-Request-Method":  "POST",
			"Access-Control-Request-Headers": "Content-Type",
		})
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 200/204 preflight, got %d", resp.StatusCode)
		}
		acao := resp.Header.Get("Access-Control-Allow-Origin")
		if acao != origin1 || acao == "*" {
			t.Fatalf("expected Access-Control-Allow-Origin=%q (never *), got %q", origin1, acao)
		}
	})

	t.Run("5_cors_allows_second_origin_and_echoes_non_wildcard", func(t *testing.T) {
		resp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/health", nil, map[string]string{
			"Origin": origin2,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		acao := resp.Header.Get("Access-Control-Allow-Origin")
		if acao != origin2 || acao == "*" {
			t.Fatalf("expected Access-Control-Allow-Origin=%q (never *), got %q", origin2, acao)
		}
	})
}

func TestTier1_F4_TargetsCRUD(t *testing.T) {
	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "10s"})
	mt1 := newMockTarget(t, nil)
	mt2 := newMockTarget(t, nil)

	var createdID string

	t.Run("1_post_creates_target", func(t *testing.T) {
		created := createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "Billing API",
			URL:            mt1.URL(),
			Method:         "GET",
			Enabled:        true,
			Tags:           []string{"prod", "payments"},
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200, 201},
		})
		createdID = created.ID
		if created.Name != "Billing API" || created.URL != mt1.URL() {
			t.Fatalf("unexpected created target: %+v", created)
		}
	})

	t.Run("2_get_lists_targets", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(raw))
		}
		list, _ := decodeList[apiTarget](t, raw)
		if len(list) != 1 || list[0].ID != createdID {
			t.Fatalf("expected 1 target with ID %s, got %+v", createdID, list)
		}
	})

	t.Run("3_get_by_id_retrieves_target", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets/"+createdID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(raw))
		}
		got := decodeItem[apiTarget](t, raw)
		if got.ID != createdID || got.Name != "Billing API" {
			t.Fatalf("unexpected target by id: %+v", got)
		}
	})

	t.Run("4_put_updates_target_fields", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodPut, proc.BaseURL+"/api/v1/targets/"+createdID, apiTarget{
			Name:           "Billing API v2",
			URL:            mt2.URL(),
			Method:         "GET",
			Enabled:        false,
			Tags:           []string{"staging"},
			Timeout:        int64(3 * time.Second),
			ExpectedStatus: []int{200, 204},
		}, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on PUT, got %d: %s", resp.StatusCode, string(raw))
		}
		updated := decodeItem[apiTarget](t, raw)
		if updated.Name != "Billing API v2" || updated.URL != mt2.URL() || updated.Enabled {
			t.Fatalf("unexpected updated target: %+v", updated)
		}
	})

	t.Run("5_delete_removes_target_and_subsequent_get_404", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodDelete, proc.BaseURL+"/api/v1/targets/"+createdID, nil, nil)
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 204/200 on DELETE, got %d: %s", resp.StatusCode, string(raw))
		}
		getResp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets/"+createdID, nil, nil)
		if getResp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 after DELETE, got %d", getResp.StatusCode)
		}
	})

	t.Run("6_post_supports_custom_method_tags_body_match_and_timeout", func(t *testing.T) {
		mtCustom := newMockTarget(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"health":"healthy-ok"}`))
		})
		created := createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:              "Custom POST Target",
			URL:               mtCustom.URL() + "/probe",
			Method:            "POST",
			Headers:           map[string]string{"X-Probe": "e2e"},
			Enabled:           true,
			Tags:              []string{"custom", "post"},
			Body:              map[string]any{"ping": true},
			Timeout:           int64(1500 * time.Millisecond),
			ExpectedStatus:    []int{201},
			ExpectedBodyMatch: "healthy-ok",
		})
		if created.Method != "POST" || !slices.Contains(created.Tags, "custom") || created.ExpectedBodyMatch != "healthy-ok" {
			t.Fatalf("custom target fields not preserved: %+v", created)
		}
		if created.Timeout != int64(1500*time.Millisecond) {
			t.Fatalf("expected timeout 1.5s (%d), got %d", int64(1500*time.Millisecond), created.Timeout)
		}
	})
}

func TestTier1_F5_CheckLogs(t *testing.T) {
	mtUp := newMockTarget(t, nil)
	mtDown := newMockTarget(t, nil)
	mtDown.SetResponse(http.StatusInternalServerError, `{"err":"boom"}`)

	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "150ms"})

	tUp := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Alpha Healthy",
		URL:            mtUp.URL() + "/alpha",
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})
	tDown := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Beta Failing",
		URL:            mtDown.URL() + "/beta",
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})

	waitForCondition(t, 6*time.Second, 50*time.Millisecond, "logs recorded for both targets", func() bool {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?limit=100", nil, nil)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		var cUp, cDown int
		for _, l := range logs {
			if l.TargetID == tUp.ID {
				cUp++
			}
			if l.TargetID == tDown.ID {
				cDown++
			}
		}
		return cUp >= 3 && cDown >= 3
	})

	t.Run("1_monitoring_populates_check_logs", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(raw))
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		if len(logs) < 6 {
			t.Fatalf("expected >=6 logs, got %d", len(logs))
		}
	})

	t.Run("2_filter_by_target_id", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+tUp.ID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(raw))
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		if len(logs) == 0 {
			t.Fatal("expected non-empty logs for tUp")
		}
		for _, l := range logs {
			if l.TargetID != tUp.ID {
				t.Fatalf("unexpected target_id %q (wanted %q)", l.TargetID, tUp.ID)
			}
		}
	})

	t.Run("3_filter_by_is_up_true_and_false", func(t *testing.T) {
		respT, rawT := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?is_up=true", nil, nil)
		if respT.StatusCode != http.StatusOK {
			t.Fatalf("is_up=true expected 200, got %d", respT.StatusCode)
		}
		logsT, _ := decodeList[apiCheckLog](t, rawT)
		if len(logsT) == 0 {
			t.Fatal("expected logs with is_up=true")
		}
		for _, l := range logsT {
			if !l.IsUp {
				t.Fatalf("expected is_up=true, got false in %+v", l)
			}
		}

		respF, rawF := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?is_up=false", nil, nil)
		if respF.StatusCode != http.StatusOK {
			t.Fatalf("is_up=false expected 200, got %d", respF.StatusCode)
		}
		logsF, _ := decodeList[apiCheckLog](t, rawF)
		if len(logsF) == 0 {
			t.Fatal("expected logs with is_up=false")
		}
		for _, l := range logsF {
			if l.IsUp {
				t.Fatalf("expected is_up=false, got true in %+v", l)
			}
		}
	})

	t.Run("4_filter_by_status_code", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?status_code=500", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		if len(logs) == 0 {
			t.Fatal("expected logs with status_code=500")
		}
		for _, l := range logs {
			if l.StatusCode != 500 {
				t.Fatalf("expected status_code 500, got %d", l.StatusCode)
			}
		}
	})

	t.Run("5_filter_by_search_and_from_to_range", func(t *testing.T) {
		from := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
		to := time.Now().UTC().Add(1 * time.Minute).Format(time.RFC3339)
		url := fmt.Sprintf("%s/api/v1/check-logs?search=alpha&from=%s&to=%s", proc.BaseURL, from, to)
		resp, raw := doJSONRequest(t, http.MethodGet, url, nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(raw))
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		if len(logs) == 0 {
			t.Fatal("expected matching logs for search=alpha in time window")
		}
		for _, l := range logs {
			if !strings.Contains(strings.ToLower(l.Name+l.URL+l.Error), "alpha") {
				t.Fatalf("log did not match search 'alpha': %+v", l)
			}
		}
	})

	t.Run("6_get_by_id_and_limit_offset_pagination", func(t *testing.T) {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?limit=2&offset=1", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		logs, pag := decodeList[apiCheckLog](t, raw)
		if len(logs) != 2 {
			t.Fatalf("expected 2 logs with limit=2, got %d", len(logs))
		}
		if pag != nil && pag.Total < 6 {
			t.Fatalf("expected pagination total >= 6, got %d", pag.Total)
		}

		oneResp, oneRaw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs/"+logs[0].ID, nil, nil)
		if oneResp.StatusCode != http.StatusOK {
			t.Fatalf("GET /api/v1/check-logs/%s expected 200, got %d: %s", logs[0].ID, oneResp.StatusCode, string(oneRaw))
		}
		one := decodeItem[apiCheckLog](t, oneRaw)
		if one.ID != logs[0].ID {
			t.Fatalf("expected id %s, got %s", logs[0].ID, one.ID)
		}
	})
}

func TestTier1_F6_JitterAndSSE(t *testing.T) {
	t.Run("1_targets_status_reflects_live_targets", func(t *testing.T) {
		mt := newMockTarget(t, nil)
		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "10s"})

		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "Live Status Target",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})

		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets/status", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(raw))
		}
		var results []apiCheckResult
		if err := json.Unmarshal(raw, &results); err != nil {
			t.Fatalf("unmarshal CheckResult slice: %v\nbody: %s", err, string(raw))
		}
		if len(results) != 1 || !results[0].IsUp || results[0].StatusCode != 200 || results[0].Name != "Live Status Target" {
			t.Fatalf("unexpected /targets/status payload: %+v", results)
		}
	})

	t.Run("2_sse_emits_status_event", func(t *testing.T) {
		mt := newMockTarget(t, nil)
		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "150ms"})
		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "SSE Healthy",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})

		sse := openSSEStream(t, proc.BaseURL+"/api/v1/targets/events", nil)
		ev := sse.WaitEvent(t, 5*time.Second, func(e sseEvent) bool {
			return e.Event == "status" && strings.Contains(e.Data, "SSE Healthy")
		}, "SSE status event with SSE Healthy")

		var results []apiCheckResult
		if err := json.Unmarshal([]byte(ev.Data), &results); err != nil {
			t.Fatalf("unmarshal SSE status data: %v\ndata: %s", err, ev.Data)
		}
	})

	t.Run("3_sse_emits_alert_event_on_500", func(t *testing.T) {
		mt := newMockTarget(t, nil)
		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "150ms"})
		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "SSE Alert Target",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})

		sse := openSSEStream(t, proc.BaseURL+"/api/v1/targets/events", nil)
		mt.SetResponse(http.StatusInternalServerError, "down")

		ev := sse.WaitEvent(t, 5*time.Second, func(e sseEvent) bool {
			return e.Event == "alert" && strings.Contains(e.Data, "SSE Alert Target")
		}, "SSE alert event on 500")

		var res apiCheckResult
		if err := json.Unmarshal([]byte(ev.Data), &res); err != nil {
			t.Fatalf("unmarshal SSE alert data: %v\ndata: %s", err, ev.Data)
		}
		if res.IsUp || res.StatusCode != 500 {
			t.Fatalf("expected IsUp=false StatusCode=500 in alert, got %+v", res)
		}
	})

	t.Run("4_checker_honors_enabled_false", func(t *testing.T) {
		mtEnabled := newMockTarget(t, nil)
		mtDisabled := newMockTarget(t, nil)
		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "120ms"})

		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "Enabled Target",
			URL:            mtEnabled.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})
		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "Disabled Target",
			URL:            mtDisabled.URL(),
			Method:         "GET",
			Enabled:        false,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})

		waitForCondition(t, 5*time.Second, 50*time.Millisecond, "enabled target checked >= 4 times", func() bool {
			return mtEnabled.RequestCount() >= 4
		})
		if got := mtDisabled.RequestCount(); got != 0 {
			t.Fatalf("expected 0 requests to disabled target, got %d", got)
		}
	})

	t.Run("5_jitter_produces_non_constant_inter_arrival_intervals", func(t *testing.T) {
		const baseInterval = 200 * time.Millisecond
		mt := newMockTarget(t, nil)
		proc := startAPIServer(t, apiServerOpts{MonitorInterval: baseInterval.String()})

		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "Jitter Probe",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})

		waitForCondition(t, 6*time.Second, 50*time.Millisecond, "at least 10 jittered requests", func() bool {
			return mt.RequestCount() >= 10
		})

		ts := mt.Timestamps()
		if len(ts) < 10 {
			t.Fatalf("expected >= 10 timestamps, got %d", len(ts))
		}

		deltas := make([]time.Duration, 0, len(ts)-1)
		minD := time.Duration(1<<63 - 1)
		maxD := time.Duration(0)
		var sum time.Duration
		for i := 1; i < len(ts); i++ {
			d := ts[i].Sub(ts[i-1])
			deltas = append(deltas, d)
			sum += d
			if d < minD {
				minD = d
			}
			if d > maxD {
				maxD = d
			}
		}

		spread := maxD - minD
		avg := sum / time.Duration(len(deltas))
		t.Logf("jitter deltas=%v min=%s max=%s spread=%s avg=%s", deltas, minD, maxD, spread, avg)

		if spread < 15*time.Millisecond {
			t.Fatalf("expected non-zero jitter spread >= 15ms across %d intervals, got %s (min=%s max=%s)", len(deltas), spread, minD, maxD)
		}
		if avg < baseInterval/2 || avg > baseInterval*3 {
			t.Fatalf("average interval %s wildly outside expected range around %s", avg, baseInterval)
		}
	})
}

//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// TIER 3: Pairwise Feature Interaction Tests
// =============================================================================

func TestTier3_CRUD_Plus_Monitoring_CheckLogs(t *testing.T) {
	mtA := newMockTarget(t, nil)
	mtB := newMockTarget(t, nil)
	mtB.SetResponse(http.StatusAccepted, `{"accepted":true}`)

	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "140ms"})

	created := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Lifecycle Target A",
		URL:            mtA.URL() + "/v1",
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})

	waitForCondition(t, 5*time.Second, 50*time.Millisecond, "check logs for v1 URL", func() bool {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+created.ID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		return len(logs) >= 2
	})

	// Update target URL and expected_status
	updResp, updRaw := doJSONRequest(t, http.MethodPut, proc.BaseURL+"/api/v1/targets/"+created.ID, apiTarget{
		Name:           "Lifecycle Target B",
		URL:            mtB.URL() + "/v2",
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{202},
	}, nil)
	if updResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT target expected 200, got %d: %s", updResp.StatusCode, string(updRaw))
	}

	waitForCondition(t, 5*time.Second, 50*time.Millisecond, "new check logs reflect updated URL and status 202", func() bool {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+created.ID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		for _, l := range logs {
			if strings.HasSuffix(l.URL, "/v2") && l.StatusCode == http.StatusAccepted && l.IsUp {
				return true
			}
		}
		return false
	})

	// Delete target and confirm monitoring stops hitting mtB
	delResp, _ := doJSONRequest(t, http.MethodDelete, proc.BaseURL+"/api/v1/targets/"+created.ID, nil, nil)
	if delResp.StatusCode != http.StatusNoContent && delResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE target expected 204/200, got %d", delResp.StatusCode)
	}
	countAtDelete := mtB.RequestCount()
	time.Sleep(450 * time.Millisecond)
	// Allow at most 1 in-flight request that started right as DELETE ran
	if mtB.RequestCount() > countAtDelete+1 {
		t.Fatalf("expected monitoring to stop after DELETE, requests grew from %d to %d", countAtDelete, mtB.RequestCount())
	}
}

func TestTier3_CORS_Plus_TargetCRUD_And_CheckLogs(t *testing.T) {
	const allowedOrigin = "http://dashboard.opspulse.local:5173"
	const blockedOrigin = "http://untrusted.example.com"

	mt := newMockTarget(t, nil)
	proc := startAPIServer(t, apiServerOpts{
		CORSAllowedOrigins: allowedOrigin,
		MonitorInterval:    "150ms",
	})

	// Preflight + POST with allowed origin
	preResp, _ := doJSONRequest(t, http.MethodOptions, proc.BaseURL+"/api/v1/targets", nil, map[string]string{
		"Origin":                         allowedOrigin,
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "Content-Type",
	})
	if preResp.Header.Get("Access-Control-Allow-Origin") != allowedOrigin {
		t.Fatalf("preflight ACAO mismatch: %q", preResp.Header.Get("Access-Control-Allow-Origin"))
	}

	postResp, postRaw := doJSONRequest(t, http.MethodPost, proc.BaseURL+"/api/v1/targets", apiTarget{
		Name:           "CORS Target",
		URL:            mt.URL(),
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	}, map[string]string{"Origin": allowedOrigin})
	if postResp.StatusCode != http.StatusCreated && postResp.StatusCode != http.StatusOK {
		t.Fatalf("POST expected 201/200, got %d: %s", postResp.StatusCode, string(postRaw))
	}
	if acao := postResp.Header.Get("Access-Control-Allow-Origin"); acao != allowedOrigin {
		t.Fatalf("POST ACAO expected %q, got %q", allowedOrigin, acao)
	}
	created := decodeItem[apiTarget](t, postRaw)

	// GET targets with blocked origin must NOT leak ACAO
	getBad, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets", nil, map[string]string{"Origin": blockedOrigin})
	if acao := getBad.Header.Get("Access-Control-Allow-Origin"); acao == "*" || acao == blockedOrigin {
		t.Fatalf("GET /targets leaked CORS for blocked origin: %q", acao)
	}

	// PUT + GET check-logs + DELETE with allowed origin
	putResp, _ := doJSONRequest(t, http.MethodPut, proc.BaseURL+"/api/v1/targets/"+created.ID, apiTarget{
		Name:           "CORS Target Updated",
		URL:            mt.URL(),
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	}, map[string]string{"Origin": allowedOrigin})
	if putResp.Header.Get("Access-Control-Allow-Origin") != allowedOrigin {
		t.Fatalf("PUT ACAO expected %q, got %q", allowedOrigin, putResp.Header.Get("Access-Control-Allow-Origin"))
	}

	logsResp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs", nil, map[string]string{"Origin": allowedOrigin})
	if logsResp.Header.Get("Access-Control-Allow-Origin") != allowedOrigin {
		t.Fatalf("GET /check-logs ACAO expected %q, got %q", allowedOrigin, logsResp.Header.Get("Access-Control-Allow-Origin"))
	}

	delResp, _ := doJSONRequest(t, http.MethodDelete, proc.BaseURL+"/api/v1/targets/"+created.ID, nil, map[string]string{"Origin": allowedOrigin})
	if delResp.Header.Get("Access-Control-Allow-Origin") != allowedOrigin {
		t.Fatalf("DELETE ACAO expected %q, got %q", allowedOrigin, delResp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestTier3_CombinedCheckLogFilters(t *testing.T) {
	mt200 := newMockTarget(t, nil)
	mt500 := newMockTarget(t, nil)
	mt500.SetResponse(http.StatusInternalServerError, "err")

	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "140ms"})
	startWindow := time.Now().UTC().Add(-5 * time.Second)

	t200 := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Filter-200",
		URL:            mt200.URL(),
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})
	t500 := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Filter-500",
		URL:            mt500.URL(),
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})

	waitForCondition(t, 6*time.Second, 50*time.Millisecond, "at least 4 failure logs for t500", func() bool {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+t500.ID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		return len(logs) >= 4
	})

	endWindow := time.Now().UTC().Add(5 * time.Second)
	query := fmt.Sprintf(
		"%s/api/v1/check-logs?target_id=%s&is_up=false&status_code=500&from=%s&to=%s&limit=2&offset=1",
		proc.BaseURL, t500.ID, startWindow.Format(time.RFC3339), endWindow.Format(time.RFC3339),
	)
	resp, raw := doJSONRequest(t, http.MethodGet, query, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("combined filter expected 200, got %d: %s", resp.StatusCode, string(raw))
	}
	logs, pag := decodeList[apiCheckLog](t, raw)
	if len(logs) != 2 {
		t.Fatalf("expected 2 paginated logs, got %d", len(logs))
	}
	for _, l := range logs {
		if l.TargetID != t500.ID || l.IsUp || l.StatusCode != 500 || l.TargetID == t200.ID {
			t.Fatalf("combined filter returned mismatched log: %+v", l)
		}
	}
	if pag != nil && pag.Total < 4 {
		t.Fatalf("expected pagination total >= 4, got %d", pag.Total)
	}
}

func TestTier3_OpsPulse_Plus_API_SharedMongoDB(t *testing.T) {
	mongoSrv := startMongoFake(t)
	mt := newMockTarget(t, nil)

	const sharedDB = "shared_opspulse_e2e"
	// API runs with very long interval so checks observed on mt come from opspulse worker!
	apiProc := startAPIServer(t, apiServerOpts{
		MongoServer:     mongoSrv,
		DBName:          sharedDB,
		MonitorInterval: "10m",
	})

	created := createTargetViaAPI(t, apiProc.BaseURL, apiTarget{
		Name:           "Shared Mongo Target",
		URL:            mt.URL(),
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})

	mt.ResetRequests()

	wp := startOpsPulseWorker(t, workerOpts{
		MongoServer:     mongoSrv,
		DBName:          sharedDB,
		MonitorInterval: "150ms",
		TargetsFile:     "/nonexistent/missing.json",
	})

	waitForCondition(t, 5*time.Second, 50*time.Millisecond, "opspulse worker hits target seeded via API", func() bool {
		return mt.RequestCount() >= 2 && wp.Alive()
	})

	// Check logs persisted by worker should be queryable via API
	waitForCondition(t, 5*time.Second, 50*time.Millisecond, "check logs visible via API for worker target", func() bool {
		resp, raw := doJSONRequest(t, http.MethodGet, apiProc.BaseURL+"/api/v1/check-logs?target_id="+created.ID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		return len(logs) >= 1
	})
}

func TestTier3_Jitter_Plus_MultiTargetConcurrency(t *testing.T) {
	const baseInterval = 200 * time.Millisecond
	proc := startAPIServer(t, apiServerOpts{MonitorInterval: baseInterval.String()})

	mocks := make([]*mockTarget, 4)
	for i := range mocks {
		mocks[i] = newMockTarget(t, nil)
		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           fmt.Sprintf("MultiTarget-%d", i),
			URL:            mocks[i].URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})
	}

	waitForCondition(t, 7*time.Second, 50*time.Millisecond, "all 4 targets hit >= 8 times", func() bool {
		for _, mt := range mocks {
			if mt.RequestCount() < 8 {
				return false
			}
		}
		return true
	})

	// Verify logs written for all 4 targets and timestamps show jitter spread
	resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?limit=200", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /check-logs expected 200, got %d", resp.StatusCode)
	}
	logs, _ := decodeList[apiCheckLog](t, raw)
	if len(logs) < 16 {
		t.Fatalf("expected >= 16 logs across 4 targets, got %d", len(logs))
	}

	ts := mocks[0].Timestamps()
	minD, maxD := time.Duration(1<<63-1), time.Duration(0)
	for i := 1; i < len(ts); i++ {
		d := ts[i].Sub(ts[i-1])
		if d < minD {
			minD = d
		}
		if d > maxD {
			maxD = d
		}
	}
	if maxD-minD < 15*time.Millisecond {
		t.Fatalf("expected jitter spread >= 15ms on concurrent targets, got %s (min=%s max=%s)", maxD-minD, minD, maxD)
	}
}

func TestTier3_PATCH_And_CustomTargetFields_Plus_CheckLogs(t *testing.T) {
	mt := newMockTarget(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"healthy-pong"}`))
	})

	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "140ms"})

	created := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:              "Custom PATCH Target",
		URL:               mt.URL() + "/webhook",
		Method:            "POST",
		Headers:           map[string]string{"X-Probe-Token": "secret-123"},
		Enabled:           false,
		Tags:              []string{"webhook", "patch"},
		Body:              map[string]any{"probe": "ping", "version": 1},
		Timeout:           int64(2 * time.Second),
		ExpectedStatus:    []int{201},
		ExpectedBodyMatch: "healthy-pong",
	})

	// While disabled, monitoring must not hit target
	time.Sleep(320 * time.Millisecond)
	if got := mt.RequestCount(); got != 0 {
		t.Fatalf("expected 0 requests while enabled=false, got %d", got)
	}

	sse := openSSEStream(t, proc.BaseURL+"/api/v1/targets/events", nil)

	// Partial PATCH to enable target without resending full payload
	patchResp, patchRaw := doJSONRequest(t, http.MethodPatch, proc.BaseURL+"/api/v1/targets/"+created.ID, map[string]any{
		"enabled": true,
	}, nil)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH enable expected 200, got %d: %s", patchResp.StatusCode, string(patchRaw))
	}
	patched := decodeItem[apiTarget](t, patchRaw)
	if !patched.Enabled {
		t.Fatalf("expected patched.Enabled=true, got false: %+v", patched)
	}
	if patched.Method != "POST" || patched.ExpectedBodyMatch != "healthy-pong" || patched.Headers["X-Probe-Token"] != "secret-123" {
		t.Fatalf("PATCH overwrote existing custom fields: %+v", patched)
	}

	waitForCondition(t, 5*time.Second, 50*time.Millisecond, "custom POST target checked >= 2 times after PATCH enable", func() bool {
		return mt.RequestCount() >= 2
	})

	reqs := mt.Requests()
	if len(reqs) == 0 {
		t.Fatal("expected recorded requests on mockTarget")
	}
	req := reqs[0]
	if req.Method != http.MethodPost {
		t.Fatalf("expected checker to send POST, got %s", req.Method)
	}
	if req.Headers.Get("X-Probe-Token") != "secret-123" {
		t.Fatalf("expected header X-Probe-Token=secret-123, got %q", req.Headers.Get("X-Probe-Token"))
	}
	if !strings.Contains(req.Headers.Get("Content-Type"), "application/json") {
		t.Fatalf("expected Content-Type application/json, got %q", req.Headers.Get("Content-Type"))
	}
	if !strings.Contains(req.Body, `"probe":"ping"`) {
		t.Fatalf("expected JSON body with probe=ping, got %q", req.Body)
	}

	// Verify SSE status event reports IsUp=true and StatusCode=201
	sse.WaitEvent(t, 5*time.Second, func(e sseEvent) bool {
		if e.Event != "status" {
			return false
		}
		var results []apiCheckResult
		if err := json.Unmarshal([]byte(e.Data), &results); err != nil {
			return false
		}
		for _, r := range results {
			if r.Name == "Custom PATCH Target" && r.IsUp && r.StatusCode == http.StatusCreated {
				return true
			}
		}
		return false
	}, "SSE status event with Custom PATCH Target IsUp=true and StatusCode=201")

	// Verify check-logs persisted with target_id, is_up=true, status_code=201
	waitForCondition(t, 5*time.Second, 50*time.Millisecond, "check logs for custom POST target", func() bool {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+created.ID+"&is_up=true&status_code=201", nil, nil)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		return len(logs) >= 1
	})

	// Partial PATCH to disable target and confirm checks stop
	disResp, disRaw := doJSONRequest(t, http.MethodPatch, proc.BaseURL+"/api/v1/targets/"+created.ID, map[string]any{
		"enabled": false,
	}, nil)
	if disResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH disable expected 200, got %d: %s", disResp.StatusCode, string(disRaw))
	}
	disabled := decodeItem[apiTarget](t, disRaw)
	if disabled.Enabled || disabled.Method != "POST" {
		t.Fatalf("unexpected target after PATCH disable: %+v", disabled)
	}

	countAtDisable := mt.RequestCount()
	time.Sleep(420 * time.Millisecond)
	if mt.RequestCount() > countAtDisable+1 {
		t.Fatalf("expected checks to stop after PATCH enabled=false, grew from %d to %d", countAtDisable, mt.RequestCount())
	}
}

// =============================================================================
// TIER 4: End-to-End Real-World Scenarios
// =============================================================================

func TestTier4_Scenario1_ProductionTargetLifecycle(t *testing.T) {
	mt1 := newMockTarget(t, nil)
	mt2 := newMockTarget(t, nil)
	mt3 := newMockTarget(t, nil)

	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "180ms"})

	t1 := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Orders API",
		URL:            mt1.URL(),
		Method:         "GET",
		Enabled:        true,
		Tags:           []string{"prod"},
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})
	_ = createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Inventory API",
		URL:            mt2.URL(),
		Method:         "GET",
		Enabled:        true,
		Tags:           []string{"prod"},
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})
	t3 := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Canary Service",
		URL:            mt3.URL(),
		Method:         "GET",
		Enabled:        false,
		Tags:           []string{"canary"},
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})

	// Wait for multiple cycles on mt1 and mt2; mt3 must remain at 0
	waitForCondition(t, 6*time.Second, 50*time.Millisecond, "t1 and t2 checked >= 8 times", func() bool {
		return mt1.RequestCount() >= 8 && mt2.RequestCount() >= 8
	})
	if mt3.RequestCount() != 0 {
		t.Fatalf("expected 0 requests on disabled t3, got %d", mt3.RequestCount())
	}

	// Verify jitter spread on mt1
	ts1 := mt1.Timestamps()
	minD, maxD := time.Duration(1<<63-1), time.Duration(0)
	for i := 1; i < len(ts1); i++ {
		d := ts1[i].Sub(ts1[i-1])
		if d < minD {
			minD = d
		}
		if d > maxD {
			maxD = d
		}
	}
	if maxD-minD < 15*time.Millisecond {
		t.Fatalf("expected jitter spread >= 15ms, got %s (min=%s max=%s)", maxD-minD, minD, maxD)
	}

	// Verify /targets/status and /check-logs
	statusResp, statusRaw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets/status", nil, nil)
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("/targets/status expected 200, got %d", statusResp.StatusCode)
	}
	var statuses []apiCheckResult
	if err := json.Unmarshal(statusRaw, &statuses); err != nil {
		t.Fatalf("unmarshal /targets/status: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 enabled targets in /targets/status, got %d", len(statuses))
	}

	logsResp, logsRaw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?limit=100", nil, nil)
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("/check-logs expected 200, got %d", logsResp.StatusCode)
	}
	logs, _ := decodeList[apiCheckLog](t, logsRaw)
	if len(logs) < 12 {
		t.Fatalf("expected >= 12 check logs, got %d", len(logs))
	}

	// Enable t3 via PUT and verify it starts receiving checks
	updResp, updRaw := doJSONRequest(t, http.MethodPut, proc.BaseURL+"/api/v1/targets/"+t3.ID, apiTarget{
		Name:           t3.Name,
		URL:            t3.URL,
		Method:         "GET",
		Enabled:        true,
		Tags:           t3.Tags,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	}, nil)
	if updResp.StatusCode != http.StatusOK {
		t.Fatalf("enable t3 expected 200, got %d: %s", updResp.StatusCode, string(updRaw))
	}
	waitForCondition(t, 5*time.Second, 50*time.Millisecond, "t3 starts receiving checks after enable", func() bool {
		return mt3.RequestCount() >= 2
	})

	// Delete t1 and verify checks stop for deleted target
	delResp, _ := doJSONRequest(t, http.MethodDelete, proc.BaseURL+"/api/v1/targets/"+t1.ID, nil, nil)
	if delResp.StatusCode != http.StatusNoContent && delResp.StatusCode != http.StatusOK {
		t.Fatalf("delete t1 expected 204/200, got %d", delResp.StatusCode)
	}
	t1CountAtDelete := mt1.RequestCount()
	time.Sleep(500 * time.Millisecond)
	if mt1.RequestCount() > t1CountAtDelete+1 {
		t.Fatalf("expected checks on deleted t1 to stop, grew from %d to %d", t1CountAtDelete, mt1.RequestCount())
	}
}

func TestTier4_Scenario2_OutageAlertingAndRecovery(t *testing.T) {
	mt := newMockTarget(t, nil)
	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "150ms"})

	created := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Payment Gateway",
		URL:            mt.URL(),
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})

	sse := openSSEStream(t, proc.BaseURL+"/api/v1/targets/events", nil)
	sse.WaitEvent(t, 4*time.Second, func(e sseEvent) bool {
		return e.Event == "status" && strings.Contains(e.Data, "Payment Gateway")
	}, "initial healthy status SSE event")

	// Simulate outage (500 Internal Server Error)
	mt.SetResponse(http.StatusInternalServerError, `{"error":"db down"}`)

	alertEv := sse.WaitEvent(t, 5*time.Second, func(e sseEvent) bool {
		return e.Event == "alert" && strings.Contains(e.Data, "Payment Gateway")
	}, "outage alert SSE event")
	var alertPayload apiCheckResult
	if err := json.Unmarshal([]byte(alertEv.Data), &alertPayload); err != nil {
		t.Fatalf("unmarshal alert event: %v", err)
	}
	if alertPayload.IsUp || alertPayload.StatusCode != 500 {
		t.Fatalf("expected IsUp=false StatusCode=500 in alert, got %+v", alertPayload)
	}

	waitForCondition(t, 4*time.Second, 50*time.Millisecond, "outage check-log with is_up=false", func() bool {
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+created.ID+"&is_up=false", nil, nil)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		return len(logs) >= 1
	})

	// Simulate recovery (200 OK)
	mt.SetResponse(http.StatusOK, `{"ok":true}`)
	recoveryMark := time.Now().UTC()

	sse.WaitEvent(t, 5*time.Second, func(e sseEvent) bool {
		if e.Event != "status" {
			return false
		}
		var res []apiCheckResult
		if err := json.Unmarshal([]byte(e.Data), &res); err != nil {
			return false
		}
		for _, r := range res {
			if r.Name == "Payment Gateway" && r.IsUp && r.StatusCode == 200 {
				return true
			}
		}
		return false
	}, "recovery status SSE event with IsUp=true")

	waitForCondition(t, 4*time.Second, 50*time.Millisecond, "recovery check-log with is_up=true after recoveryMark", func() bool {
		from := recoveryMark.Format(time.RFC3339Nano)
		resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+created.ID+"&is_up=true&from="+from, nil, nil)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		logs, _ := decodeList[apiCheckLog](t, raw)
		return len(logs) >= 1
	})
}

func doJSONRequestErr(method, url string, body any, headers map[string]string) (*http.Response, []byte, error) {
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
				return nil, nil, fmt.Errorf("json.Marshal request: %w", err)
			}
			bodyReader = bytes.NewReader(b)
		}
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("http.NewRequest(%s, %s): %w", method, url, err)
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
		return nil, nil, fmt.Errorf("HTTP %s %s failed: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response body for %s %s: %w", method, url, err)
	}
	return resp, respBytes, nil
}

func TestTier4_Scenario4_ConcurrentMutationsSSEAndGracefulShutdown(t *testing.T) {
	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "120ms"})

	baseMock := newMockTarget(t, nil)
	seed := createTargetViaAPI(t, proc.BaseURL, apiTarget{
		Name:           "Baseline Service",
		URL:            baseMock.URL(),
		Method:         "GET",
		Enabled:        true,
		Timeout:        int64(2 * time.Second),
		ExpectedStatus: []int{200},
	})

	workerMocks := make([]*mockTarget, 4)
	for i := range workerMocks {
		workerMocks[i] = newMockTarget(t, nil)
	}

	// Two concurrent SSE clients
	sse1 := openSSEStream(t, proc.BaseURL+"/api/v1/targets/events", nil)
	sse2 := openSSEStream(t, proc.BaseURL+"/api/v1/targets/events", nil)

	var wg sync.WaitGroup
	errCh := make(chan error, 32)

	// Concurrent target CRUD goroutines (never call t.Fatalf inside child goroutines)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(idx int, mt *mockTarget) {
			defer wg.Done()
			postResp, postRaw, err := doJSONRequestErr(http.MethodPost, proc.BaseURL+"/api/v1/targets", apiTarget{
				Name:           fmt.Sprintf("Worker-%d", idx),
				URL:            fmt.Sprintf("%s/w/%d", mt.URL(), idx),
				Method:         "GET",
				Enabled:        true,
				Timeout:        int64(2 * time.Second),
				ExpectedStatus: []int{200},
			}, nil)
			if err != nil {
				errCh <- fmt.Errorf("worker %d POST error: %w", idx, err)
				return
			}
			if postResp.StatusCode != http.StatusCreated && postResp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("worker %d POST status %d: %s", idx, postResp.StatusCode, string(postRaw))
				return
			}
			var created apiTarget
			var probe map[string]json.RawMessage
			if err := json.Unmarshal(postRaw, &probe); err == nil && len(probe["data"]) > 0 {
				if err := json.Unmarshal(probe["data"], &created); err != nil {
					errCh <- fmt.Errorf("worker %d unmarshal data: %w", idx, err)
					return
				}
			} else if err := json.Unmarshal(postRaw, &created); err != nil {
				errCh <- fmt.Errorf("worker %d unmarshal target: %w", idx, err)
				return
			}
			if created.ID == "" {
				errCh <- fmt.Errorf("worker %d POST returned empty id: %s", idx, string(postRaw))
				return
			}

			putResp, putRaw, err := doJSONRequestErr(http.MethodPut, proc.BaseURL+"/api/v1/targets/"+created.ID, apiTarget{
				Name:           fmt.Sprintf("Worker-%d-Updated", idx),
				URL:            fmt.Sprintf("%s/w/%d", mt.URL(), idx),
				Method:         "GET",
				Enabled:        true,
				Timeout:        int64(2 * time.Second),
				ExpectedStatus: []int{200},
			}, nil)
			if err != nil {
				errCh <- fmt.Errorf("worker %d PUT error: %w", idx, err)
				return
			}
			if putResp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("worker %d PUT status %d: %s", idx, putResp.StatusCode, string(putRaw))
				return
			}

			getResp, getRaw, err := doJSONRequestErr(http.MethodGet, proc.BaseURL+"/api/v1/targets", nil, nil)
			if err != nil {
				errCh <- fmt.Errorf("worker %d GET targets error: %w", idx, err)
				return
			}
			if getResp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("worker %d GET targets status %d: %s", idx, getResp.StatusCode, string(getRaw))
				return
			}
		}(i, workerMocks[i])
	}

	// Concurrent check-logs reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 6; i++ {
			resp, raw, err := doJSONRequestErr(http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+seed.ID, nil, nil)
			if err != nil {
				errCh <- fmt.Errorf("concurrent check-logs GET error: %w", err)
				return
			}
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("concurrent check-logs GET expected 200, got %d: %s", resp.StatusCode, string(raw))
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	sse1.WaitEvent(t, 4*time.Second, func(e sseEvent) bool { return e.Event == "status" }, "sse1 status")
	sse2.WaitEvent(t, 4*time.Second, func(e sseEvent) bool { return e.Event == "status" }, "sse2 status")
	wg.Wait()
	close(errCh)

	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		t.Fatalf("concurrent goroutine errors:\n%s", strings.Join(errs, "\n"))
	}

	sse1.Close()
	sse2.Close()

	// Send SIGTERM and assert clean graceful shutdown with exit code 0
	if err := proc.Stop(5 * time.Second); err != nil {
		t.Fatalf("expected graceful SIGTERM exit code 0, got error: %v\nlogs:\n%s", err, proc.Logs())
	}
}

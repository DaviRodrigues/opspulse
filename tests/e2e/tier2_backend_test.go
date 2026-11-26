//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestTier2_F2_PersistenceBoundaries(t *testing.T) {
	t.Run("1_startup_fails_cleanly_when_mongodb_unreachable", func(t *testing.T) {
		unusedPort := freeTCPPort(t)
		proc := startAPIServer(t, apiServerOpts{
			DBURI:         fmt.Sprintf("mongodb://127.0.0.1:%s/?directConnection=true", unusedPort),
			DBTimeout:     "500ms",
			SkipReadyWait: true,
		})

		select {
		case err := <-proc.doneCh:
			proc.doneCh <- err
			if err == nil {
				t.Fatal("expected non-zero exit when MongoDB is unreachable, got exit 0")
			}
		case <-time.After(6 * time.Second):
			t.Fatalf("expected api process to fail fast when MongoDB is unreachable\nlogs:\n%s", proc.Logs())
		}
	})

	t.Run("2_unique_url_duplicate_rejected", func(t *testing.T) {
		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "10s"})
		mt := newMockTarget(t, nil)

		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "Unique One",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})

		resp, raw := doJSONRequest(t, http.MethodPost, proc.BaseURL+"/api/v1/targets", apiTarget{
			Name:           "Unique Duplicate",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		}, nil)
		if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 409 or 400 for duplicate URL, got %d: %s", resp.StatusCode, string(raw))
		}
	})

	t.Run("3_empty_db_returns_empty_lists_not_500", func(t *testing.T) {
		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "10s"})

		respT, rawT := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets", nil, nil)
		if respT.StatusCode != http.StatusOK {
			t.Fatalf("GET /targets expected 200, got %d: %s", respT.StatusCode, string(rawT))
		}
		targets, _ := decodeList[apiTarget](t, rawT)
		if len(targets) != 0 {
			t.Fatalf("expected 0 targets, got %d", len(targets))
		}

		respL, rawL := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs", nil, nil)
		if respL.StatusCode != http.StatusOK {
			t.Fatalf("GET /check-logs expected 200, got %d: %s", respL.StatusCode, string(rawL))
		}
		logs, _ := decodeList[apiCheckLog](t, rawL)
		if len(logs) != 0 {
			t.Fatalf("expected 0 check logs, got %d", len(logs))
		}

		respS, rawS := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets/status", nil, nil)
		if respS.StatusCode != http.StatusOK {
			t.Fatalf("GET /targets/status expected 200, got %d: %s", respS.StatusCode, string(rawS))
		}
	})

	t.Run("4_disabled_target_persists_and_honored_across_restart", func(t *testing.T) {
		mongoSrv := startMongoFake(t)
		mt := newMockTarget(t, nil)

		proc1 := startAPIServer(t, apiServerOpts{
			MongoServer:     mongoSrv,
			DBName:          "disabled_persist_db",
			MonitorInterval: "120ms",
		})
		created := createTargetViaAPI(t, proc1.BaseURL, apiTarget{
			Name:           "Initially Disabled",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        false,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})
		if err := proc1.Stop(5 * time.Second); err != nil {
			t.Fatalf("proc1.Stop: %v", err)
		}

		proc2 := startAPIServer(t, apiServerOpts{
			MongoServer:     mongoSrv,
			DBName:          "disabled_persist_db",
			MonitorInterval: "120ms",
		})
		time.Sleep(450 * time.Millisecond)

		resp, raw := doJSONRequest(t, http.MethodGet, proc2.BaseURL+"/api/v1/targets/"+created.ID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		got := decodeItem[apiTarget](t, raw)
		if got.Enabled {
			t.Fatal("expected target enabled=false to persist across restart")
		}
		if mt.RequestCount() != 0 {
			t.Fatalf("expected 0 requests to disabled target across restart, got %d", mt.RequestCount())
		}
	})

	t.Run("5_opspulse_handles_empty_targets_collection_gracefully", func(t *testing.T) {
		mongoSrv := startMongoFake(t)
		wp := startOpsPulseWorker(t, workerOpts{
			MongoServer:     mongoSrv,
			DBName:          "empty_worker_db",
			MonitorInterval: "150ms",
		})
		time.Sleep(500 * time.Millisecond)
		if !wp.Alive() {
			t.Fatal("expected opspulse worker to remain alive when targets collection is empty")
		}
	})
}

func TestTier2_F3_HealthAndCORSBoundaries(t *testing.T) {
	proc := startAPIServer(t, apiServerOpts{
		CORSAllowedOrigins: "http://a.test , http://b.test ",
		MonitorInterval:    "10s",
	})

	t.Run("1_disallowed_origin_not_echoed_and_not_wildcard", func(t *testing.T) {
		evil := "https://evil.attacker.test"
		resp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/targets", nil, map[string]string{
			"Origin": evil,
		})
		acao := resp.Header.Get("Access-Control-Allow-Origin")
		if acao == "*" || acao == evil {
			t.Fatalf("CORS leaked disallowed origin or wildcard: %q", acao)
		}

		preResp, _ := doJSONRequest(t, http.MethodOptions, proc.BaseURL+"/api/v1/targets", nil, map[string]string{
			"Origin":                        evil,
			"Access-Control-Request-Method": "POST",
		})
		preACAO := preResp.Header.Get("Access-Control-Allow-Origin")
		if preACAO == "*" || preACAO == evil {
			t.Fatalf("preflight CORS leaked disallowed origin or wildcard: %q", preACAO)
		}
	})

	t.Run("2_sse_disallowed_origin_does_not_receive_wildcard", func(t *testing.T) {
		evil := "https://evil-sse.test"
		resp, err := func() (*http.Response, error) {
			req, err := http.NewRequest(http.MethodGet, proc.BaseURL+"/api/v1/targets/events", nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Origin", evil)
			client := &http.Client{Timeout: 2 * time.Second}
			return client.Do(req)
		}()
		if err != nil {
			t.Fatalf("SSE request failed: %v", err)
		}
		_ = resp.Body.Close()
		acao := resp.Header.Get("Access-Control-Allow-Origin")
		if acao == "*" || acao == evil {
			t.Fatalf("SSE CORS leaked disallowed origin or wildcard: %q", acao)
		}
	})

	t.Run("3_requests_without_origin_succeed", func(t *testing.T) {
		resp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/health", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("4_cors_origins_with_whitespace_around_commas_work", func(t *testing.T) {
		for _, origin := range []string{"http://a.test", "http://b.test"} {
			resp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/health", nil, map[string]string{
				"Origin": origin,
			})
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
				t.Fatalf("expected Access-Control-Allow-Origin=%q after trimming whitespace, got %q", origin, got)
			}
		}
	})

	t.Run("5_unknown_route_returns_404", func(t *testing.T) {
		resp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/nonexistent", nil, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})
}

func TestTier2_F4_TargetsCRUDBoundaries(t *testing.T) {
	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "10s"})
	mt := newMockTarget(t, nil)

	t.Run("1_malformed_json_returns_400", func(t *testing.T) {
		resp, _ := doJSONRequest(t, http.MethodPost, proc.BaseURL+"/api/v1/targets", `{bad json`, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for malformed JSON, got %d", resp.StatusCode)
		}
	})

	t.Run("2_missing_required_name_or_url_returns_400", func(t *testing.T) {
		respNoName, _ := doJSONRequest(t, http.MethodPost, proc.BaseURL+"/api/v1/targets", apiTarget{
			URL:     mt.URL(),
			Enabled: true,
		}, nil)
		if respNoName.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing name, got %d", respNoName.StatusCode)
		}

		respNoURL, _ := doJSONRequest(t, http.MethodPost, proc.BaseURL+"/api/v1/targets", apiTarget{
			Name:    "NoURL",
			Enabled: true,
		}, nil)
		if respNoURL.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing url, got %d", respNoURL.StatusCode)
		}
	})

	t.Run("3_invalid_url_scheme_returns_400", func(t *testing.T) {
		for _, badURL := range []string{"not-a-url", "ftp://example.com", "javascript:alert(1)"} {
			resp, _ := doJSONRequest(t, http.MethodPost, proc.BaseURL+"/api/v1/targets", apiTarget{
				Name:    "Bad Scheme",
				URL:     badURL,
				Enabled: true,
			}, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid URL %q, got %d", badURL, resp.StatusCode)
			}
		}
	})

	t.Run("4_invalid_objectid_on_get_put_delete_returns_400_or_404", func(t *testing.T) {
		badID := "not-an-object-id"
		for _, tc := range []struct {
			method string
			body   any
		}{
			{http.MethodGet, nil},
			{http.MethodPut, apiTarget{Name: "X", URL: mt.URL()}},
			{http.MethodDelete, nil},
		} {
			resp, _ := doJSONRequest(t, tc.method, proc.BaseURL+"/api/v1/targets/"+badID, tc.body, nil)
			if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
				t.Fatalf("%s /targets/%s expected 400/404, got %d", tc.method, badID, resp.StatusCode)
			}
		}
	})

	t.Run("5_nonexistent_valid_objectid_returns_404", func(t *testing.T) {
		missingID := bson.NewObjectID().Hex()
		for _, tc := range []struct {
			method string
			body   any
		}{
			{http.MethodGet, nil},
			{http.MethodPut, apiTarget{Name: "Missing", URL: mt.URL(), Method: "GET"}},
			{http.MethodDelete, nil},
		} {
			resp, _ := doJSONRequest(t, tc.method, proc.BaseURL+"/api/v1/targets/"+missingID, tc.body, nil)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("%s /targets/%s expected 404, got %d", tc.method, missingID, resp.StatusCode)
			}
		}
	})

	t.Run("6_duplicate_url_on_post_returns_409_or_400", func(t *testing.T) {
		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "First",
			URL:            mt.URL() + "/dup",
			Method:         "GET",
			Enabled:        true,
			ExpectedStatus: []int{200},
		})
		resp, _ := doJSONRequest(t, http.MethodPost, proc.BaseURL+"/api/v1/targets", apiTarget{
			Name:           "Second",
			URL:            mt.URL() + "/dup",
			Method:         "GET",
			Enabled:        true,
			ExpectedStatus: []int{200},
		}, nil)
		if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 409 or 400 on duplicate URL, got %d", resp.StatusCode)
		}
	})
}

func TestTier2_F5_CheckLogsBoundaries(t *testing.T) {
	proc := startAPIServer(t, apiServerOpts{MonitorInterval: "10s"})

	t.Run("1_invalid_target_id_filter", func(t *testing.T) {
		resp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id=invalid-id", nil, nil)
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 400 or 200 (never 500), got %d", resp.StatusCode)
		}
	})

	t.Run("2_invalid_is_up_bool_returns_400", func(t *testing.T) {
		resp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?is_up=notabool", nil, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid is_up, got %d", resp.StatusCode)
		}
	})

	t.Run("3_invalid_from_to_timestamp_returns_400", func(t *testing.T) {
		respFrom, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?from=bad-date", nil, nil)
		if respFrom.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid from timestamp, got %d", respFrom.StatusCode)
		}
		respTo, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?to=bad-date", nil, nil)
		if respTo.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid to timestamp, got %d", respTo.StatusCode)
		}
	})

	t.Run("4_limit_and_offset_edge_cases_never_500", func(t *testing.T) {
		for _, q := range []string{"limit=0", "limit=-5", "offset=999999", "offset=-1"} {
			resp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?"+q, nil, nil)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("query %q expected 200 or 400, got %d", q, resp.StatusCode)
			}
		}
	})

	t.Run("5_get_check_log_by_invalid_and_missing_id", func(t *testing.T) {
		respBad, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs/not-valid", nil, nil)
		if respBad.StatusCode != http.StatusBadRequest && respBad.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 400 or 404, got %d", respBad.StatusCode)
		}

		missingID := bson.NewObjectID().Hex()
		respMiss, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs/"+missingID, nil, nil)
		if respMiss.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for nonexistent check log, got %d", respMiss.StatusCode)
		}
	})
}

func TestTier2_F6_JitterAndSSEBoundaries(t *testing.T) {
	t.Run("1_five_concurrent_targets_staggered_no_crash", func(t *testing.T) {
		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "200ms"})
		targets := make([]*mockTarget, 5)
		for i := range targets {
			targets[i] = newMockTarget(t, nil)
			createTargetViaAPI(t, proc.BaseURL, apiTarget{
				Name:           fmt.Sprintf("Concurrent-%d", i),
				URL:            targets[i].URL(),
				Method:         "GET",
				Enabled:        true,
				Timeout:        int64(2 * time.Second),
				ExpectedStatus: []int{200},
			})
		}

		waitForCondition(t, 6*time.Second, 50*time.Millisecond, "all 5 targets checked >= 2 times", func() bool {
			for _, mt := range targets {
				if mt.RequestCount() < 2 {
					return false
				}
			}
			return true
		})
	})

	t.Run("2_slow_timeout_target_records_failed_check_log", func(t *testing.T) {
		mtSlow := newMockTarget(t, nil)
		mtSlow.SetDelay(500 * time.Millisecond)

		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "200ms"})
		created := createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "Slow Timeout Target",
			URL:            mtSlow.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(80 * time.Millisecond),
			ExpectedStatus: []int{200},
		})

		var foundDown *apiCheckLog
		waitForCondition(t, 5*time.Second, 50*time.Millisecond, "timeout CheckLog with is_up=false", func() bool {
			resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+created.ID, nil, nil)
			if resp.StatusCode != http.StatusOK {
				return false
			}
			logs, _ := decodeList[apiCheckLog](t, raw)
			for i := range logs {
				if !logs[i].IsUp && logs[i].Error != "" {
					foundDown = &logs[i]
					return true
				}
			}
			return false
		})
		if foundDown == nil || foundDown.IsUp {
			t.Fatalf("expected failed CheckLog with error, got %+v", foundDown)
		}
	})

	t.Run("3_unexpected_status_code_sets_is_up_false", func(t *testing.T) {
		mt503 := newMockTarget(t, nil)
		mt503.SetResponse(http.StatusServiceUnavailable, "unavailable")

		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "150ms"})
		created := createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "Unexpected 503",
			URL:            mt503.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})

		waitForCondition(t, 5*time.Second, 50*time.Millisecond, "503 CheckLog recorded with is_up=false", func() bool {
			resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+created.ID, nil, nil)
			if resp.StatusCode != http.StatusOK {
				return false
			}
			logs, _ := decodeList[apiCheckLog](t, raw)
			for _, l := range logs {
				if l.StatusCode == http.StatusServiceUnavailable && !l.IsUp {
					return true
				}
			}
			return false
		})
	})

	t.Run("4_expected_body_match_mismatch_marks_is_up_false", func(t *testing.T) {
		mtMismatch := newMockTarget(t, nil)
		mtMismatch.SetResponse(http.StatusOK, `{"status":"degraded"}`)

		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "150ms"})
		created := createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:              "Body Match Target",
			URL:               mtMismatch.URL(),
			Method:            "GET",
			Enabled:           true,
			Timeout:           int64(2 * time.Second),
			ExpectedStatus:    []int{200},
			ExpectedBodyMatch: `"status":"healthy"`,
		})

		waitForCondition(t, 5*time.Second, 50*time.Millisecond, "body mismatch CheckLog with is_up=false", func() bool {
			resp, raw := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/check-logs?target_id="+created.ID, nil, nil)
			if resp.StatusCode != http.StatusOK {
				return false
			}
			logs, _ := decodeList[apiCheckLog](t, raw)
			for _, l := range logs {
				if !l.IsUp && strings.TrimSpace(l.Error) != "" {
					return true
				}
			}
			return false
		})
	})

	t.Run("5_sse_client_disconnect_cleans_up_without_panic", func(t *testing.T) {
		mt := newMockTarget(t, nil)
		proc := startAPIServer(t, apiServerOpts{MonitorInterval: "120ms"})
		createTargetViaAPI(t, proc.BaseURL, apiTarget{
			Name:           "Disconnect Probe",
			URL:            mt.URL(),
			Method:         "GET",
			Enabled:        true,
			Timeout:        int64(2 * time.Second),
			ExpectedStatus: []int{200},
		})

		sse := openSSEStream(t, proc.BaseURL+"/api/v1/targets/events", nil)
		sse.WaitEvent(t, 4*time.Second, func(e sseEvent) bool {
			return e.Event == "status"
		}, "initial status event before disconnect")
		sse.Close()

		// Let several monitoring cycles fire after disconnect
		time.Sleep(400 * time.Millisecond)

		resp, _ := doJSONRequest(t, http.MethodGet, proc.BaseURL+"/api/v1/health", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected server healthy after SSE disconnect, got %d", resp.StatusCode)
		}
	})
}

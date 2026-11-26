import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { normalizeBaseUrl, buildEndpointUrl } from "../src/lib/url.ts";
import {
  DEFAULT_EXPECTED_STATUS,
  DEFAULT_TIMEOUT_MS,
  formatIntervalLabel,
  formValuesToPayload,
  msToNs,
  nsToMs,
  nsToSec,
  parseExpectedStatusInput,
  parseHeadersInput,
  parseJsonBodyInput,
  parseTagsInput,
  secToNs,
  targetToFormValues,
  validateTargetForm,
} from "../src/lib/targetForm.ts";
import {
  calculatePercentile,
  computeCheckLogInsights,
  computeCheckLogsPagination,
  formatLatencyMs,
} from "../src/lib/insights.ts";
import {
  AVAILABILITY_FILTER_STORAGE_KEY,
  buildLiveResultLookup,
  classifyTargetAvailability,
  findLiveResultForTarget,
  findOrphanLiveResults,
  loadAvailabilityFilter,
  normalizeTargetMatchKey,
  pruneDeletedTargetStatus,
  resolveEffectiveLiveResults,
  saveAvailabilityFilter,
} from "../src/lib/targetStatus.ts";
import type { CheckLog, CheckResult, Target } from "../src/types/check.ts";

describe("url helpers", () => {
  it("normalizes empty, whitespace, and trailing-slash base URLs", () => {
    assert.equal(normalizeBaseUrl(undefined), "");
    assert.equal(normalizeBaseUrl("   "), "");
    assert.equal(normalizeBaseUrl("https://api.example.com/"), "https://api.example.com");
    assert.equal(
      normalizeBaseUrl("  https://e2e-sentinel-api.invalid/opspulse///  "),
      "https://e2e-sentinel-api.invalid/opspulse",
    );
  });

  it("builds endpoint URLs cleanly without double slashes", () => {
    assert.equal(buildEndpointUrl("", "/api/v1/targets"), "/api/v1/targets");
    assert.equal(buildEndpointUrl("", "api/v1/targets"), "/api/v1/targets");
    assert.equal(
      buildEndpointUrl("https://api.example.com/", "/api/v1/targets/events"),
      "https://api.example.com/api/v1/targets/events",
    );
    assert.equal(
      buildEndpointUrl("https://e2e-sentinel-api.invalid/opspulse/", "api/v1/check-logs"),
      "https://e2e-sentinel-api.invalid/opspulse/api/v1/check-logs",
    );
  });
});

describe("targetForm helpers", () => {
  it("converts nanoseconds and milliseconds safely", () => {
    assert.equal(nsToMs(5_000_000_000), 5000);
    assert.equal(nsToMs(0), 0);
    assert.equal(nsToMs(-100), 0);
    assert.equal(msToNs(2500), 2_500_000_000);
    assert.equal(msToNs(-5), 0);
  });

  it("parses expected status codes with defaults and range validation", () => {
    assert.deepEqual(parseExpectedStatusInput(""), {
      codes: DEFAULT_EXPECTED_STATUS,
    });
    assert.deepEqual(parseExpectedStatusInput("200, 201, 200, 204"), {
      codes: [200, 201, 204],
    });
    const invalidLow = parseExpectedStatusInput("99, 200");
    assert.ok(invalidLow.error);
    const invalidHigh = parseExpectedStatusInput("200, 600");
    assert.ok(invalidHigh.error);
    const nonNumeric = parseExpectedStatusInput("200, ok");
    assert.ok(nonNumeric.error);
  });

  it("parses tags deduplicating and stripping blanks", () => {
    assert.deepEqual(parseTagsInput(" prod , api, , prod , critical "), [
      "prod",
      "api",
      "critical",
    ]);
    assert.deepEqual(parseTagsInput("   "), []);
  });

  it("parses headers key-value pairs and rejects invalid empty-key values", () => {
    assert.deepEqual(parseHeadersInput([]), {});
    assert.deepEqual(
      parseHeadersInput([
        { key: " Authorization ", value: "Bearer tok" },
        { key: "  ", value: "   " },
      ]),
      { headers: { Authorization: "Bearer tok" } },
    );
    const missingKey = parseHeadersInput([{ key: "", value: "val" }]);
    assert.ok(missingKey.error);
  });

  it("parses JSON body only as plain object", () => {
    assert.deepEqual(parseJsonBodyInput("   "), {});
    assert.deepEqual(parseJsonBodyInput('{"ping":true,"count":2}'), {
      body: { ping: true, count: 2 },
    });
    assert.ok(parseJsonBodyInput("[1, 2, 3]").error);
    assert.ok(parseJsonBodyInput('"string"').error);
    assert.ok(parseJsonBodyInput("{bad json").error);
  });

  it("validates form fields and maps target <-> form <-> payload with explicit enabled", () => {
    const emptyErrors = validateTargetForm({
      name: "",
      url: "ftp://example.com",
      method: "POST",
      enabled: false,
      timeoutMs: "-10",
      intervalSec: "",
      expectedStatusText: "999",
      tagsText: "",
      expectedBodyMatch: "",
      headers: [],
      bodyText: "[]",
    });
    assert.ok(emptyErrors.name);
    assert.ok(emptyErrors.url);
    assert.ok(emptyErrors.timeoutMs);
    assert.ok(emptyErrors.expectedStatusText);
    assert.ok(emptyErrors.bodyText);

    const defaultValues = targetToFormValues(null);
    assert.equal(defaultValues.enabled, true);
    assert.equal(defaultValues.timeoutMs, String(DEFAULT_TIMEOUT_MS));

    const existing: Target = {
      id: "65f1a2b3c4d5e6f7a8b9c0d1",
      name: "Auth API",
      url: "https://auth.example.com/health",
      method: "POST",
      headers: { "X-Env": "prod" },
      enabled: false,
      tags: ["auth", "core"],
      body: { check: "deep" },
      timeout: 3_000_000_000,
      expected_status: [200, 202],
      expected_body_match: "ok",
      created_at: "2026-11-26T00:00:00Z",
      updated_at: "2026-11-26T00:00:00Z",
    };

    const form = targetToFormValues(existing);
    assert.equal(form.enabled, false);
    assert.equal(form.timeoutMs, "3000");
    assert.equal(form.expectedStatusText, "200, 202");
    assert.equal(form.tagsText, "auth, core");
    assert.deepEqual(validateTargetForm(form), {});

    const payload = formValuesToPayload(form);
    assert.strictEqual(payload.enabled, false);
    assert.equal(payload.timeout, 3_000_000_000);
    assert.deepEqual(payload.expected_status, [200, 202]);
    assert.deepEqual(payload.headers, { "X-Env": "prod" });
    assert.deepEqual(payload.body, { check: "deep" });

    const getForm = { ...form, method: "GET" as const };
    const getPayload = formValuesToPayload(getForm);
    assert.equal(getPayload.body, null);
  });

  it("rejects trailing empty colons and port 0 while allowing valid ports and IPv6 hosts", () => {
    const baseValues = targetToFormValues(null);
    baseValues.name = "Port Check";

    for (const invalidUrl of [
      "http://example.com:",
      "http://example.com:/health",
      "http://example.com:0",
      "http://[::1]:",
      "http://[::1]:0",
      "http://example.com:65536",
    ]) {
      const errs = validateTargetForm({ ...baseValues, url: invalidUrl });
      assert.ok(errs.url, `Expected URL validation error for ${invalidUrl}`);
    }

    for (const validUrl of [
      "http://example.com:80",
      "https://example.com:443/health",
      "http://example.com:65535/status",
      "http://[::1]/health",
      "http://[::1]:8080/health",
      "http://example.com/path:with:colon",
    ]) {
      const errs = validateTargetForm({ ...baseValues, url: validUrl });
      assert.equal(
        errs.url,
        undefined,
        `Expected ${validUrl} to pass URL validation, got: ${errs.url}`,
      );
    }
  });

  it("supports non-100ms timeouts like 80ms and 1550ms cleanly", () => {
    for (const [ns, expectedMs] of [
      [80_000_000, "80"],
      [1_550_000_000, "1550"],
    ] as const) {
      const form = targetToFormValues({
        id: "t-timeout",
        name: "Fast Check",
        url: "https://fast.example.com/health",
        method: "GET",
        enabled: true,
        tags: [],
        timeout: ns,
        expected_status: [200],
        expected_body_match: "",
        created_at: "2026-11-26T00:00:00Z",
        updated_at: "2026-11-26T00:00:00Z",
      });
      assert.equal(form.timeoutMs, expectedMs);
      assert.deepEqual(validateTargetForm(form), {});
      assert.equal(formValuesToPayload(form).timeout, ns);
    }
  });
});

describe("insights helpers", () => {
  it("formats latencies and calculates percentiles", () => {
    assert.equal(formatLatencyMs(0), "--");
    assert.equal(formatLatencyMs(500_000), "0.50 ms");
    assert.equal(formatLatencyMs(125_400_000), "125.40 ms");
    assert.equal(calculatePercentile([], 95), 0);
    assert.equal(calculatePercentile([10, 20, 30, 40, 100], 95), 100);
    assert.equal(calculatePercentile([10, 20, 30, 40, 50], 50), 30);
  });

  it("computes empty and populated check_logs insights accurately", () => {
    const empty = computeCheckLogInsights([]);
    assert.equal(empty.totalChecks, 0);
    assert.equal(empty.uptimePercent, 0);
    assert.equal(empty.p95LatencyMs, 0);
    assert.deepEqual(empty.topFailingTargets, []);
    assert.deepEqual(empty.slowestTargets, []);

    const logs: CheckLog[] = [
      {
        id: "1",
        target_id: "t1",
        name: "API A",
        url: "https://a.example.com",
        status_code: 200,
        latency: 100_000_000, // 100ms
        is_up: true,
        checked_at: "2026-11-26T10:00:00Z",
      },
      {
        id: "2",
        target_id: "t1",
        name: "API A",
        url: "https://a.example.com",
        status_code: 503,
        latency: 300_000_000, // 300ms
        is_up: false,
        error: "service unavailable",
        checked_at: "2026-11-26T10:01:00Z",
      },
      {
        id: "3",
        target_id: "t2",
        name: "API B",
        url: "https://b.example.com",
        status_code: 0,
        latency: 500_000_000, // 500ms timeout
        is_up: false,
        error: "timeout",
        checked_at: "2026-11-26T10:02:00Z",
      },
      {
        id: "4",
        target_id: "t3",
        name: "API C",
        url: "https://c.example.com",
        status_code: 204,
        latency: 50_000_000, // 50ms
        is_up: true,
        checked_at: "2026-11-26T10:03:00Z",
      },
    ];

    const insights = computeCheckLogInsights(logs);
    assert.equal(insights.totalChecks, 4);
    assert.equal(insights.upCount, 2);
    assert.equal(insights.downCount, 2);
    assert.equal(insights.uptimePercent, 50);
    assert.equal(insights.errorRatePercent, 50);
    assert.equal(insights.minLatencyMs, 50);
    assert.equal(insights.maxLatencyMs, 500);
    assert.equal(insights.avgLatencyMs, 237.5);
    assert.equal(insights.p95LatencyMs, 500);
    assert.deepEqual(insights.statusDistribution, {
      "2xx": 2,
      "3xx": 0,
      "4xx": 0,
      "5xx": 1,
      network_error: 1,
    });
    assert.equal(insights.topFailingTargets.length, 2);
    assert.equal(insights.topFailingTargets[0].targetId, "t2");
    assert.equal(insights.slowestTargets[0].targetId, "t2");
    assert.equal(insights.slowestTargets[0].avgLatencyMs, 500);
  });

  it("clamps CheckLogs pagination cleanly so currentPage never exceeds totalPages", () => {
    assert.deepEqual(computeCheckLogsPagination(0, 50, 0), {
      totalPages: 1,
      currentPage: 1,
      clampedOffset: 0,
    });
    assert.deepEqual(computeCheckLogsPagination(0, 50, 100), {
      totalPages: 1,
      currentPage: 1,
      clampedOffset: 0,
    });
    assert.deepEqual(computeCheckLogsPagination(40, 50, 100), {
      totalPages: 1,
      currentPage: 1,
      clampedOffset: 0,
    });
    assert.deepEqual(computeCheckLogsPagination(120, 50, 150), {
      totalPages: 3,
      currentPage: 3,
      clampedOffset: 100,
    });
    assert.deepEqual(computeCheckLogsPagination(120, 50, 50), {
      totalPages: 3,
      currentPage: 2,
      clampedOffset: 50,
    });
  });
});

describe("targetStatus helpers (F-02 & F-04)", () => {
  it("overrides initialStatus with empty SSE status [] once hasReceivedStatus is true", () => {
    const initial: CheckResult[] = [
      {
        Name: "Stale API",
        URL: "https://stale.example.com",
        StatusCode: 200,
        Latency: 25_000_000,
        IsUp: true,
      },
    ];

    assert.deepEqual(
      resolveEffectiveLiveResults({
        hasReceivedStatus: false,
        liveTargets: [],
        initialStatus: initial,
      }),
      initial,
    );

    assert.deepEqual(
      resolveEffectiveLiveResults({
        hasReceivedStatus: true,
        liveTargets: [],
        initialStatus: initial,
      }),
      [],
    );
  });

  it("prevents deleted targets from resurrecting as orphan SSE cards", () => {
    const a: CheckResult = {
      Name: "API A",
      URL: "https://a.example.com/health",
      StatusCode: 200,
      Latency: 10_000_000,
      IsUp: true,
    };
    const b: CheckResult = {
      Name: "API B",
      URL: "https://b.example.com/health",
      StatusCode: 500,
      Latency: 50_000_000,
      IsUp: false,
    };

    assert.deepEqual(
      pruneDeletedTargetStatus([a, b], "HTTPS://B.example.com/health"),
      [a],
    );

    const remaining = resolveEffectiveLiveResults({
      hasReceivedStatus: true,
      liveTargets: [a, b],
      initialStatus: [a, b],
      deletedUrls: ["https://b.example.com/health"],
    });
    assert.deepEqual(remaining, [a]);
    assert.deepEqual(
      findOrphanLiveResults(
        [{ url: "https://a.example.com/health" }],
        remaining,
      ),
      [],
    );
  });

  it("matches live results strictly by URL without byName bleed or trailing-slash collision", () => {
    assert.notEqual(
      normalizeTargetMatchKey("https://example.com/api"),
      normalizeTargetMatchKey("https://example.com/api/"),
    );
    assert.notEqual(
      normalizeTargetMatchKey("https://example.com"),
      normalizeTargetMatchKey("https://example.com/"),
    );
    assert.equal(
      normalizeTargetMatchKey("  HTTPS://Example.COM/V1/Health "),
      "https://example.com/V1/Health",
    );

    const liveResults: CheckResult[] = [
      {
        Name: "API Health",
        URL: "https://a.example.com/api",
        StatusCode: 200,
        Latency: 15_000_000,
        IsUp: true,
      },
    ];
    const lookup = buildLiveResultLookup(liveResults);

    // Same name, different URL (e.g. paused target B) must NOT match A's live result
    assert.equal(
      findLiveResultForTarget({ url: "https://b.example.com/api" }, lookup),
      undefined,
    );
    // Different trailing slash must NOT collide
    assert.equal(
      findLiveResultForTarget({ url: "https://a.example.com/api/" }, lookup),
      undefined,
    );
    // Exact URL (case-insensitive scheme/host) matches cleanly
    assert.deepEqual(
      findLiveResultForTarget({ url: "HTTPS://A.EXAMPLE.COM/api" }, lookup),
      liveResults[0],
    );
    // Orphan detection does not suppress distinct URLs sharing a name
    assert.deepEqual(
      findOrphanLiveResults(
        [{ url: "https://b.example.com/api" }],
        liveResults,
      ),
      liveResults,
    );
  });

  it("classifies availability states and persists filter preference in storage", () => {
    assert.equal(classifyTargetAvailability({ enabled: false }), "paused");
    assert.equal(classifyTargetAvailability({ enabled: true }), "pending");
    assert.equal(
      classifyTargetAvailability({ enabled: true }, { IsUp: true }),
      "up",
    );
    assert.equal(
      classifyTargetAvailability({ enabled: true }, { IsUp: false }),
      "down",
    );

    const store = new Map<string, string>();
    const fakeStorage = {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => {
        store.set(k, v);
      },
    };

    assert.equal(loadAvailabilityFilter(fakeStorage), "all");
    saveAvailabilityFilter(fakeStorage, "down");
    assert.equal(store.get(AVAILABILITY_FILTER_STORAGE_KEY), "down");
    assert.equal(loadAvailabilityFilter(fakeStorage), "down");

    store.set(AVAILABILITY_FILTER_STORAGE_KEY, "invalid-value");
    assert.equal(loadAvailabilityFilter(fakeStorage), "all");

    const throwingStorage = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
    };
    assert.equal(loadAvailabilityFilter(throwingStorage), "all");
    assert.doesNotThrow(() => saveAvailabilityFilter(throwingStorage, "up"));
  });
});

describe("M6 interval & fastestTarget helpers", () => {
  it("converts, validates, and maps per-target interval cleanly", () => {
    assert.equal(nsToSec(30_000_000_000), 30);
    assert.equal(secToNs(15), 15_000_000_000);
    assert.equal(formatIntervalLabel(0), "Padrão");
    assert.equal(formatIntervalLabel(undefined), "Padrão");
    assert.equal(formatIntervalLabel(15_000_000_000), "15s");
    assert.equal(formatIntervalLabel(120_000_000_000), "2m");

    const defaultForm = targetToFormValues(null);
    assert.equal(defaultForm.intervalSec, "");

    const payloadDefault = formValuesToPayload({
      ...defaultForm,
      name: "DefaultSvc",
      url: "https://default.example.com",
    });
    assert.equal(payloadDefault.interval, 0);

    const badErrors = validateTargetForm({
      ...defaultForm,
      name: "NegSvc",
      url: "https://neg.example.com",
      intervalSec: "-5",
    });
    assert.ok(badErrors.intervalSec);

    const customTarget: Target = {
      id: "507f1f77bcf86cd799439011",
      name: "FastSvc",
      url: "https://fast.example.com",
      method: "GET",
      enabled: true,
      tags: [],
      timeout: 5_000_000_000,
      interval: 10_000_000_000,
      expected_status: [200],
      expected_body_match: "",
      created_at: "2026-11-26T00:00:00Z",
      updated_at: "2026-11-26T00:00:00Z",
    };
    const editForm = targetToFormValues(customTarget);
    assert.equal(editForm.intervalSec, "10");
    const updatedPayload = formValuesToPayload(editForm);
    assert.equal(updatedPayload.interval, 10_000_000_000);
  });

  it("computes fastestTarget on average while ignoring zero-latency entries", () => {
    assert.equal(computeCheckLogInsights([]).fastestTarget, null);

    const zeroOnly: CheckLog[] = [
      {
        id: "1",
        target_id: "t0",
        name: "TimeoutSvc",
        url: "https://timeout.example.com",
        status_code: 0,
        latency: 0,
        is_up: false,
        checked_at: "2026-11-26T10:00:00Z",
      },
    ];
    assert.equal(computeCheckLogInsights(zeroOnly).fastestTarget, null);

    const sampleLogs: CheckLog[] = [
      {
        id: "1",
        target_id: "t-slow",
        name: "Slow API",
        url: "https://slow.example.com",
        status_code: 200,
        latency: 300_000_000,
        is_up: true,
        checked_at: "2026-11-26T10:00:00Z",
      },
      {
        id: "2",
        target_id: "t-fast",
        name: "Fast API",
        url: "https://fast.example.com",
        status_code: 200,
        latency: 20_000_000,
        is_up: true,
        checked_at: "2026-11-26T10:01:00Z",
      },
      {
        id: "3",
        target_id: "t-fast",
        name: "Fast API",
        url: "https://fast.example.com",
        status_code: 200,
        latency: 40_000_000,
        is_up: true,
        checked_at: "2026-11-26T10:02:00Z",
      },
    ];
    const insights = computeCheckLogInsights(sampleLogs);
    assert.ok(insights.fastestTarget);
    assert.equal(insights.fastestTarget.targetId, "t-fast");
    assert.equal(insights.fastestTarget.name, "Fast API");
    assert.equal(insights.fastestTarget.avgLatencyMs, 30);
  });
});

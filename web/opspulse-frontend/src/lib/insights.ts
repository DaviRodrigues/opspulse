import type { CheckLog } from "../types/check.ts";

export interface StatusDistribution {
  "2xx": number;
  "3xx": number;
  "4xx": number;
  "5xx": number;
  network_error: number;
}

export interface TargetInsightSummary {
  targetKey: string;
  targetId: string;
  name: string;
  url: string;
  total: number;
  upCount: number;
  failures: number;
  uptimePercent: number;
  avgLatencyMs: number;
}

export interface CheckLogInsights {
  totalChecks: number;
  upCount: number;
  downCount: number;
  uptimePercent: number;
  errorRatePercent: number;
  avgLatencyMs: number;
  minLatencyMs: number;
  maxLatencyMs: number;
  p95LatencyMs: number;
  statusDistribution: StatusDistribution;
  topFailingTargets: TargetInsightSummary[];
  slowestTargets: TargetInsightSummary[];
  fastestTarget: TargetInsightSummary | null;
}

export interface CheckLogsPageInfo {
  totalPages: number;
  currentPage: number;
  clampedOffset: number;
}

export function computeCheckLogsPagination(
  total: number,
  limit: number,
  offset: number,
): CheckLogsPageInfo {
  const safeLimit = Number.isFinite(limit) && limit > 0 ? Math.floor(limit) : 50;
  const safeTotal = Number.isFinite(total) && total > 0 ? Math.floor(total) : 0;
  const safeOffset = Number.isFinite(offset) && offset > 0 ? Math.floor(offset) : 0;

  const totalPages = Math.max(1, Math.ceil(safeTotal / safeLimit));
  const maxValidOffset = safeTotal > 0 ? (totalPages - 1) * safeLimit : 0;
  const clampedOffset = Math.min(safeOffset, maxValidOffset);
  const currentPage = Math.min(
    Math.floor(clampedOffset / safeLimit) + 1,
    totalPages,
  );

  return { totalPages, currentPage, clampedOffset };
}

export function formatLatencyMs(ns: number): string {
  if (!Number.isFinite(ns) || ns <= 0) {
    return "--";
  }
  const ms = ns / 1_000_000;
  if (ms < 0.01) {
    return "< 0.01 ms";
  }
  return `${ms.toFixed(2)} ms`;
}

export function calculatePercentile(
  values: readonly number[],
  percentile: number,
): number {
  if (values.length === 0) {
    return 0;
  }
  const sorted = [...values].sort((a, b) => a - b);
  const clamped = Math.max(0, Math.min(100, percentile));
  if (clamped === 0) {
    return sorted[0];
  }
  const index = Math.ceil((clamped / 100) * sorted.length) - 1;
  return sorted[Math.max(0, Math.min(sorted.length - 1, index))];
}

interface TargetAccumulator {
  targetKey: string;
  targetId: string;
  name: string;
  url: string;
  total: number;
  upCount: number;
  failures: number;
  latencySumMs: number;
}

export function computeCheckLogInsights(
  logs: readonly CheckLog[],
): CheckLogInsights {
  const statusDistribution: StatusDistribution = {
    "2xx": 0,
    "3xx": 0,
    "4xx": 0,
    "5xx": 0,
    network_error: 0,
  };

  if (logs.length === 0) {
    return {
      totalChecks: 0,
      upCount: 0,
      downCount: 0,
      uptimePercent: 0,
      errorRatePercent: 0,
      avgLatencyMs: 0,
      minLatencyMs: 0,
      maxLatencyMs: 0,
      p95LatencyMs: 0,
      statusDistribution,
      topFailingTargets: [],
      slowestTargets: [],
      fastestTarget: null,
    };
  }

  const latenciesMs: number[] = [];
  const byTarget = new Map<string, TargetAccumulator>();

  const totals = logs.reduce(
    (acc, log) => {
      if (log.is_up) {
        acc.upCount += 1;
      } else {
        acc.downCount += 1;
      }

      const code = log.status_code;
      if (code === 0) {
        statusDistribution.network_error += 1;
      } else if (code >= 200 && code < 300) {
        statusDistribution["2xx"] += 1;
      } else if (code >= 300 && code < 400) {
        statusDistribution["3xx"] += 1;
      } else if (code >= 400 && code < 500) {
        statusDistribution["4xx"] += 1;
      } else if (code >= 500 && code < 600) {
        statusDistribution["5xx"] += 1;
      }

      const latencyMs =
        Number.isFinite(log.latency) && log.latency > 0
          ? log.latency / 1_000_000
          : 0;
      latenciesMs.push(latencyMs);
      acc.latencySumMs += latencyMs;
      if (latencyMs < acc.minLatencyMs) {
        acc.minLatencyMs = latencyMs;
      }
      if (latencyMs > acc.maxLatencyMs) {
        acc.maxLatencyMs = latencyMs;
      }

      const targetKey = log.target_id || log.url || log.name;
      const existing = byTarget.get(targetKey);
      if (existing) {
        existing.total += 1;
        if (log.is_up) {
          existing.upCount += 1;
        } else {
          existing.failures += 1;
        }
        existing.latencySumMs += latencyMs;
      } else {
        byTarget.set(targetKey, {
          targetKey,
          targetId: log.target_id,
          name: log.name || log.url,
          url: log.url,
          total: 1,
          upCount: log.is_up ? 1 : 0,
          failures: log.is_up ? 0 : 1,
          latencySumMs: latencyMs,
        });
      }

      return acc;
    },
    {
      upCount: 0,
      downCount: 0,
      latencySumMs: 0,
      minLatencyMs: Number.POSITIVE_INFINITY,
      maxLatencyMs: 0,
    },
  );

  const totalChecks = logs.length;
  const targetSummaries: TargetInsightSummary[] = Array.from(
    byTarget.values(),
  ).map((entry) => ({
    targetKey: entry.targetKey,
    targetId: entry.targetId,
    name: entry.name,
    url: entry.url,
    total: entry.total,
    upCount: entry.upCount,
    failures: entry.failures,
    uptimePercent: (entry.upCount / entry.total) * 100,
    avgLatencyMs: entry.latencySumMs / entry.total,
  }));

  const topFailingTargets = [...targetSummaries]
    .filter((item) => item.failures > 0)
    .sort(
      (a, b) =>
        b.failures - a.failures || a.uptimePercent - b.uptimePercent,
    )
    .slice(0, 5);

  const slowestTargets = [...targetSummaries]
    .sort((a, b) => b.avgLatencyMs - a.avgLatencyMs)
    .slice(0, 5);

  const targetsWithLatency = targetSummaries.filter(
    (item) => item.avgLatencyMs > 0,
  );
  const fastestTarget =
    targetsWithLatency.length > 0
      ? [...targetsWithLatency].sort(
          (a, b) =>
            a.avgLatencyMs - b.avgLatencyMs ||
            b.uptimePercent - a.uptimePercent ||
            a.name.localeCompare(b.name),
        )[0]
      : null;

  return {
    totalChecks,
    upCount: totals.upCount,
    downCount: totals.downCount,
    uptimePercent: (totals.upCount / totalChecks) * 100,
    errorRatePercent: (totals.downCount / totalChecks) * 100,
    avgLatencyMs: totals.latencySumMs / totalChecks,
    minLatencyMs: Number.isFinite(totals.minLatencyMs)
      ? totals.minLatencyMs
      : 0,
    maxLatencyMs: totals.maxLatencyMs,
    p95LatencyMs: calculatePercentile(latenciesMs, 95),
    statusDistribution,
    topFailingTargets,
    slowestTargets,
    fastestTarget,
  };
}

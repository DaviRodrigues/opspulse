import type { CheckResult, Target } from "../types/check.ts";

export function normalizeTargetMatchKey(rawUrl: string): string {
  const trimmed = rawUrl.trim();
  if (!trimmed) {
    return "";
  }
  return trimmed.replace(/^(https?:\/\/[^/?#]+)/i, (authority) =>
    authority.toLowerCase(),
  );
}

export function resolveEffectiveLiveResults(params: {
  hasReceivedStatus: boolean;
  liveTargets: readonly CheckResult[];
  initialStatus: readonly CheckResult[];
  deletedUrls?: readonly string[];
}): CheckResult[] {
  const source = params.hasReceivedStatus
    ? params.liveTargets
    : params.initialStatus;

  if (!params.deletedUrls || params.deletedUrls.length === 0) {
    return [...source];
  }

  const deletedKeys = new Set(
    params.deletedUrls.map((u) => normalizeTargetMatchKey(u)).filter(Boolean),
  );

  if (deletedKeys.size === 0) {
    return [...source];
  }

  return source.filter((item) => {
    const key = normalizeTargetMatchKey(item.URL ?? "");
    return !key || !deletedKeys.has(key);
  });
}

export function pruneDeletedTargetStatus(
  statusList: readonly CheckResult[],
  deletedUrl: string,
): CheckResult[] {
  const targetKey = normalizeTargetMatchKey(deletedUrl);
  if (!targetKey) {
    return [...statusList];
  }
  return statusList.filter(
    (item) => normalizeTargetMatchKey(item.URL ?? "") !== targetKey,
  );
}

export function buildLiveResultLookup(
  liveResults: readonly CheckResult[],
): Map<string, CheckResult> {
  const byUrl = new Map<string, CheckResult>();
  for (const item of liveResults) {
    if (item.URL) {
      const key = normalizeTargetMatchKey(item.URL);
      if (key) {
        byUrl.set(key, item);
      }
    }
  }
  return byUrl;
}

export function findLiveResultForTarget(
  target: Pick<Target, "url">,
  liveByUrl: ReadonlyMap<string, CheckResult>,
): CheckResult | undefined {
  const key = normalizeTargetMatchKey(target.url);
  return key ? liveByUrl.get(key) : undefined;
}

export function findOrphanLiveResults(
  targets: readonly Pick<Target, "url">[],
  liveResults: readonly CheckResult[],
): CheckResult[] {
  const knownUrls = new Set(
    targets.map((t) => normalizeTargetMatchKey(t.url)).filter(Boolean),
  );
  return liveResults.filter((live) => {
    const key = normalizeTargetMatchKey(live.URL ?? "");
    return !key || !knownUrls.has(key);
  });
}

export type AvailabilityFilter = "all" | "up" | "down" | "paused" | "pending";
export type TargetAvailabilityState = Exclude<AvailabilityFilter, "all">;

export const AVAILABILITY_FILTER_STORAGE_KEY =
  "opspulse.dashboard.availabilityFilter";

const VALID_AVAILABILITY_FILTERS: readonly AvailabilityFilter[] = [
  "all",
  "up",
  "down",
  "paused",
  "pending",
];

export interface StorageLike {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

export function isAvailabilityFilter(
  value: unknown,
): value is AvailabilityFilter {
  return (
    typeof value === "string" &&
    VALID_AVAILABILITY_FILTERS.includes(value as AvailabilityFilter)
  );
}

export function classifyTargetAvailability(
  target: Pick<Target, "enabled">,
  live?: Pick<CheckResult, "IsUp">,
): TargetAvailabilityState {
  if (!target.enabled) {
    return "paused";
  }
  if (!live) {
    return "pending";
  }
  return live.IsUp ? "up" : "down";
}

export function loadAvailabilityFilter(
  storage?: StorageLike | null,
): AvailabilityFilter {
  if (!storage) {
    return "all";
  }
  try {
    const stored = storage.getItem(AVAILABILITY_FILTER_STORAGE_KEY);
    return isAvailabilityFilter(stored) ? stored : "all";
  } catch {
    return "all";
  }
}

export function saveAvailabilityFilter(
  storage: StorageLike | null | undefined,
  filter: AvailabilityFilter,
): void {
  if (!storage || !isAvailabilityFilter(filter)) {
    return;
  }
  try {
    storage.setItem(AVAILABILITY_FILTER_STORAGE_KEY, filter);
  } catch {
    // Ignore storage quota or privacy-mode errors
  }
}

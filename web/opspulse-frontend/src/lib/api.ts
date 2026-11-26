import axios from "axios";
import { buildEndpointUrl, normalizeBaseUrl } from "@/lib/url";
import type {
  ApiErrorResponse,
  CheckLog,
  CheckLogFilters,
  CheckResult,
  PaginatedResponse,
  PatchTargetPayload,
  Target,
  TargetPayload,
  TargetQueryParams,
} from "@/types/check";

export const API_BASE_URL = normalizeBaseUrl(
  import.meta.env.VITE_API_BASE_URL,
);

export function getSSEEndpointUrl(): string {
  return buildEndpointUrl(API_BASE_URL, "/api/v1/targets/events");
}

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    "Content-Type": "application/json",
  },
});

function cleanQueryParams(
  raw: Record<string, string | number | boolean | undefined>,
): Record<string, string | number | boolean> {
  const cleaned: Record<string, string | number | boolean> = {};
  for (const [key, value] of Object.entries(raw)) {
    if (value === undefined || value === null) {
      continue;
    }
    if (typeof value === "string" && value.trim() === "") {
      continue;
    }
    if (typeof value === "number" && !Number.isFinite(value)) {
      continue;
    }
    cleaned[key] = typeof value === "string" ? value.trim() : value;
  }
  return cleaned;
}

export function extractApiErrorMessage(
  error: unknown,
  fallback = "Erro inesperado ao comunicar com o servidor.",
): string {
  if (axios.isAxiosError<ApiErrorResponse>(error)) {
    const apiMsg = error.response?.data?.error;
    if (typeof apiMsg === "string" && apiMsg.trim().length > 0) {
      return apiMsg;
    }
    if (error.message && error.message.trim().length > 0) {
      return error.message;
    }
  }
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return fallback;
}

export async function listTargets(
  params: TargetQueryParams = {},
): Promise<PaginatedResponse<Target>> {
  const response = await apiClient.get<PaginatedResponse<Target>>(
    "/api/v1/targets",
    {
      params: cleanQueryParams({
        enabled: params.enabled,
        tag: params.tag,
        search: params.search,
        limit: params.limit,
        offset: params.offset,
      }),
    },
  );

  return {
    data: response.data?.data ?? [],
    pagination: response.data?.pagination ?? {
      total: 0,
      limit: params.limit ?? 50,
      offset: params.offset ?? 0,
    },
  };
}

export async function createTarget(payload: TargetPayload): Promise<Target> {
  const response = await apiClient.post<Target>("/api/v1/targets", payload);
  return response.data;
}

export async function updateTarget(
  id: string,
  payload: TargetPayload,
): Promise<Target> {
  const response = await apiClient.put<Target>(
    `/api/v1/targets/${encodeURIComponent(id)}`,
    payload,
  );
  return response.data;
}

export async function patchTarget(
  id: string,
  payload: PatchTargetPayload,
): Promise<Target> {
  const response = await apiClient.patch<Target>(
    `/api/v1/targets/${encodeURIComponent(id)}`,
    payload,
  );
  return response.data;
}

export async function deleteTarget(id: string): Promise<void> {
  await apiClient.delete(`/api/v1/targets/${encodeURIComponent(id)}`);
}

export async function getTargetsStatus(): Promise<CheckResult[]> {
  const response = await apiClient.get<CheckResult[]>(
    "/api/v1/targets/status",
  );
  return Array.isArray(response.data) ? response.data : [];
}

export async function listCheckLogs(
  filters: CheckLogFilters = {},
): Promise<PaginatedResponse<CheckLog>> {
  const response = await apiClient.get<PaginatedResponse<CheckLog>>(
    "/api/v1/check-logs",
    {
      params: cleanQueryParams({
        target_id: filters.target_id,
        is_up: filters.is_up,
        status_code: filters.status_code,
        from: filters.from,
        to: filters.to,
        search: filters.search,
        limit: filters.limit,
        offset: filters.offset,
      }),
    },
  );

  return {
    data: response.data?.data ?? [],
    pagination: response.data?.pagination ?? {
      total: 0,
      limit: filters.limit ?? 50,
      offset: filters.offset ?? 0,
    },
  };
}

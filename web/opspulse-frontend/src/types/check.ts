export interface CheckResult {
  Name: string;
  URL: string;
  StatusCode: number;
  Latency: number; // Nanosegundos vindos do Go
  IsUp: boolean;
  Error?: string;
}

export type ConnectionStatus = "connected" | "connecting" | "disconnected";

export interface LogMessage {
  id: string;
  timestamp: string;
  text: string;
  type: "info" | "success" | "warn" | "error";
}

export type HttpMethod =
  | "GET"
  | "POST"
  | "PUT"
  | "PATCH"
  | "DELETE"
  | "HEAD"
  | "OPTIONS";

export interface Target {
  id: string;
  name: string;
  url: string;
  method: HttpMethod | string;
  headers?: Record<string, string>;
  enabled: boolean;
  tags: string[];
  body?: Record<string, unknown> | null;
  timeout: number; // Nanosegundos (int64)
  interval?: number; // Nanosegundos (int64; 0 ou ausente usa MONITOR_INTERVAL)
  expected_status: number[];
  expected_body_match: string;
  created_at: string;
  updated_at: string;
}

export interface TargetPayload {
  name: string;
  url: string;
  method?: HttpMethod | string;
  headers?: Record<string, string>;
  enabled: boolean;
  tags?: string[];
  body?: Record<string, unknown> | null;
  timeout?: number; // Nanosegundos (int64)
  interval?: number; // Nanosegundos (int64; 0 usa MONITOR_INTERVAL)
  expected_status?: number[];
  expected_body_match?: string;
}

export type PatchTargetPayload = Partial<TargetPayload>;

export interface CheckLog {
  id: string;
  target_id: string;
  name: string;
  url: string;
  status_code: number; // 0 para erros de rede / timeout
  latency: number; // Nanosegundos (int64)
  is_up: boolean;
  error?: string;
  checked_at: string;
}

export interface PaginationMeta {
  total: number;
  limit: number;
  offset: number;
}

export interface PaginatedResponse<T> {
  data: T[];
  pagination: PaginationMeta;
}

export interface TargetListParams {
  enabled?: boolean;
  tag?: string;
  search?: string;
  limit?: number;
  offset?: number;
}

export type TargetQueryParams = TargetListParams;

export interface CheckLogFilters {
  target_id?: string;
  is_up?: boolean;
  status_code?: number;
  from?: string;
  to?: string;
  search?: string;
  limit?: number;
  offset?: number;
}

export interface ApiErrorResponse {
  error: string;
}

import type { HttpMethod, Target, TargetPayload } from "../types/check.ts";

export const HTTP_METHODS: readonly HttpMethod[] = [
  "GET",
  "POST",
  "PUT",
  "PATCH",
  "DELETE",
  "HEAD",
  "OPTIONS",
];

export const METHODS_WITH_BODY: readonly HttpMethod[] = [
  "POST",
  "PUT",
  "PATCH",
];

export const DEFAULT_TIMEOUT_MS = 5000;
export const DEFAULT_EXPECTED_STATUS: readonly number[] = [200, 201, 204];

export interface HeaderEntry {
  key: string;
  value: string;
}

export interface TargetFormValues {
  name: string;
  url: string;
  method: HttpMethod;
  enabled: boolean;
  timeoutMs: string;
  intervalSec: string;
  expectedStatusText: string;
  tagsText: string;
  expectedBodyMatch: string;
  headers: HeaderEntry[];
  bodyText: string;
}

export function nsToMs(ns: number): number {
  if (!Number.isFinite(ns) || ns <= 0) {
    return 0;
  }
  return ns / 1_000_000;
}

export function msToNs(ms: number): number {
  if (!Number.isFinite(ms) || ms <= 0) {
    return 0;
  }
  return Math.round(ms * 1_000_000);
}

export function nsToSec(ns: number): number {
  if (!Number.isFinite(ns) || ns <= 0) {
    return 0;
  }
  return ns / 1_000_000_000;
}

export function secToNs(sec: number): number {
  if (!Number.isFinite(sec) || sec <= 0) {
    return 0;
  }
  return Math.round(sec * 1_000_000_000);
}

export function formatIntervalLabel(ns?: number): string {
  if (!ns || !Number.isFinite(ns) || ns <= 0) {
    return "Padrão";
  }
  const sec = nsToSec(ns);
  if (sec >= 60 && sec % 60 === 0) {
    return `${sec / 60}m`;
  }
  return Number.isInteger(sec) ? `${sec}s` : `${sec.toFixed(1)}s`;
}

export function methodSupportsBody(method: string): boolean {
  return METHODS_WITH_BODY.includes(method.toUpperCase() as HttpMethod);
}

export function parseExpectedStatusInput(raw: string): {
  codes: number[];
  error?: string;
} {
  const trimmed = raw.trim();
  if (!trimmed) {
    return { codes: [...DEFAULT_EXPECTED_STATUS] };
  }

  const parts = trimmed
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean);

  if (parts.length === 0) {
    return { codes: [...DEFAULT_EXPECTED_STATUS] };
  }

  const codes: number[] = [];
  const seen = new Set<number>();

  for (const part of parts) {
    if (!/^\d+$/.test(part)) {
      return {
        codes: [],
        error: `Código HTTP inválido: "${part}". Use inteiros entre 100 e 599 separados por vírgula.`,
      };
    }
    const code = Number.parseInt(part, 10);
    if (code < 100 || code > 599) {
      return {
        codes: [],
        error: `Código HTTP fora do intervalo permitido (100-599): ${code}.`,
      };
    }
    if (!seen.has(code)) {
      seen.add(code);
      codes.push(code);
    }
  }

  return { codes };
}

export function parseTagsInput(raw: string): string[] {
  const tags: string[] = [];
  const seen = new Set<string>();

  for (const part of raw.split(",")) {
    const tag = part.trim();
    if (tag && !seen.has(tag)) {
      seen.add(tag);
      tags.push(tag);
    }
  }

  return tags;
}

export function parseHeadersInput(entries: HeaderEntry[]): {
  headers?: Record<string, string>;
  error?: string;
} {
  const result: Record<string, string> = {};

  for (const entry of entries) {
    const key = entry.key.trim();
    const value = entry.value.trim();

    if (!key && !value) {
      continue;
    }
    if (!key && value) {
      return {
        error: "Existe um cabeçalho com valor preenchido mas sem nome da chave.",
      };
    }
    result[key] = value;
  }

  if (Object.keys(result).length === 0) {
    return {};
  }

  return { headers: result };
}

export function parseJsonBodyInput(raw: string): {
  body?: Record<string, unknown>;
  error?: string;
} {
  const trimmed = raw.trim();
  if (!trimmed) {
    return {};
  }

  try {
    const parsed: unknown = JSON.parse(trimmed);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return {
        error: "O corpo JSON deve ser um objeto JSON válido ({...}).",
      };
    }
    return { body: parsed as Record<string, unknown> };
  } catch {
    return {
      error: "JSON inválido no corpo da requisição.",
    };
  }
}

export function validateTargetForm(
  values: TargetFormValues,
): Record<string, string> {
  const errors: Record<string, string> = {};

  if (!values.name.trim()) {
    errors.name = "Informe um nome para o alvo.";
  }

  const trimmedUrl = values.url.trim();
  if (!trimmedUrl) {
    errors.url = "Informe a URL do alvo.";
  } else if (/\s/.test(trimmedUrl)) {
    errors.url = "A URL não pode conter espaços.";
  } else if (!/^https?:\/\//i.test(trimmedUrl)) {
    errors.url = "A URL deve usar protocolo http:// ou https://.";
  } else {
    try {
      const parsed = new URL(trimmedUrl);
      const afterScheme = trimmedUrl.replace(/^https?:\/\//i, "");
      const authorityEnd = afterScheme.search(/[/?#]/);
      const rawAuthority =
        authorityEnd === -1 ? afterScheme : afterScheme.slice(0, authorityEnd);
      const atIdx = rawAuthority.lastIndexOf("@");
      const rawHostPort =
        atIdx === -1 ? rawAuthority : rawAuthority.slice(atIdx + 1);

      if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
        errors.url = "A URL deve usar protocolo http:// ou https://.";
      } else if (!parsed.hostname || rawHostPort.endsWith(":")) {
        errors.url = "A URL precisa de um host e porta válidos.";
      } else if (parsed.port !== "") {
        const portNum = Number.parseInt(parsed.port, 10);
        if (
          !/^\d+$/.test(parsed.port) ||
          portNum < 1 ||
          portNum > 65535
        ) {
          errors.url = "A porta da URL deve estar entre 1 e 65535.";
        }
      }
    } catch {
      errors.url = "Informe uma URL HTTP/HTTPS válida.";
    }
  }

  if (!HTTP_METHODS.includes(values.method)) {
    errors.method = "Selecione um método HTTP válido.";
  }

  const timeoutRaw = values.timeoutMs.trim();
  const timeoutNum = Number(timeoutRaw);
  if (!timeoutRaw || !Number.isFinite(timeoutNum) || timeoutNum < 0) {
    errors.timeoutMs = "Informe um timeout em milissegundos (>= 0).";
  }

  const intervalRaw = (values.intervalSec ?? "").trim();
  if (intervalRaw !== "") {
    const intervalNum = Number(intervalRaw);
    if (!Number.isFinite(intervalNum) || intervalNum < 0) {
      errors.intervalSec =
        "Informe um intervalo em segundos (>= 0) ou deixe vazio para o padrão.";
    }
  }

  const statusResult = parseExpectedStatusInput(values.expectedStatusText);
  if (statusResult.error) {
    errors.expectedStatusText = statusResult.error;
  }

  const headersResult = parseHeadersInput(values.headers);
  if (headersResult.error) {
    errors.headers = headersResult.error;
  }

  if (methodSupportsBody(values.method)) {
    const bodyResult = parseJsonBodyInput(values.bodyText);
    if (bodyResult.error) {
      errors.bodyText = bodyResult.error;
    }
  }

  return errors;
}

export function targetToFormValues(target?: Target | null): TargetFormValues {
  if (!target) {
    return {
      name: "",
      url: "",
      method: "GET",
      enabled: true,
      timeoutMs: String(DEFAULT_TIMEOUT_MS),
      intervalSec: "",
      expectedStatusText: DEFAULT_EXPECTED_STATUS.join(", "),
      tagsText: "",
      expectedBodyMatch: "",
      headers: [],
      bodyText: "",
    };
  }

  const normalizedMethod = String(target.method || "GET").toUpperCase();
  const method = HTTP_METHODS.includes(normalizedMethod as HttpMethod)
    ? (normalizedMethod as HttpMethod)
    : "GET";

  const timeoutMs =
    target.timeout > 0 ? nsToMs(target.timeout) : DEFAULT_TIMEOUT_MS;
  const expectedStatus =
    target.expected_status && target.expected_status.length > 0
      ? target.expected_status
      : DEFAULT_EXPECTED_STATUS;

  const headers = Object.entries(target.headers ?? {}).map(([key, value]) => ({
    key,
    value,
  }));

  const bodyText =
    target.body && typeof target.body === "object"
      ? JSON.stringify(target.body, null, 2)
      : "";

  return {
    name: target.name ?? "",
    url: target.url ?? "",
    method,
    enabled: Boolean(target.enabled),
    timeoutMs: String(timeoutMs),
    intervalSec:
      target.interval && target.interval > 0
        ? String(nsToSec(target.interval))
        : "",
    expectedStatusText: expectedStatus.join(", "),
    tagsText: (target.tags ?? []).join(", "),
    expectedBodyMatch: target.expected_body_match ?? "",
    headers,
    bodyText,
  };
}

export function formValuesToPayload(values: TargetFormValues): TargetPayload {
  const parsedStatus = parseExpectedStatusInput(values.expectedStatusText);
  const parsedHeaders = parseHeadersInput(values.headers);
  const timeoutVal = Number(values.timeoutMs.trim());
  const timeoutNs =
    Number.isFinite(timeoutVal) && timeoutVal > 0
      ? msToNs(timeoutVal)
      : msToNs(DEFAULT_TIMEOUT_MS);

  const intervalRaw = (values.intervalSec ?? "").trim();
  const intervalVal = intervalRaw === "" ? 0 : Number(intervalRaw);
  const intervalNs =
    Number.isFinite(intervalVal) && intervalVal > 0 ? secToNs(intervalVal) : 0;

  const payload: TargetPayload = {
    name: values.name.trim(),
    url: values.url.trim(),
    method: values.method,
    enabled: Boolean(values.enabled),
    tags: parseTagsInput(values.tagsText),
    timeout: timeoutNs,
    interval: intervalNs,
    expected_status:
      parsedStatus.codes.length > 0
        ? parsedStatus.codes
        : [...DEFAULT_EXPECTED_STATUS],
    expected_body_match: values.expectedBodyMatch.trim(),
  };

  if (parsedHeaders.headers) {
    payload.headers = parsedHeaders.headers;
  }

  if (methodSupportsBody(values.method)) {
    const parsedBody = parseJsonBodyInput(values.bodyText);
    payload.body = parsedBody.body ?? null;
  } else {
    payload.body = null;
  }

  return payload;
}

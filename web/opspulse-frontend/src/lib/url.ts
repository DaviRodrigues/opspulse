export function normalizeBaseUrl(raw?: string): string {
  return (raw ?? "").trim().replace(/\/+$/, "");
}

export function buildEndpointUrl(baseUrl: string, path: string): string {
  const normalizedBase = normalizeBaseUrl(baseUrl);
  const cleanPath = path.trim().replace(/^\/+/, "");
  const normalizedPath = cleanPath ? `/${cleanPath}` : "";

  if (!normalizedBase) {
    return normalizedPath || "/";
  }

  return `${normalizedBase}${normalizedPath}`;
}

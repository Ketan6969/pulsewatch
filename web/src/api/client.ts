// Thin fetch wrapper. All transport concerns (auth header, error envelope,
// JSON parsing) live here so feature code only deals with typed data.

const TOKEN_KEY = "pulsewatch.apiToken";

export function getApiToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? "";
  } catch {
    return "";
  }
}

export function setApiToken(token: string) {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* storage unavailable: token lasts for this page only */
  }
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public fields?: Record<string, string>,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

type Query = Record<string, string | number | undefined | null>;

function buildURL(path: string, query?: Query): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(query ?? {})) {
    if (v !== undefined && v !== null && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `${path}?${s}` : path;
}

export async function request<T>(
  method: string,
  path: string,
  opts: { query?: Query; body?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  const token = getApiToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";

  let res: Response;
  try {
    res = await fetch(buildURL(path, opts.query), {
      method,
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      signal: opts.signal,
    });
  } catch (err) {
    if ((err as Error).name === "AbortError") throw err;
    throw new ApiError(0, "network_error", "Unable to reach the Pulsewatch server");
  }

  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: unknown = undefined;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      /* non-JSON body */
    }
  }
  if (!res.ok) {
    const e = (data as { error?: { code: string; message: string; fields?: Record<string, string> } })?.error;
    throw new ApiError(res.status, e?.code ?? "http_error", e?.message ?? `Request failed (${res.status})`, e?.fields);
  }
  return data as T;
}

export const api = {
  get: <T>(path: string, query?: Query, signal?: AbortSignal) => request<T>("GET", path, { query, signal }),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, { body }),
  put: <T>(path: string, body?: unknown) => request<T>("PUT", path, { body }),
  del: (path: string) => request<void>("DELETE", path),
};

import type { CSRFToken } from "./generated";
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    public requestId?: string,
  ) {
    super(code);
  }
}
let csrf: Promise<string> | undefined;
export function resetSession() {
  csrf = undefined;
}
async function token() {
  csrf ??= fetch("/api/auth/csrf", {
    credentials: "same-origin",
    cache: "no-store",
  })
    .then(decode<CSRFToken>)
    .then((value) => value.csrf_token)
    .catch((e) => {
      csrf = undefined;
      throw e;
    });
  return csrf;
}
async function decode<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      body.code || "unavailable",
      body.request_id,
    );
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}
export async function request<T>(
  path: string,
  options: {
    method?: string;
    body?: unknown;
    key?: string;
    signal?: AbortSignal;
  } = {},
): Promise<T> {
  const method = options.method ?? "GET";
  const headers: Record<string, string> = { Accept: "application/json" };
  if (method !== "GET") headers["X-CSRF-Token"] = await token();
  if (options.key) headers["Idempotency-Key"] = options.key;
  const form = options.body instanceof FormData;
  if (options.body !== undefined && !form)
    headers["Content-Type"] = "application/json";
  const response = await fetch("/api" + path, {
    method,
    headers,
    credentials: "same-origin",
    cache: "no-store",
    body: form
      ? (options.body as FormData)
      : options.body === undefined
        ? undefined
        : JSON.stringify(options.body),
    signal: options.signal,
  });
  if (response.status === 403) csrf = undefined; // Next explicit attempt obtains fresh CSRF. Never replay writes silently.
  return decode<T>(response);
}
// A retry with the same payload must reuse its key after a lost response.
// Kept only in memory, scoped to each mounted workflow (never browser storage).
export class Commands {
  private keys = new Map<string, string>();
  private uncertain = new Map<string, unknown>();
  retry<T>(path: string): Promise<T> | undefined {
    const body = this.uncertain.get(path);
    return body === undefined ? undefined : this.send<T>(path, body);
  }
  send<T>(path: string, body: unknown): Promise<T> {
    const fingerprint = JSON.stringify([path, body]);
    let key = this.keys.get(fingerprint);
    if (!key) {
      key = crypto.randomUUID();
      this.keys.set(fingerprint, key);
    }
    return request<T>(path, { method: "POST", body, key })
      .then((result) => {
        this.uncertain.delete(path);
        return result;
      })
      .catch((error) => {
        if (!(error instanceof ApiError) || error.status >= 500)
          this.uncertain.set(path, body);
        else this.uncertain.delete(path);
        throw error;
      });
  }
}
export const post = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: "POST", body });
export const patch = <T>(path: string, body: unknown) =>
  request<T>(path, { method: "PATCH", body });

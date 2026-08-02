// Fetch wrapper for the local apitest ui server (UI_SPECIFICATION.md §4).
// Bearer auth from sessionStorage['apitest.token'], the {"error":{...}}
// envelope mapped to ApiError, and a 401 → disconnected signal hook.

export const API_BASE = '/api/v1';

const TOKEN_KEY = 'apitest.token';

export function getToken(): string | null {
  try {
    return sessionStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

export function setToken(token: string): void {
  try {
    sessionStorage.setItem(TOKEN_KEY, token);
  } catch {
    // sessionStorage unavailable — the Disconnected screen will surface it.
  }
}

interface ErrorEnvelope {
  error?: {
    code?: string;
    message?: string;
    hint?: string;
    details?: unknown;
  };
}

/** Mirror of the §4.1 error envelope plus the HTTP status. */
export class ApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly hint?: string;
  readonly details?: unknown;

  constructor(code: string, status: number, message: string, hint?: string, details?: unknown) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.hint = hint;
    this.details = details;
  }
}

type UnauthorizedHandler = () => void;
let unauthorizedHandler: UnauthorizedHandler | null = null;

/**
 * Registers the 401 → 'disconnected' signal hook: a missing/stale token means
 * the server restarted; there is no login form, only the Disconnected screen.
 */
export function setUnauthorizedHandler(fn: UnauthorizedHandler | null): void {
  unauthorizedHandler = fn;
}

async function request(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  const token = getToken();
  if (token !== null) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  if (init.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  let res: Response;
  try {
    res = await fetch(API_BASE + path, { ...init, headers });
  } catch (err) {
    throw new ApiError('network_error', 0, err instanceof Error ? err.message : 'network request failed');
  }

  if (!res.ok) {
    let code = 'internal';
    let message = `HTTP ${res.status}`;
    let hint: string | undefined;
    let details: unknown;
    try {
      const body = (await res.json()) as ErrorEnvelope;
      if (body.error) {
        code = body.error.code ?? code;
        message = body.error.message ?? message;
        hint = body.error.hint;
        details = body.error.details;
      }
    } catch {
      // non-JSON error body — keep the defaults
    }
    if (res.status === 401) {
      unauthorizedHandler?.();
    }
    throw new ApiError(code, res.status, message, hint, details);
  }
  return res;
}

/** JSON fetch — resolves undefined for 204 No Content. */
export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await request(path, init);
  if (res.status === 204) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

/** Raw fetch with auth + error mapping — for /body and /files streams. */
export function apiFetchRaw(path: string, init: RequestInit = {}): Promise<Response> {
  return request(path, init);
}

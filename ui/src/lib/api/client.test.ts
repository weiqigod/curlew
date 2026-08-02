import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, apiFetch, apiFetchRaw, getToken, setToken, setUnauthorizedHandler } from './client';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('api client', () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    sessionStorage.clear();
    fetchMock.mockReset();
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    setUnauthorizedHandler(null);
  });

  it('attaches the bearer token from sessionStorage["curlew.token"]', async () => {
    setToken('deadbeefdeadbeefdeadbeefdeadbeef');
    expect(getToken()).toBe('deadbeefdeadbeefdeadbeefdeadbeef');
    fetchMock.mockResolvedValue(jsonResponse({ ok: true }));

    await apiFetch('/meta');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/v1/meta');
    expect(new Headers(init.headers).get('Authorization')).toBe(
      'Bearer deadbeefdeadbeefdeadbeefdeadbeef',
    );
  });

  it('omits the Authorization header when no token is stored', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ ok: true }));
    await apiFetch('/meta');
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(new Headers(init.headers).has('Authorization')).toBe(false);
  });

  it('sets Content-Type: application/json on bodies', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ run_id: 'x', state: 'running' }, 202));
    await apiFetch('/runs', { method: 'POST', body: JSON.stringify({ env: 'dev' }) });
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json');
  });

  it('maps the {"error":{...}} envelope to ApiError {code, status, details}', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(
        {
          error: {
            code: 'run_active',
            message: 'a run is already in progress',
            hint: 'wait or cancel',
            details: { run_id: 'abc' },
          },
        },
        409,
      ),
    );

    const err = await apiFetch('/runs', { method: 'POST', body: '{}' }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiErr = err as ApiError;
    expect(apiErr.code).toBe('run_active');
    expect(apiErr.status).toBe(409);
    expect(apiErr.message).toBe('a run is already in progress');
    expect(apiErr.hint).toBe('wait or cancel');
    expect(apiErr.details).toEqual({ run_id: 'abc' });
  });

  it('falls back to defaults on non-JSON error bodies', async () => {
    fetchMock.mockResolvedValue(new Response('boom', { status: 500 }));
    const err = await apiFetch('/meta').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).code).toBe('internal');
    expect((err as ApiError).status).toBe(500);
  });

  it('signals the unauthorized handler on 401 (disconnected flow)', async () => {
    const onUnauthorized = vi.fn();
    setUnauthorizedHandler(onUnauthorized);
    fetchMock.mockResolvedValue(
      jsonResponse({ error: { code: 'unauthorized', message: 'missing or invalid session token' } }, 401),
    );

    const err = await apiFetch('/meta').catch((e: unknown) => e);
    expect(onUnauthorized).toHaveBeenCalledTimes(1);
    expect((err as ApiError).code).toBe('unauthorized');
    expect((err as ApiError).status).toBe(401);
  });

  it('does not signal the unauthorized handler on other failures', async () => {
    const onUnauthorized = vi.fn();
    setUnauthorizedHandler(onUnauthorized);
    fetchMock.mockResolvedValue(
      jsonResponse({ error: { code: 'not_found', message: 'unknown run' } }, 404),
    );
    await apiFetch('/runs/zzz').catch(() => undefined);
    expect(onUnauthorized).not.toHaveBeenCalled();
  });

  it('wraps network failures as ApiError {code: network_error, status: 0}', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
    const err = await apiFetch('/meta').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).code).toBe('network_error');
    expect((err as ApiError).status).toBe(0);
  });

  it('resolves undefined for 204 No Content', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(apiFetch('/open', { method: 'POST', body: '{}' })).resolves.toBeUndefined();
  });

  it('apiFetchRaw returns the raw Response with auth applied', async () => {
    setToken('tok');
    fetchMock.mockResolvedValue(new Response('raw-bytes', { status: 200 }));
    const res = await apiFetchRaw('/runs/r/requests/q/body?which=response');
    expect(await res.text()).toBe('raw-bytes');
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/v1/runs/r/requests/q/body?which=response');
    expect(new Headers(init.headers).get('Authorization')).toBe('Bearer tok');
  });
});

import { ApiError } from '$lib/types/api-error';

/** Options for the typed fetch wrapper. */
export interface RequestOptions extends Omit<RequestInit, 'body'> {
	body?: unknown;
	params?: Record<string, string | number | undefined>;
	token?: string;
	fetch?: typeof fetch;
}

const DEFAULT_API_URL = 'http://localhost:5000';

/**
 * Returns the configured public API base URL.
 * Uses `PUBLIC_API_URL` env var at build time when running inside SvelteKit;
 * falls back to localhost for unit tests (no SvelteKit env available).
 */
function getApiBase(): string {
	try {
		// SvelteKit dynamic env — only available in SSR context
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const env = (globalThis as any).__SVELTEKIT_DYNAMIC_ENV__;
		if (env?.PUBLIC_API_URL) return env.PUBLIC_API_URL;
	} catch {
		/* not in SvelteKit context */
	}
	// Fallback for unit tests and standalone Node usage
	return process.env.PUBLIC_API_URL ?? DEFAULT_API_URL;
}

function buildUrl(path: string, params?: RequestOptions['params']): string {
	const base = getApiBase();
	const url = new URL(path.startsWith('/') ? path : `/${path}`, base);
	if (params) {
		for (const [k, v] of Object.entries(params)) {
			if (v !== undefined) url.searchParams.set(k, String(v));
		}
	}
	return url.toString();
}

/** Makes a typed HTTP request. Throws `ApiError` on non-2xx responses. */
export async function request<T>(
	method: string,
	path: string,
	opts: RequestOptions = {}
): Promise<T> {
	const fetchFn = opts.fetch ?? globalThis.fetch;
	const headers = new Headers(opts.headers);
	if (opts.body !== undefined && !headers.has('Content-Type')) {
		headers.set('Content-Type', 'application/json');
	}
	if (opts.token) headers.set('Authorization', `Bearer ${opts.token}`);

	const res = await fetchFn(buildUrl(path, opts.params), {
		...opts,
		method,
		headers,
		body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined
	});

	if (!res.ok) {
		let code = 'server_error';
		let description = res.statusText;
		let field: string | undefined;
		let details: Record<string, unknown> | undefined;
		let problemType: string | undefined;
		try {
			const body = await res.json();
			// RFC 7807 problem detail: extract type URI for structured error matching
			if (typeof body?.type === 'string') problemType = body.type;
			// RFC 7807 uses `title` and `detail`; legacy backend uses `description`
			if (body?.code) code = body.code;
			if (body?.description) description = body.description;
			if (typeof body?.detail === 'string') description = body.detail;
			else if (body?.title && !body?.description) description = body.title;
			// 409-style responses use "message" instead of "description"
			if (body?.message && !body?.description) description = body.message;
			if (body?.field) field = body.field;
			if (body && typeof body === 'object') {
				const {
					code: _c,
					description: _d,
					message: _m,
					field: _f,
					type: _t,
					title: _ti,
					detail: _det,
					status: _s,
					...rest
				} = body as Record<string, unknown>;
				// Suppress unused variable lints — we only care about `rest`
				void _c;
				void _d;
				void _m;
				void _f;
				void _t;
				void _ti;
				void _det;
				void _s;
				if (Object.keys(rest).length > 0) details = rest;
			}
		} catch {
			/* non-JSON error body — use defaults */
		}
		throw new ApiError(code, description, res.status, field, details, problemType);
	}

	if (res.status === 204) return undefined as T;
	return (await res.json()) as T;
}

/** Convenience API object with typed GET/POST/PATCH/PUT/DELETE shortcuts. */
export const api = {
	get: <T>(path: string, opts?: RequestOptions) => request<T>('GET', path, opts),
	post: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
		request<T>('POST', path, { ...opts, body }),
	patch: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
		request<T>('PATCH', path, { ...opts, body }),
	put: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
		request<T>('PUT', path, { ...opts, body }),
	delete: <T>(path: string, opts?: RequestOptions) => request<T>('DELETE', path, opts)
};

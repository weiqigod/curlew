import { ApiError } from '$lib/types/api-error';
import type { RequestOptions } from './client';
import type { VaultConfigResponse } from '$lib/types/vault-config';

const BASE_URL_TEMPLATE = (orgId: string) =>
	`/api/v1/organizations/${encodeURIComponent(orgId)}/vault-config`;

async function rawRequest<T>(
	method: string,
	url: string,
	opts: RequestOptions & { rawBody?: string; contentType?: string } = {}
): Promise<T> {
	const fetchFn = opts.fetch ?? globalThis.fetch;
	const headers = new Headers(opts.headers);
	if (opts.contentType) headers.set('Content-Type', opts.contentType);
	if (opts.token) headers.set('Authorization', `Bearer ${opts.token}`);

	// Build absolute URL from relative path
	const apiBase =
		(typeof process !== 'undefined' ? process.env.PUBLIC_API_URL : undefined) ??
		'http://localhost:5000';
	const fullUrl = url.startsWith('http') ? url : `${apiBase}${url}`;

	const res = await fetchFn(fullUrl, {
		method,
		headers,
		body: opts.rawBody,
	});

	if (!res.ok) {
		let code = 'server_error';
		let description = res.statusText;
		let problemType: string | undefined;
		let details: Record<string, unknown> | undefined;
		try {
			const body = await res.json();
			if (typeof body?.type === 'string') problemType = body.type;
			if (body?.code) code = body.code;
			if (typeof body?.detail === 'string') description = body.detail;
			else if (body?.title) description = body.title;
			if (body && typeof body === 'object') {
				const { code: _c, type: _t, title: _ti, detail: _d, status: _s, ...rest } =
					body as Record<string, unknown>;
				void _c; void _t; void _ti; void _d; void _s;
				if (Object.keys(rest).length > 0) details = rest;
			}
		} catch { /* non-JSON */ }
		throw new ApiError(code, description, res.status, undefined, details, problemType);
	}

	if (res.status === 204) return undefined as T;
	return res.json() as Promise<T>;
}

/** Typed API client for vault-config endpoints. */
export const vaultConfigApi = {
	/**
	 * Fetches the vault-config template for an organization.
	 * Returns `null` on 404 (no config set yet). Throws `ApiError` on other failures.
	 */
	async get(orgId: string, opts: RequestOptions = {}): Promise<VaultConfigResponse | null> {
		try {
			return await rawRequest<VaultConfigResponse>('GET', BASE_URL_TEMPLATE(orgId), opts);
		} catch (err) {
			if (err instanceof ApiError && err.status === 404) return null;
			throw err;
		}
	},

	/**
	 * Saves the vault-config template for an organization.
	 * Sends the YAML string as `application/yaml` body.
	 * Returns the updated `VaultConfigResponse` (with version and optional warnings).
	 */
	async put(orgId: string, yaml: string, opts: RequestOptions = {}): Promise<VaultConfigResponse> {
		return rawRequest<VaultConfigResponse>('PUT', BASE_URL_TEMPLATE(orgId), {
			...opts,
			rawBody: yaml,
			contentType: 'application/yaml',
		});
	},

	/**
	 * Deletes the vault-config template for an organization.
	 * Returns `void` on 204.
	 */
	async remove(orgId: string, opts: RequestOptions = {}): Promise<void> {
		await rawRequest<void>('DELETE', BASE_URL_TEMPLATE(orgId), opts);
	},
};

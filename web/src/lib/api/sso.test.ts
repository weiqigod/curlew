import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ssoApi } from './sso';
import { isApiError } from '$lib/types/api-error';

function makeFetch(status: number, body: string): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: status >= 200 && status < 300,
		status,
		statusText:
			status === 400
				? 'Bad Request'
				: status === 403
					? 'Forbidden'
					: status === 500
						? 'Internal Server Error'
						: 'OK',
		json: () => Promise.resolve(JSON.parse(body)),
		text: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

const ssoConfigResponse = JSON.stringify({
	sso_enabled: true,
	sso_provider: 'saml',
	saml_config: {
		idp_metadata_url: 'https://idp.example.com/metadata',
		acs_url: 'https://sp.example.com/acs',
		entity_id: 'https://sp.example.com',
		idp_sso_url: 'https://idp.example.com/sso'
	},
	oidc_config: null
});

const orgResponse = JSON.stringify({
	id: 'org_abc123',
	name: 'Acme',
	slug: 'acme',
	role: 'owner',
	seat_count: 1,
	seat_limit: 10,
	status: 'active',
	created_at: '2026-04-01T00:00:00Z',
	sso_enabled: true,
	sso_provider: 'saml'
});

beforeEach(() => {
	vi.restoreAllMocks();
});

describe('ssoApi.get', () => {
	it('get returns parsed config', async () => {
		const fetchFn = makeFetch(200, ssoConfigResponse);
		const result = await ssoApi.get('org_abc123', { fetch: fetchFn });
		expect(result.sso_enabled).toBe(true);
		expect(result.sso_provider).toBe('saml');
		expect(result.saml_config?.idp_metadata_url).toBe('https://idp.example.com/metadata');
	});

	it('get propagates 403 permission_denied', async () => {
		const fetchFn = makeFetch(
			403,
			'{"code":"permission_denied","description":"Owner only"}'
		);
		let caught: unknown;
		try {
			await ssoApi.get('org_abc123', { fetch: fetchFn });
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('permission_denied');
		expect(err.status).toBe(403);
	});
});

describe('ssoApi.upsertSaml', () => {
	it('upsertSaml sends PUT JSON body and returns SsoOrganizationResponse', async () => {
		const fetchFn = makeFetch(200, orgResponse);
		const result = await ssoApi.upsertSaml(
			'org_abc123',
			{
				idp_metadata_url: 'https://idp.example.com/metadata',
				acs_url: 'https://sp.example.com/acs',
				entity_id: 'https://sp.example.com'
			},
			{ fetch: fetchFn }
		);
		expect(result.sso_enabled).toBe(true);
		expect(result.sso_provider).toBe('saml');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[1].method).toBe('PUT');
		const body = JSON.parse(call[1].body as string);
		expect(body.idp_metadata_url).toBe('https://idp.example.com/metadata');
	});

	it('upsertSaml propagates 400 invalid_sso_config with field', async () => {
		const fetchFn = makeFetch(
			400,
			'{"code":"invalid_sso_config","description":"bad url","field":"idp_metadata_url"}'
		);
		let caught: unknown;
		try {
			await ssoApi.upsertSaml(
				'org_abc123',
				{ idp_metadata_url: 'bad', acs_url: 'x', entity_id: 'y' },
				{ fetch: fetchFn }
			);
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('invalid_sso_config');
		expect(err.field).toBe('idp_metadata_url');
	});
});

describe('ssoApi.upsertOidc', () => {
	it('upsertOidc sends PUT JSON body with client_secret', async () => {
		const oidcOrg = JSON.stringify({ ...JSON.parse(orgResponse), sso_provider: 'oidc' });
		const fetchFn = makeFetch(200, oidcOrg);
		const result = await ssoApi.upsertOidc(
			'org_abc123',
			{
				issuer_url: 'https://idp.example.com',
				client_id: 'my_client',
				client_secret: 'super_secret'
			},
			{ fetch: fetchFn }
		);
		expect(result.sso_provider).toBe('oidc');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[1].method).toBe('PUT');
		const body = JSON.parse(call[1].body as string);
		expect(body.client_secret).toBe('super_secret');
	});

	it('upsertOidc propagates 400 oidc_discovery_failed', async () => {
		const fetchFn = makeFetch(
			400,
			'{"code":"oidc_discovery_failed","description":"cannot reach discovery endpoint"}'
		);
		let caught: unknown;
		try {
			await ssoApi.upsertOidc(
				'org_abc123',
				{ issuer_url: 'https://bad.example.com', client_id: 'id', client_secret: 'secret' },
				{ fetch: fetchFn }
			);
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('oidc_discovery_failed');
	});
});

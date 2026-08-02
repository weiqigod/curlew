import { describe, it, expect, vi } from 'vitest';
import { vaultConfigApi } from './vault-config';

function mockFetch(body: unknown, status = 200, headers: Record<string, string> = {}): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: status >= 200 && status < 300,
		status,
		headers: new Headers(headers),
		json: () => Promise.resolve(body),
		text: () => Promise.resolve(JSON.stringify(body))
	}) as unknown as typeof fetch;
}

const VALID_RESPONSE = {
	template: 'team_secrets:\n  provider: aws-secrets-manager\n',
	version: 1,
	updated_at: '2026-05-12T10:00:00Z',
	updated_by_email: 'owner@test.com',
	warnings: null
};

describe('vaultConfigApi', () => {
	it('get returns parsed template + version', async () => {
		const fetchFn = mockFetch(VALID_RESPONSE);

		const result = await vaultConfigApi.get('org_abc123', { fetch: fetchFn });

		expect(result).not.toBeNull();
		expect(result!.version).toBe(1);
		expect(result!.template).toContain('team_secrets');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('/organizations/org_abc123/vault-config');
	});

	it('get treats 404 as null', async () => {
		const fetchFn = mockFetch({ code: 'vault_config_not_found' }, 404);

		const result = await vaultConfigApi.get('org_abc123', { fetch: fetchFn });

		expect(result).toBeNull();
	});

	it('put sends yaml as text body with Content-Type application/yaml', async () => {
		const fetchFn = mockFetch(VALID_RESPONSE);

		const yaml = 'team_secrets:\n  provider: aws-secrets-manager\n';
		await vaultConfigApi.put('org_abc123', yaml, { fetch: fetchFn });

		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const init = call[1] as RequestInit;
		const headers = new Headers(init.headers as HeadersInit);
		expect(headers.get('Content-Type')).toContain('application/yaml');
		expect(init.body).toBe(yaml);
	});

	it('put 422 throws ApiError with problem type', async () => {
		const fetchFn = mockFetch(
			{
				type: 'https://api.apitool.dev/errors/vault-template-suspicious-value',
				title: 'Template contains likely-secret values',
				status: 422,
				offending_paths: ['team_secrets.password']
			},
			422
		);

		await expect(vaultConfigApi.put('org_abc123', 'password: secret123456789', { fetch: fetchFn }))
			.rejects.toMatchObject({
				status: 422,
				problemType: 'https://api.apitool.dev/errors/vault-template-suspicious-value'
			});
	});

	it('remove returns void on 204', async () => {
		const fetchFn = vi.fn().mockResolvedValue({
			ok: true,
			status: 204,
			headers: new Headers(),
			json: () => Promise.resolve(undefined)
		}) as unknown as typeof fetch;

		await expect(vaultConfigApi.remove('org_abc123', { fetch: fetchFn }))
			.resolves.toBeUndefined();
	});

	it('get propagates 402 payment required as ApiError', async () => {
		const fetchFn = mockFetch(
			{ type: 'https://api.apitool.dev/errors/tier-ineligible', title: 'Tier ineligible' },
			402
		);

		await expect(vaultConfigApi.get('org_abc123', { fetch: fetchFn }))
			.rejects.toMatchObject({ status: 402 });
	});
});

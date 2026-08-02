import { describe, it, expect, vi } from 'vitest';
import { request, api } from './client';
import { isApiError } from '$lib/types/api-error';

function makeFetch(status: number, body: string): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: status >= 200 && status < 300,
		status,
		statusText: status === 403 ? 'Forbidden' : status === 500 ? 'Internal Server Error' : 'OK',
		json: () => Promise.resolve(JSON.parse(body)),
		text: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

describe('api.get', () => {
	const cases = [
		{
			name: 'returns parsed body on 200',
			status: 200,
			body: '{"ok":true}',
			expected: { ok: true }
		},
		{
			name: 'attaches bearer token',
			status: 200,
			body: '{"ok":true}',
			token: 'mytoken',
			assertHeader: 'Authorization'
		},
		{
			name: 'throws ApiError with backend code/msg',
			status: 403,
			body: '{"code":"permission_denied","description":"nope"}',
			throwsCode: 'permission_denied',
			throwsStatus: 403
		},
		{
			name: 'throws generic ApiError on non-JSON 5xx',
			status: 500,
			body: '<html>oops</html>',
			throwsCode: 'server_error',
			throwsStatus: 500
		},
		{
			name: 'serializes query params',
			status: 200,
			body: '{}',
			params: { limit: 10, range: '7d' },
			assertUrlContains: '?limit=10&range=7d'
		}
	];

	for (const c of cases) {
		it(c.name, async () => {
			const fetchFn = makeFetch(c.status, c.body);

			if (c.throwsCode !== undefined) {
				await expect(
					request<unknown>('GET', '/test', {
						fetch: fetchFn
					})
				).rejects.toSatisfy((err: unknown) => isApiError(err) && (err as { code: string }).code === c.throwsCode);
				return;
			}

			const result = await request<unknown>('GET', '/test', {
				fetch: fetchFn,
				token: c.token,
				params: c.params as Record<string, string | number | undefined> | undefined
			});

			if (c.expected !== undefined) {
				expect(result).toEqual(c.expected);
			}

			if (c.assertHeader) {
				const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
				const headers = call[1].headers as Headers;
				expect(headers.get('Authorization')).toBe('Bearer mytoken');
			}

			if (c.assertUrlContains) {
				const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
				const url = call[0] as string;
				expect(url).toContain(c.assertUrlContains);
			}
		});
	}
});

describe('api.get details capture', () => {
	it('request captures non-reserved body fields into ApiError.details', async () => {
		const fetchFn = makeFetch(
			409,
			'{"code":"role_in_use","message":"Role in use","member_count":5}'
		);
		let caught: unknown;
		try {
			await api.get('/foo', { fetch: fetchFn });
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('role_in_use');
		expect((err.details as { member_count: number }).member_count).toBe(5);
	});
});

describe('RFC 7807 problem-detail parsing', () => {
	it('422 with type/detail/score sets problemType and uses detail as description', async () => {
		const fetchFn = makeFetch(
			422,
			JSON.stringify({
				type: 'https://apitool.dev/errors/password-too-weak',
				title: 'Password rejected',
				status: 422,
				detail: 'score 1/4',
				score: 1
			})
		);
		let caught: unknown;
		try {
			await api.post('/test', {}, { fetch: fetchFn });
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.problemType).toBe('https://apitool.dev/errors/password-too-weak');
		expect(err.message).toBe('score 1/4');
		expect((err.details as { score: number }).score).toBe(1);
	});

	it('400 with type/title (no description, no detail) uses title as description', async () => {
		const fetchFn = makeFetch(
			400,
			JSON.stringify({
				type: 'https://apitool.dev/errors/password-reset-token-invalid',
				title: 'Token invalid',
				status: 400
			})
		);
		let caught: unknown;
		try {
			await api.post('/test', {}, { fetch: fetchFn });
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.problemType).toBe('https://apitool.dev/errors/password-reset-token-invalid');
		expect(err.message).toBe('Token invalid');
	});

	it('400 with only type (no detail, no title, no description) sets problemType and falls back to statusText', async () => {
		const fetchFn = vi.fn().mockResolvedValue({
			ok: false,
			status: 400,
			statusText: 'Bad Request',
			json: () =>
				Promise.resolve({
					type: 'https://apitool.dev/errors/email-verification-token-invalid'
				}),
			text: () => Promise.resolve('')
		}) as unknown as typeof fetch;
		let caught: unknown;
		try {
			await api.post('/test', {}, { fetch: fetchFn });
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.problemType).toBe('https://apitool.dev/errors/email-verification-token-invalid');
		expect(err.message).toBe('Bad Request');
	});
});

describe('api.put', () => {
	it('sends PUT with JSON body and returns parsed response', async () => {
		const fetchFn = makeFetch(200, '{"id":"org_1","sso_enabled":true}');
		const result = await api.put<{ id: string; sso_enabled: boolean }>(
			'/api/v1/organizations/org_1/sso/saml',
			{ idp_metadata_url: 'https://idp.example.com/meta' },
			{ fetch: fetchFn }
		);
		expect(result.sso_enabled).toBe(true);
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[1].method).toBe('PUT');
		const body = JSON.parse(call[1].body as string);
		expect(body.idp_metadata_url).toBe('https://idp.example.com/meta');
	});

	it('throws ApiError with field on 400 invalid_sso_config', async () => {
		const fetchFn = makeFetch(
			400,
			'{"code":"invalid_sso_config","description":"bad url","field":"idp_metadata_url"}'
		);
		let caught: unknown;
		try {
			await api.put('/api/v1/organizations/org_1/sso/saml', {}, { fetch: fetchFn });
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('invalid_sso_config');
		expect(err.field).toBe('idp_metadata_url');
	});
});

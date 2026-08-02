import { describe, it, expect, vi } from 'vitest';
import { prChecksApi } from './pr-checks';
import type { PrCheck } from '$lib/types/pr-checks';

function makeFetchReturning(pr_checks: Partial<PrCheck>[]): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status: 200,
		json: () => Promise.resolve({ pr_checks })
	}) as unknown as typeof fetch;
}

describe('prChecksApi.list', () => {
	it('calls /api/v1/organizations/:id/pr-checks and returns array', async () => {
		const fetchFn = makeFetchReturning([
			{ id: 'prc_1', repo: 'acme/api', pr: 7, state: 'success', result_id: 'res_1', created_at: '2026-04-17T10:00:00Z' }
		]);
		const out = await prChecksApi.list('org_abc', { token: 't', fetch: fetchFn });
		expect(out).toHaveLength(1);
		expect(out[0].pr).toBe(7);
	});

	it('returns empty list when no checks', async () => {
		const fetchFn = makeFetchReturning([]);
		const out = await prChecksApi.list('org_abc', { fetch: fetchFn });
		expect(out).toHaveLength(0);
	});

	it('passes limit param to backend', async () => {
		const fetchFn = makeFetchReturning([]);
		await prChecksApi.list('org_abc', { fetch: fetchFn, limit: 25 });
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const url = call[0] as string;
		expect(url).toContain('limit=25');
	});

	it('propagates error from backend', async () => {
		const fetchFn = vi.fn().mockResolvedValue({
			ok: false,
			status: 403,
			statusText: 'Forbidden',
			json: () => Promise.resolve({ code: 'permission_denied', description: 'Permission denied.' })
		}) as unknown as typeof fetch;
		await expect(prChecksApi.list('org_abc', { fetch: fetchFn })).rejects.toThrow();
	});
});

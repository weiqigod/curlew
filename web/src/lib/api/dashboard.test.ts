import { describe, it, expect, vi } from 'vitest';
import { dashboardApi } from './dashboard';

function mockFetch(body: unknown, status = 200): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: status >= 200 && status < 300,
		status,
		headers: new Headers(),
		json: () => Promise.resolve(body),
		text: () => Promise.resolve(JSON.stringify(body))
	}) as unknown as typeof fetch;
}

const STATS_BODY = {
	window: '30d',
	window_start: '2026-04-12T00:00:00Z',
	window_end: '2026-05-12T00:00:00Z',
	totals: {
		runs: 100,
		pass_count: 90,
		fail_count: 10,
		skipped_count: 0,
		pass_rate: 0.9,
		avg_duration_ms: 250,
		p50_duration_ms: 200,
		p95_duration_ms: 800
	},
	trend: [
		{
			date: '2026-05-11',
			runs: 10,
			pass_count: 9,
			fail_count: 1,
			pass_rate: 0.9,
			avg_duration_ms: 250
		}
	]
};

const FAILURES_BODY = {
	window: '30d',
	limit: 10,
	limit_clamped: false,
	items: [
		{
			method: 'GET',
			path_template: '/api/users/:id',
			failure_count: 5,
			first_seen_at: '2026-05-01T00:00:00Z',
			last_seen_at: '2026-05-11T00:00:00Z',
			sample_run_ids: ['run_abc123']
		}
	]
};

describe('dashboardApi.getStats', () => {
	it('parses stats envelope and returns typed response', async () => {
		const fetchFn = mockFetch(STATS_BODY);
		const result = await dashboardApi.getStats('org_abc', { fetch: fetchFn });
		expect(result.window).toBe('30d');
		expect(result.totals.runs).toBe(100);
		expect(result.totals.pass_rate).toBe(0.9);
		expect(result.trend).toHaveLength(1);
		expect(result.trend[0].date).toBe('2026-05-11');
	});

	it('forwards ?window=30d query param in the request URL', async () => {
		const fetchFn = mockFetch(STATS_BODY);
		await dashboardApi.getStats('org_abc', { fetch: fetchFn, timeWindow: '30d' });
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('window=30d');
	});

	it('omits window query param when not specified', async () => {
		const fetchFn = mockFetch(STATS_BODY);
		await dashboardApi.getStats('org_abc', { fetch: fetchFn });
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).not.toContain('window=');
	});

	it('includes orgId in the request URL', async () => {
		const fetchFn = mockFetch(STATS_BODY);
		await dashboardApi.getStats('org_abc', { fetch: fetchFn });
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('/organizations/org_abc/results/stats');
	});

	it('surfaces ApiError on 402 tier-ineligible', async () => {
		const fetchFn = mockFetch(
			{
				type: 'https://api.apitool.dev/errors/tier-ineligible',
				code: 'dashboard_tier_ineligible',
				title: 'Tier ineligible',
				detail: 'This feature requires the team tier or above.'
			},
			402
		);
		await expect(dashboardApi.getStats('org_abc', { fetch: fetchFn })).rejects.toMatchObject({
			status: 402,
			problemType: 'https://api.apitool.dev/errors/tier-ineligible'
		});
	});

	it('surfaces ApiError on 400 unsupported_window', async () => {
		const fetchFn = mockFetch(
			{
				type: 'https://api.apitool.dev/errors/unsupported-window',
				code: 'unsupported_window',
				title: 'Unsupported window',
				detail: 'Allowed values: 7d, 30d, 90d.',
				received: '14d',
				allowed_values: ['7d', '30d', '90d']
			},
			400
		);
		await expect(
			dashboardApi.getStats('org_abc', { fetch: fetchFn, timeWindow: '14d' })
		).rejects.toMatchObject({
			status: 400,
			code: 'unsupported_window'
		});
	});
});

describe('dashboardApi.getFailures', () => {
	it('parses failures envelope and returns typed response', async () => {
		const fetchFn = mockFetch(FAILURES_BODY);
		const result = await dashboardApi.getFailures('org_abc', { fetch: fetchFn });
		expect(result.window).toBe('30d');
		expect(result.items).toHaveLength(1);
		expect(result.items[0].method).toBe('GET');
		expect(result.items[0].failure_count).toBe(5);
	});

	it('forwards window and limit query params', async () => {
		const fetchFn = mockFetch(FAILURES_BODY);
		await dashboardApi.getFailures('org_abc', { fetch: fetchFn, timeWindow: '7d', limit: 10 });
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('window=7d');
		expect(call[0]).toContain('limit=10');
	});

	it('forwards limit_clamped through response', async () => {
		const body = { ...FAILURES_BODY, limit_clamped: true };
		const fetchFn = mockFetch(body);
		const result = await dashboardApi.getFailures('org_abc', { fetch: fetchFn });
		expect(result.limit_clamped).toBe(true);
	});

	it('omits window and limit params when not specified', async () => {
		const fetchFn = mockFetch(FAILURES_BODY);
		await dashboardApi.getFailures('org_abc', { fetch: fetchFn });
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).not.toContain('window=');
		expect(call[0]).not.toContain('limit=');
	});

	it('includes orgId in the request URL', async () => {
		const fetchFn = mockFetch(FAILURES_BODY);
		await dashboardApi.getFailures('org_abc', { fetch: fetchFn });
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('/organizations/org_abc/results/failures');
	});
});

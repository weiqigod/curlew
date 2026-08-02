import { describe, it, expect, vi } from 'vitest';
import { schedulesApi } from './schedules';
import type { Schedule, ScheduledRun } from '$lib/types/schedules';

function makeSchedule(overrides: Partial<Schedule> = {}): Schedule {
	return {
		id: 'sched_abc123',
		name: 'nightly',
		cron_expression: '0 2 * * *',
		timezone: 'UTC',
		collection_ref: 'smoke.yaml',
		enabled: true,
		next_run_at: '2026-05-12T02:00:00Z',
		last_run_at: null,
		created_at: '2026-05-11T00:00:00Z',
		...overrides
	};
}

function makeRun(overrides: Partial<ScheduledRun> = {}): ScheduledRun {
	return {
		run_id: 'run_abc123',
		status: 'queued',
		created_at: '2026-05-11T00:00:00Z',
		started_at: null,
		completed_at: null,
		result_id: null,
		...overrides
	};
}

function mockFetch(body: unknown, status = 200): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: status >= 200 && status < 300,
		status,
		json: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

describe('schedulesApi', () => {
	it('list parses {schedules: [...]} envelope', async () => {
		const schedule = makeSchedule({ timezone: 'Europe/Stockholm' });
		const fetchFn = mockFetch({ schedules: [schedule] });

		const result = await schedulesApi.list('org_abc', { fetch: fetchFn });

		expect(result).toHaveLength(1);
		expect(result[0].timezone).toBe('Europe/Stockholm');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('/organizations/org_abc/schedules');
	});

	it('create posts body and returns dto', async () => {
		const created = makeSchedule({ name: 'morning', timezone: 'Asia/Tokyo' });
		const fetchFn = mockFetch(created, 201);

		const result = await schedulesApi.create(
			'org_abc',
			{ name: 'morning', cron: '0 9 * * *', timezone: 'Asia/Tokyo', collection_ref: 'smoke.yaml' },
			{ fetch: fetchFn }
		);

		expect(result.timezone).toBe('Asia/Tokyo');
		expect(result.name).toBe('morning');
	});

	it('runNow returns {run_id, status}', async () => {
		const fetchFn = mockFetch({ run_id: 'run_abc', status: 'queued' }, 202);

		const result = await schedulesApi.runNow('org_abc', 'nightly', { fetch: fetchFn });

		expect(result.run_id).toBe('run_abc');
		expect(result.status).toBe('queued');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('/schedules/nightly/run-now');
	});

	it('listRuns clamps limit and parses envelope', async () => {
		const run = makeRun();
		const fetchFn = mockFetch({ runs: [run] });

		const result = await schedulesApi.listRuns('org_abc', 'nightly', { fetch: fetchFn, limit: 5 });

		expect(result).toHaveLength(1);
		expect(result[0].run_id).toBe('run_abc123');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('limit=5');
	});

	it('list surfaces ApiError on 402 tier-ineligible', async () => {
		const fetchFn = mockFetch(
			{
				type: 'https://api.apitool.dev/errors/tier-ineligible',
				code: 'schedule_executor_tier_ineligible',
				title: 'Tier ineligible'
			},
			402
		);

		await expect(schedulesApi.list('org_abc', { fetch: fetchFn })).rejects.toMatchObject({
			status: 402,
			problemType: 'https://api.apitool.dev/errors/tier-ineligible'
		});
	});
});

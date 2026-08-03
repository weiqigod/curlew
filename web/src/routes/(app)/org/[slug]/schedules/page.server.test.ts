import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Schedule } from '$lib/types/schedules';
import { load } from './+page.server';

const findBySlug = vi.fn();
const listSchedules = vi.fn();
const listRuns = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/schedules', () => ({
	schedulesApi: {
		list: (...a: unknown[]) => listSchedules(...a),
		listRuns: (...a: unknown[]) => listRuns(...a)
	}
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'team' };

const SCHEDULES = [
	{ name: 'nightly', cron_expression: '0 2 * * *', timezone: 'UTC' },
	{ name: 'weekly', cron_expression: '0 3 * * 1', timezone: 'UTC' }
] as Schedule[];

function event() {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL('http://localhost/org/acme/schedules'),
		fetch: vi.fn()
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	} as any;
}

async function loadThrows(ev: unknown) {
	try {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		await load(ev as any);
	} catch (e) {
		return e as { status: number; location?: string };
	}
	throw new Error('expected load to throw');
}

beforeEach(() => {
	vi.clearAllMocks();
	findBySlug.mockResolvedValue(ORG);
	listSchedules.mockResolvedValue(SCHEDULES);
	listRuns.mockResolvedValue([{ status: 'completed' }]);
});

describe('schedules load', () => {
	it('returns the schedule list with the most-recent run for each, positionally aligned', async () => {
		listRuns
			.mockResolvedValueOnce([{ status: 'completed' }])
			.mockResolvedValueOnce([{ status: 'failed' }]);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.schedules).toEqual(SCHEDULES);
		expect(r.lastRuns).toEqual([{ status: 'completed' }, { status: 'failed' }]);
		expect(r.error).toBeNull();
		// One run lookup per schedule, each capped at the newest entry.
		expect(listRuns).toHaveBeenCalledTimes(2);
		expect(listRuns).toHaveBeenCalledWith(
			'org_1',
			'nightly',
			expect.objectContaining({ limit: 1 })
		);
	});

	it('maps a schedule with no runs to null rather than dropping it', async () => {
		listRuns.mockResolvedValue([]);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.lastRuns).toEqual([null, null]);
		expect(r.schedules).toHaveLength(2);
	});

	it('swallows a per-schedule run-lookup failure and still renders the list', async () => {
		listRuns
			.mockRejectedValueOnce(new Error('runs unavailable'))
			.mockResolvedValueOnce([{ status: 'queued' }]);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.error).toBeNull();
		expect(r.lastRuns).toEqual([null, { status: 'queued' }]);
	});

	it('degrades to an error message when the schedules list itself fails', async () => {
		listSchedules.mockRejectedValue(new Error('schedules unavailable'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.error).toBe('schedules unavailable');
		expect(r.schedules).toEqual([]);
		expect(r.lastRuns).toEqual([]);
	});

	it('redirects below team tier with the team_tier_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, tier: 'professional' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=team_tier_required');
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		expect((await loadThrows(event())).status).toBe(404);
	});

	it('redirects to /login when unauthenticated', async () => {
		const e = await loadThrows({ ...event(), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fschedules');
	});
});

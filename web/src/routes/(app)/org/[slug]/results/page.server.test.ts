import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Result } from '$lib/types/results';
import { load } from './+page.server';

const findBySlug = vi.fn();
const listResults = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/results', () => ({
	resultsApi: { list: (...a: unknown[]) => listResults(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'team' };

/** Two runs: one just now, one 40 days ago — straddles the default 30d window. */
function fixtures(): Result[] {
	return [
		{
			id: 'recent',
			collection_name: 'smoke-tests',
			run_at: new Date(Date.now() - 60_000).toISOString(),
			pass_count: 9,
			fail_count: 1,
			duration_ms: 1000,
			triggered_by: 'owner@example.com'
		},
		{
			id: 'old',
			collection_name: 'regression',
			run_at: new Date(Date.now() - 40 * 86_400_000).toISOString(),
			pass_count: 5,
			fail_count: 0,
			duration_ms: 500,
			triggered_by: 'owner@example.com'
		}
	] as Result[];
}

function event(search = '') {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL(`http://localhost/org/acme/results${search}`),
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
	listResults.mockResolvedValue(fixtures());
});

describe('results load', () => {
	it('defaults to the 30d range and filters out older runs', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.range).toBe('30d');
		expect(r.results.map((x: Result) => x.id)).toEqual(['recent']);
		expect(r.summary.total_runs).toBe(1);
		expect(r.error).toBeNull();
	});

	it('honours ?range=all and keeps every run', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event('?range=all'))) as any;

		expect(r.range).toBe('all');
		expect(r.results).toHaveLength(2);
		expect(r.summary.total_runs).toBe(2);
		expect(r.trend).toHaveLength(2);
	});

	it('falls back to 30d for a range value outside the allow-list', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event('?range=14d'))) as any;

		expect(r.range).toBe('30d');
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

	it('degrades to an error message with an empty summary when the fetch fails', async () => {
		listResults.mockRejectedValue(new Error('boom'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.error).toBe('boom');
		expect(r.results).toEqual([]);
		expect(r.summary.total_runs).toBe(0);
		expect(r.trend).toEqual([]);
	});

	it('redirects to /login preserving the query string', async () => {
		const e = await loadThrows({ ...event('?range=7d'), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fresults%3Frange%3D7d');
	});
});

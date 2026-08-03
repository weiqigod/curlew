import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import { load } from './+page.server';

const findBySlug = vi.fn();
const getStats = vi.fn();
const getFailures = vi.fn();
const listResults = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/dashboard', () => ({
	dashboardApi: {
		getStats: (...a: unknown[]) => getStats(...a),
		getFailures: (...a: unknown[]) => getFailures(...a)
	}
}));
vi.mock('$lib/api/results', () => ({
	resultsApi: { list: (...a: unknown[]) => listResults(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'team' };
const STATS = { window: '30d', totals: { runs: 42 }, trend: [] };
const FAILURES = { window: '30d', limit: 10, limit_clamped: false, items: [] };

function event(search = '') {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL(`http://localhost/org/acme/dashboard${search}`),
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
	getStats.mockResolvedValue(STATS);
	getFailures.mockResolvedValue(FAILURES);
	listResults.mockResolvedValue([]);
});

describe('dashboard load', () => {
	it('returns stats, failures and recent runs for a team-tier org', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.tierGate).toBe(false);
		expect(r.stats).toEqual(STATS);
		expect(r.failures).toEqual(FAILURES);
		expect(r.windowError).toBeNull();
		expect(r.loadError).toBeNull();
		// The window echoed by the backend wins over the requested one.
		expect(r.window).toBe('30d');
	});

	it('forwards the raw window value to the backend rather than validating locally', async () => {
		await load(event('?window=14d'));

		expect(getStats).toHaveBeenCalledWith(
			'org_1',
			expect.objectContaining({ timeWindow: '14d' })
		);
		expect(getFailures).toHaveBeenCalledWith(
			'org_1',
			expect.objectContaining({ timeWindow: '14d', limit: 10 })
		);
	});

	it('short-circuits below team tier without fetching stats', async () => {
		findBySlug.mockResolvedValue({ ...ORG, tier: 'free' });

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.tierGate).toBe(true);
		expect(r.stats).toBeNull();
		expect(r.window).toBe('30d');
		// The doomed fetch is skipped entirely.
		expect(getStats).not.toHaveBeenCalled();
		expect(getFailures).not.toHaveBeenCalled();
	});

	it('maps a 402 from /results/stats onto the tier gate', async () => {
		getStats.mockRejectedValue(new ApiError('dashboard_tier_ineligible', 'Tier ineligible', 402));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.tierGate).toBe(true);
		expect(r.stats).toBeNull();
		// Sibling results are suppressed behind the gate.
		expect(r.failures).toBeNull();
		expect(r.recentRuns).toEqual([]);
	});

	it('maps a 400 from /results/stats onto a window error', async () => {
		getStats.mockRejectedValue(
			new ApiError('unsupported_window', 'Allowed values: 7d, 30d, 90d.', 400)
		);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event('?window=14d'))) as any;

		expect(r.windowError).toBe('Allowed values: 7d, 30d, 90d.');
		expect(r.tierGate).toBe(false);
		expect(r.failures).toBeNull();
		// Falls back to the requested window for the picker.
		expect(r.window).toBe('14d');
	});

	it('reports any other stats failure as a load error', async () => {
		getStats.mockRejectedValue(new Error('socket hang up'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.loadError).toBe('socket hang up');
		expect(r.tierGate).toBe(false);
		expect(r.windowError).toBeNull();
	});

	it('still renders stats when only the recent-runs fetch fails', async () => {
		listResults.mockRejectedValue(new Error('results unavailable'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.stats).toEqual(STATS);
		expect(r.recentRuns).toEqual([]);
		expect(r.loadError).toBeNull();
	});

	it('redirects to /login preserving the query string', async () => {
		const e = await loadThrows({ ...event('?window=7d'), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fdashboard%3Fwindow%3D7d');
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		expect((await loadThrows(event())).status).toBe(404);
	});
});

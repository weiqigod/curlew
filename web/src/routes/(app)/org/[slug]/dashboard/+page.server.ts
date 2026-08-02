import type { PageServerLoad } from './$types';
import { requireAuth } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { dashboardApi } from '$lib/api/dashboard';
import { resultsApi } from '$lib/api/results';
import { ApiError } from '$lib/types/api-error';
import { error } from '@sveltejs/kit';
import type { StatsResponse, FailuresResponse } from '$lib/types/dashboard';
import type { Result } from '$lib/types/results';

/** Loads all data needed by the /dashboard page. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);
	// Forward the raw window value to the backend — it owns the allow-list (Open Decision 10).
	const rawWindow = url.searchParams.get('window') ?? undefined;

	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	if (!org) throw error(404, 'Organization not found');

	// Below-Team short-circuit: render the upgrade prompt without a doomed fetch.
	if (org.tier !== 'team' && org.tier !== 'enterprise') {
		return {
			org,
			window: rawWindow ?? '30d',
			tierGate: true as const,
			stats: null as StatsResponse | null,
			failures: null as FailuresResponse | null,
			recentRuns: [] as Result[],
			windowError: null as string | null,
			loadError: null as string | null
		};
	}

	let stats: StatsResponse | null = null;
	let failures: FailuresResponse | null = null;
	let recentRuns: Result[] = [];
	let tierGate = false;
	let windowError: string | null = null;
	let loadError: string | null = null;

	// Fan out all three requests in parallel; recent-runs failure degrades gracefully.
	const [statsRes, failuresRes, runsRes] = await Promise.allSettled([
		dashboardApi.getStats(org.id, { token, fetch, timeWindow: rawWindow }),
		dashboardApi.getFailures(org.id, { token, fetch, timeWindow: rawWindow, limit: 10 }),
		resultsApi.list(org.id, { token, fetch, limit: 10 })
	]);

	// Stats governs the page state — tier gate or window error short-circuits the rest.
	if (statsRes.status === 'fulfilled') {
		stats = statsRes.value;
	} else {
		const e = statsRes.reason;
		if (e instanceof ApiError && e.status === 402) {
			tierGate = true;
		} else if (e instanceof ApiError && e.status === 400) {
			windowError = e.message;
		} else {
			loadError = e instanceof Error ? e.message : 'Failed to load dashboard';
		}
	}

	if (failuresRes.status === 'fulfilled' && !tierGate && !windowError) {
		failures = failuresRes.value;
	}
	if (runsRes.status === 'fulfilled' && !tierGate && !windowError) {
		recentRuns = runsRes.value;
	}

	return {
		org,
		window: stats?.window ?? rawWindow ?? '30d',
		tierGate,
		stats,
		failures,
		recentRuns,
		windowError,
		loadError
	};
};

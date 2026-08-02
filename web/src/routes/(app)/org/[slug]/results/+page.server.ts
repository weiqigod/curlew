import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { resultsApi } from '$lib/api/results';
import { summarize, bucketByDay, filterByRange } from '$lib/results/stats';
import type { TimeRange } from '$lib/types/results';

const ALLOWED_RANGES: TimeRange[] = ['24h', '7d', '30d', 'all'];

/** Loads results data for the team dashboard page. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);
	const rawRange = url.searchParams.get('range') ?? '30d';
	const range: TimeRange = (ALLOWED_RANGES as string[]).includes(rawRange)
		? (rawRange as TimeRange)
		: '30d';

	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);

	try {
		const allResults = await resultsApi.list(teamOrg.id, { token, fetch, limit: 100 });
		const scoped = filterByRange(allResults, range);
		return {
			org: teamOrg,
			range,
			results: scoped,
			summary: summarize(scoped),
			trend: bucketByDay(scoped),
			error: null as string | null
		};
	} catch (e) {
		const message = e instanceof Error ? e.message : 'failed to load results';
		return {
			org: teamOrg,
			range,
			results: [],
			summary: summarize([]),
			trend: [],
			error: message
		};
	}
};

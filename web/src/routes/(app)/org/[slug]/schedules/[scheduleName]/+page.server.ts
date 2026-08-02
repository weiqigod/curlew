import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { requireAuth, requireTeamTier } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { schedulesApi } from '$lib/api/schedules';
import { ApiError } from '$lib/types/api-error';

/** Loads a schedule and its run history. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);

	try {
		const [schedule, runs] = await Promise.all([
			schedulesApi.get(teamOrg.id, params.scheduleName, { token, fetch }),
			schedulesApi.listRuns(teamOrg.id, params.scheduleName, { token, fetch, limit: 50 })
		]);

		return { org: teamOrg, schedule, runs, error: null as string | null };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, `Schedule '${params.scheduleName}' not found`);
		}
		const message = e instanceof Error ? e.message : 'failed to load schedule runs';
		return { org: teamOrg, schedule: null, runs: [], error: message };
	}
};

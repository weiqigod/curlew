import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { schedulesApi } from '$lib/api/schedules';
import type { ScheduledRun } from '$lib/types/schedules';

/** Loads the schedules list + most-recent run for each schedule. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);

	try {
		const schedules = await schedulesApi.list(teamOrg.id, { token, fetch });

		// Fetch the most-recent run for each schedule in parallel (limit=1).
		// Failures on individual run lookups are swallowed — list still renders.
		const lastRuns: Array<ScheduledRun | null> = await Promise.all(
			schedules.map((s) =>
				schedulesApi
					.listRuns(teamOrg.id, s.name, { token, fetch, limit: 1 })
					.then((rs) => rs[0] ?? null)
					.catch(() => null)
			)
		);

		return { org: teamOrg, schedules, lastRuns, error: null as string | null };
	} catch (e) {
		const message = e instanceof Error ? e.message : 'failed to load schedules';
		return { org: teamOrg, schedules: [], lastRuns: [], error: message };
	}
};

import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { prChecksApi } from '$lib/api/pr-checks';

/** Loads PR check data for the team pr-checks page. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);

	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);

	try {
		const checks = await prChecksApi.list(teamOrg.id, { token, fetch, limit: 50 });
		return {
			org: teamOrg,
			checks,
			error: null as string | null
		};
	} catch (e) {
		const message = e instanceof Error ? e.message : 'failed to load pr checks';
		return {
			org: teamOrg,
			checks: [],
			error: message
		};
	}
};

import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier, requireOrgAdmin } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { notificationsApi } from '$lib/api/notifications';

/** Loads notification rules and delivery log for an admin of a team-tier org. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);
	const adminOrg = requireOrgAdmin(teamOrg, params.slug);

	try {
		const [rules, deliveries] = await Promise.all([
			notificationsApi.listRules(adminOrg.id, { token, fetch }),
			notificationsApi.listDeliveries(adminOrg.id, { token, fetch, limit: 25 })
		]);
		return { org: adminOrg, rules, deliveries, error: null as string | null };
	} catch (e) {
		const message = e instanceof Error ? e.message : 'Failed to load notifications.';
		return { org: adminOrg, rules: [], deliveries: [], error: message };
	}
};

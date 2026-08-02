import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier, requireOrgOwner } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { subscriptionsApi } from '$lib/api/subscriptions';

/** Loads billing subscription data for an owner of a team-tier org. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);
	const ownerOrg = requireOrgOwner(teamOrg, params.slug);

	try {
		const { subscription, tier } = await subscriptionsApi.get(ownerOrg.id, { token, fetch });
		return { org: ownerOrg, subscription, tier, error: null as string | null };
	} catch (e) {
		const message = e instanceof Error ? e.message : 'Failed to load subscription.';
		return { org: ownerOrg, subscription: null, tier: ownerOrg.tier, error: message };
	}
};

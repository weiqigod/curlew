import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier, requireOrgAdmin } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { membersApi } from '$lib/api/members';
import { invitationsApi } from '$lib/api/invitations';

/** Loads members and pending invitations for an admin of a team-tier org. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);
	const adminOrg = requireOrgAdmin(teamOrg, params.slug);

	try {
		const [members, invitations] = await Promise.all([
			membersApi.list(adminOrg.id, { token, fetch }),
			invitationsApi.list(adminOrg.id, { token, fetch })
		]);
		return { org: adminOrg, members, invitations, error: null as string | null };
	} catch (e) {
		const message = e instanceof Error ? e.message : 'Failed to load members.';
		return { org: adminOrg, members: [], invitations: [], error: message };
	}
};

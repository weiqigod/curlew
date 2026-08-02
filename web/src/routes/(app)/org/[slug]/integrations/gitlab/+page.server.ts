// SSR loader for /org/[slug]/integrations/gitlab (M16-016).
// Tier-gated (Team or above). Loads the list of GitLab installations for the org.
import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { gitlabIntegrationsApi } from '$lib/api/gitlab-integrations';
import type { GitLabInstallation } from '$lib/types/gitlab-integrations';
import { ApiError } from '$lib/types/api-error';

export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);

	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);

	let installations: GitLabInstallation[] = [];
	let loadError: string | null = null;

	try {
		installations = await gitlabIntegrationsApi.list(teamOrg.id, { token, fetch });
	} catch (e) {
		if (e instanceof ApiError && e.status === 402) {
			// requireTeamTier should already have redirected — be defensive.
			throw e;
		}
		loadError = e instanceof Error ? e.message : 'Failed to load integrations';
	}

	return { org: teamOrg, installations, loadError };
};

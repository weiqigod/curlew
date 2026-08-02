import type { PageServerLoad } from './$types';
import { requireAuth, requireEnterpriseTier, requireOrgOwner } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { ssoApi } from '$lib/api/sso';
import type { SsoConfigView } from '$lib/types/sso';

const emptySsoConfig: SsoConfigView = {
	sso_enabled: false,
	sso_provider: null,
	saml_config: null,
	oidc_config: null
};

/** Loads the SSO configuration for an owner of an enterprise-tier org. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const entOrg = requireEnterpriseTier(org, params.slug);
	const ownerOrg = requireOrgOwner(entOrg, params.slug);

	try {
		const config = await ssoApi.get(ownerOrg.id, { token, fetch });
		return { org: ownerOrg, config, error: null as string | null };
	} catch (e) {
		const message = e instanceof Error ? e.message : 'Failed to load SSO configuration.';
		return {
			org: ownerOrg,
			config: emptySsoConfig,
			error: message
		};
	}
};

import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { vaultConfigApi } from '$lib/api/vault-config';
import { auditLogApi } from '$lib/api/audit-log';

/** SSR loader for the vault-config page. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);

	// Fetch current vault-config (null if not yet created) and last 10 audit entries in parallel.
	const [currentConfig, auditEntries] = await Promise.all([
		vaultConfigApi.get(teamOrg.id, { token, fetch }).catch(() => null),
		auditLogApi
			.list(teamOrg.id, {
				token,
				fetch,
				event_type: 'vault_config.upserted,vault_config.deleted',
				limit: 10,
			})
			.catch(() => []),
	]);

	return { org: teamOrg, currentConfig, auditEntries };
};

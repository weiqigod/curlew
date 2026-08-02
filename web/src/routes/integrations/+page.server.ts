// Refs docs/SPECIFICATION.md:8413–8417 (Installation Lifecycle:
// dashboard-initiated vs webhook-first install paths).
import type { PageServerLoad } from './$types';
import { requireAuth } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { api } from '$lib/api/client';
import { ApiError } from '$lib/types/api-error';

/** URL returned by the install-url endpoint. */
interface InstallUrlResponse {
	install_url: string;
	state_expires_at: string;
}

/** A repository reference within an installation's repo_set. */
interface RepoRef {
	owner: string;
	name: string;
	id?: number;
}

/** A GitHub App installation as returned by the state endpoint. */
interface InstallationDto {
	installation_id: number;
	account_login: string;
	account_type: string;
	repo_set: RepoRef[];
	claimed_at: string | null;
	suspended_at: string | null;
}

interface InstallStateResponse {
	installation: InstallationDto | null;
}

/** SSR loader for /integrations.
 *
 * Resolves the authenticated user's first organisation, fetches the GitHub
 * install-url (admin/owner only) and the current install state. RBAC:
 * only admin/owner can see the Connect button — members get a read-only view.
 *
 * Per Open Decision #10 the touched surface is bounded to
 * web/src/routes/integrations/* only — no billing, no SSO, no dashboards.
 */
export const load: PageServerLoad = async ({ locals, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);

	const orgs = await organizationsApi.list({ token, fetch }).catch(() => []);
	const org = orgs[0] ?? null;
	const isAdmin = !!org && (org.role === 'owner' || org.role === 'admin');

	let installUrl: string | null = null;
	let installation: InstallationDto | null = null;
	let stateError: string | null = null;

	if (isAdmin && org) {
		try {
			const r = await api.get<InstallUrlResponse>(
				'/api/v1/integrations/github/install-url',
				{ token, fetch }
			);
			installUrl = r.install_url;
		} catch (e) {
			// Non-fatal — Connect button is hidden if the URL cannot be minted.
			installUrl = null;
			if (e instanceof ApiError && e.status >= 500) {
				stateError = e.message;
			}
		}
	}

	if (org) {
		try {
			const r = await api.get<InstallStateResponse>('/api/v1/integrations/github', {
				token,
				fetch
			});
			installation = r.installation;
		} catch (e) {
			if (e instanceof ApiError && e.status === 404) {
				installation = null; // not installed yet — expected state
			} else if (e instanceof ApiError) {
				stateError = e.message;
			} else {
				stateError = e instanceof Error ? e.message : 'failed to load install state';
			}
		}
	}

	return {
		org,
		isAdmin,
		installUrl,
		installation,
		stateError,
		showInstalledToast: url.searchParams.get('installed') === 'true'
	};
};

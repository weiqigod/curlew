// M14-021: org-scoped GitHub integration view.
// Shows install state and the most-recent pr-check's posted_at / check_run_id
// from the M14-018 outbound Checks API poster.
import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { prChecksApi } from '$lib/api/pr-checks';
import { api } from '$lib/api/client';
import { ApiError } from '$lib/types/api-error';

/** A repository reference within an installation's repo_set. */
interface RepoRef {
	owner: string;
	name: string;
	id?: number;
}

/** GitHub App installation as returned by GET /api/v1/integrations/github. */
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

/** SSR loader for /org/<slug>/integrations/github.
 *
 * Loads the GitHub install state for the org and the most-recent pr-check
 * (including posted_at and check_run_id) so the Playwright spec can assert
 * that a check-run was posted within 5 seconds of the upload.
 */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);

	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const teamOrg = requireTeamTier(org, params.slug);

	let installation: InstallationDto | null = null;
	let installationError: string | null = null;

	try {
		const r = await api.get<InstallStateResponse>('/api/v1/integrations/github', {
			token,
			fetch
		});
		installation = r.installation;
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			installation = null;
		} else {
			installationError = e instanceof Error ? e.message : 'failed to load install state';
		}
	}

	// Load the most-recent pr-check to surface posted_at for the convergence spec.
	let lastCheck = null;
	try {
		const checks = await prChecksApi.list(teamOrg.id, { token, fetch, limit: 1 });
		lastCheck = checks[0] ?? null;
	} catch {
		// Non-fatal — page renders without check data.
	}

	return {
		org: teamOrg,
		installation,
		installationError,
		lastCheck
	};
};

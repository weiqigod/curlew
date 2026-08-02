// API client for GitLab integrations (M16-016).
// Refs docs/SPECIFICATION.md:9177-9195 (Per-Org GitLab Configuration).
import { api, type RequestOptions } from './client';
import type {
	GitLabInstallation,
	CreateGitLabIntegrationRequest
} from '$lib/types/gitlab-integrations';

interface ListResponse {
	installations: GitLabInstallation[];
}

/** Typed client for /api/v1/integrations/gitlab. */
export const gitlabIntegrationsApi = {
	/**
	 * List all active (non-deleted) GitLab installations for the given org.
	 * Requires Team tier — throws ApiError(402) for Free-tier orgs.
	 * Pass `orgId` explicitly so multi-org users do not get a 400 ambiguous_org_id error.
	 */
	async list(orgId: string, opts: RequestOptions = {}): Promise<GitLabInstallation[]> {
		const r = await api.get<ListResponse>(
			`/api/v1/integrations/gitlab?org_id=${encodeURIComponent(orgId)}`,
			opts
		);
		return r.installations;
	},

	/**
	 * Create a new GitLab integration for the given org.
	 * Resolves project_path → numeric project_id via the GitLab API before persisting.
	 * Returns the created installation row (PAT is never echoed back).
	 */
	async create(
		orgId: string,
		body: CreateGitLabIntegrationRequest,
		opts: RequestOptions = {}
	): Promise<GitLabInstallation> {
		return await api.post<GitLabInstallation>(
			`/api/v1/integrations/gitlab?org_id=${encodeURIComponent(orgId)}`,
			body,
			opts
		);
	},

	/**
	 * Soft-delete a GitLab installation by id.
	 * Returns void on 204 success.
	 */
	async remove(orgId: string, id: string, opts: RequestOptions = {}): Promise<void> {
		await api.delete<void>(
			`/api/v1/integrations/gitlab/${encodeURIComponent(id)}?org_id=${encodeURIComponent(orgId)}`,
			opts
		);
	}
};

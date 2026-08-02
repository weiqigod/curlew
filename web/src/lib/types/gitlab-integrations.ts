// Types for GitLab integrations (M16-016).
// Refs docs/SPECIFICATION.md:9177-9195 (Per-Org GitLab Configuration).

/** A single GitLab installation row returned by GET /api/v1/integrations/gitlab. */
export interface GitLabInstallation {
	id: string;
	project_id: number;
	project_path: string;
	gitlab_base_url: string;
	created_at: string;
	access_token_revoked_at: string | null;
	last_status_post_at: string | null;
}

/** Request body for POST /api/v1/integrations/gitlab. */
export interface CreateGitLabIntegrationRequest {
	project_path: string;
	access_token: string;
	gitlab_base_url?: string;
	gitlab_ca_bundle?: string;
}

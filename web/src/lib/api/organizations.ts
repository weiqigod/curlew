import { api, type RequestOptions } from './client';
import type { Organization } from '$lib/types/organization';

interface ListResponse {
	organizations: Organization[];
}

/** API client for organization endpoints. */
export const organizationsApi = {
	/** List all organizations the authenticated user belongs to. */
	async list(opts?: RequestOptions): Promise<Organization[]> {
		const { organizations } = await api.get<ListResponse>('/api/v1/organizations', opts);
		return organizations;
	},

	/** Find an organization by slug, returning null if not found. */
	async findBySlug(slug: string, opts?: RequestOptions): Promise<Organization | null> {
		const orgs = await this.list(opts);
		return orgs.find((o) => o.slug === slug) ?? null;
	}
};

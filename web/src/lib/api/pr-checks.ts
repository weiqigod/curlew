import { api, type RequestOptions } from './client';
import type { PrCheck } from '$lib/types/pr-checks';

interface ListPrChecksResponse {
	pr_checks: PrCheck[];
}

/** API client for pr-checks endpoints. */
export const prChecksApi = {
	/**
	 * List the most recent PR checks for an organization.
	 * Backend clamps limit to [1, 100].
	 */
	async list(
		orgId: string,
		opts: RequestOptions & { limit?: number } = {}
	): Promise<PrCheck[]> {
		const { limit = 50, ...rest } = opts;
		const { pr_checks } = await api.get<ListPrChecksResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/pr-checks`,
			{ ...rest, params: { ...rest.params, limit } }
		);
		return pr_checks;
	}
};

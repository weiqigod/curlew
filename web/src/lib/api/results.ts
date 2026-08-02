import { api, type RequestOptions } from './client';
import type { Result } from '$lib/types/results';

interface ListResultsResponse {
	results: Result[];
}

/** API client for test results endpoints. */
export const resultsApi = {
	/**
	 * List the most recent results for an organization.
	 * Backend clamps limit to [1, 100].
	 *
	 * **Design note (time-range filtering):** The `?range` parameter is intentionally
	 * applied client-side (in `stats.filterByRange`) rather than being forwarded to
	 * the backend. The M4-004 `ListResults` endpoint does not accept a `range`
	 * parameter — it accepts only `limit`. We fetch the maximum of 100 results and
	 * filter in the loader. A future backend stats endpoint (e.g. `/results/stats`)
	 * tracked separately can accept date ranges server-side; until then this approach
	 * keeps M4-005 self-contained and avoids a breaking change to the M4-004 API.
	 */
	async list(
		orgId: string,
		opts: RequestOptions & { limit?: number } = {}
	): Promise<Result[]> {
		const { limit = 100, ...rest } = opts;
		const { results } = await api.get<ListResultsResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/results`,
			{ ...rest, params: { ...rest.params, limit } }
		);
		return results;
	}
};

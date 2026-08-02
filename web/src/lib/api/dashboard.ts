import { api, type RequestOptions } from './client';
import type { StatsResponse, FailuresResponse } from '$lib/types/dashboard';

/** Options for dashboard stats requests. */
export interface StatsOptions extends RequestOptions {
	/** Time window query param (e.g. "7d", "30d", "90d"). Forwarded verbatim to the backend. */
	timeWindow?: string;
}

/** Options for dashboard failures requests. */
export interface FailuresOptions extends RequestOptions {
	/** Time window query param (e.g. "7d", "30d", "90d"). Forwarded verbatim to the backend. */
	timeWindow?: string;
	/** Maximum number of failure groups to return. Backend clamps to 50. */
	limit?: number;
}

/** API client for dashboard statistics endpoints. */
export const dashboardApi = {
	/**
	 * Fetch aggregated run stats for the given org and optional time window.
	 * Throws `ApiError` with status 402 for tier-ineligible orgs,
	 * and status 400 for unsupported `timeWindow` values.
	 */
	async getStats(orgId: string, opts: StatsOptions = {}): Promise<StatsResponse> {
		const { timeWindow, ...rest } = opts;
		return api.get<StatsResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/results/stats`,
			{
				...rest,
				params: {
					...rest.params,
					...(timeWindow !== undefined ? { window: timeWindow } : {})
				}
			}
		);
	},

	/**
	 * Fetch the top frequently-failing endpoints for the given org and optional time window.
	 * Results are ordered by failure_count descending.
	 */
	async getFailures(orgId: string, opts: FailuresOptions = {}): Promise<FailuresResponse> {
		const { timeWindow, limit, ...rest } = opts;
		return api.get<FailuresResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/results/failures`,
			{
				...rest,
				params: {
					...rest.params,
					...(timeWindow !== undefined ? { window: timeWindow } : {}),
					...(limit !== undefined ? { limit } : {})
				}
			}
		);
	}
};

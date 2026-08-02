import { api, type RequestOptions } from './client';
import type { CreateScheduleRequest, Schedule, ScheduledRun } from '$lib/types/schedules';

interface ListSchedulesResponse {
	schedules: Schedule[];
}

interface ListRunsResponse {
	runs: ScheduledRun[];
}

/** API client for cron schedules and their run history. */
export const schedulesApi = {
	/**
	 * List all schedules for an organization.
	 * Requires Team tier or above (backend returns 402 for Free orgs).
	 */
	async list(orgId: string, opts: RequestOptions = {}): Promise<Schedule[]> {
		const { schedules } = await api.get<ListSchedulesResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/schedules`,
			opts
		);
		return schedules;
	},

	/** Create a new cron schedule for an organization. */
	async create(
		orgId: string,
		body: CreateScheduleRequest,
		opts: RequestOptions = {}
	): Promise<Schedule> {
		return await api.post<Schedule>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/schedules`,
			body,
			opts
		);
	},

	/** Get a single schedule by name. */
	async get(orgId: string, name: string, opts: RequestOptions = {}): Promise<Schedule> {
		return await api.get<Schedule>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/schedules/${encodeURIComponent(name)}`,
			opts
		);
	},

	/** Immediately enqueue a run for the named schedule. Returns 202 with run_id + status. */
	async runNow(
		orgId: string,
		name: string,
		opts: RequestOptions = {}
	): Promise<{ run_id: string; status: string }> {
		return await api.post(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/schedules/${encodeURIComponent(name)}/run-now`,
			undefined,
			opts
		);
	},

	/** List runs for the named schedule, newest first. Backend clamps limit to [1, 100]. */
	async listRuns(
		orgId: string,
		name: string,
		opts: RequestOptions & { limit?: number } = {}
	): Promise<ScheduledRun[]> {
		const { limit = 50, ...rest } = opts;
		const { runs } = await api.get<ListRunsResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/schedules/${encodeURIComponent(name)}/runs`,
			{ ...rest, params: { ...rest.params, limit } }
		);
		return runs;
	}
};

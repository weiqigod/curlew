import { api, type RequestOptions } from './client';
import type { UserExportRequest } from '$lib/types/user-data';

/** Typed API client for the GDPR user data export endpoints (M18-004). */
export const userDataApi = {
	/**
	 * Creates a new export request for the authenticated user.
	 * Returns 202 on success or throws `ApiError` with status 429 on rate-limit.
	 */
	async createExportRequest(opts: RequestOptions = {}): Promise<UserExportRequest> {
		return api.post<UserExportRequest>('/api/v1/users/me/export-requests', undefined, opts);
	},

	/**
	 * Polls the status of an export request by id.
	 * When status is `ready`, the response includes `signed_url` and `expires_at`.
	 */
	async getExportRequest(id: string, opts: RequestOptions = {}): Promise<UserExportRequest> {
		return api.get<UserExportRequest>(`/api/v1/users/me/export-requests/${encodeURIComponent(id)}`, opts);
	},
};

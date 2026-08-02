import { api, type RequestOptions } from './client';
import type {
	DeletionRequestResponse,
	ReauthTokenResponse,
	UserDeletionStatus,
} from '$lib/types/user-deletion';

/** Typed API client for the GDPR account deletion endpoints (M18-005 / M18-006). */
export const userDeletionApi = {
	/**
	 * Issues a short-lived re-auth token (drto_ prefix) by verifying the user's
	 * current password. The returned token must be passed as the `X-Reauth-Token`
	 * header when calling `requestDeletion`.
	 * Throws `ApiError` (401) when the password is incorrect.
	 */
	async issueReauthToken(password: string, opts: RequestOptions = {}): Promise<ReauthTokenResponse> {
		return api.post<ReauthTokenResponse>('/api/v1/auth/reauth', { password }, opts);
	},

	/**
	 * Initiates a 30-day GDPR account deletion request.
	 * Requires `reauthToken` (from `issueReauthToken`) in the `X-Reauth-Token` header.
	 * Returns 202 on success with `finalizes_at` and `cancel_url`.
	 * Throws `ApiError` with status 409 and `code = 'owner_cannot_leave'` when the
	 * user is the sole owner of an org with other members.
	 */
	async requestDeletion(
		reauthToken: string,
		opts: RequestOptions = {}
	): Promise<DeletionRequestResponse> {
		return api.post<DeletionRequestResponse>('/api/v1/users/me/deletion-requests', undefined, {
			...opts,
			headers: {
				...opts.headers,
				'X-Reauth-Token': reauthToken,
			},
		});
	},

	/**
	 * Cancels a pending account deletion request.
	 * Returns 200 on success, 404 when no pending request exists.
	 */
	async cancelDeletion(opts: RequestOptions = {}): Promise<void> {
		await api.post('/api/v1/users/me/deletion-requests/cancel', undefined, opts);
	},

	/**
	 * Retrieves the current deletion request status for the authenticated user.
	 * Returns 404 when no pending or completed deletion request exists.
	 */
	async getStatus(opts: RequestOptions = {}): Promise<UserDeletionStatus> {
		return api.get<UserDeletionStatus>('/api/v1/users/me/deletion-requests/status', opts);
	},
};

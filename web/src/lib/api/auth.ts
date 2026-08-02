import { api, type RequestOptions } from './client';

/** Generic success response from auth endpoints. */
export interface AuthOkResponse {
	ok: boolean;
	message?: string;
}

/**
 * Typed client for the four M16-003 auth endpoints.
 * All endpoints are AllowAnonymous — no token required.
 */
export const authApi = {
	/**
	 * Requests a password-reset email for `email`.
	 * The backend always returns 200 regardless of whether the email exists
	 * (enumeration defense). Pass `opts.fetch` to use a scoped fetch (e.g. from
	 * a SvelteKit server action).
	 */
	requestPasswordReset(email: string, opts?: RequestOptions): Promise<AuthOkResponse> {
		return api.post<AuthOkResponse>('/api/v1/auth/password-reset/request', { email }, opts);
	},

	/**
	 * Confirms a password-reset by submitting `token` (from the email link) and
	 * `newPassword`. Throws `ApiError` with `problemType` ending in
	 * `/password-too-weak` (422) or `/password-reset-token-invalid` (400) on
	 * failure.
	 */
	confirmPasswordReset(
		token: string,
		newPassword: string,
		opts?: RequestOptions
	): Promise<AuthOkResponse> {
		return api.post<AuthOkResponse>(
			'/api/v1/auth/password-reset/confirm',
			{ token, new_password: newPassword },
			opts
		);
	},

	/**
	 * Requests a new email-verification email for `email`. The backend always
	 * returns 200 regardless of whether the email exists (enumeration defense).
	 */
	resendEmailVerification(email: string, opts?: RequestOptions): Promise<AuthOkResponse> {
		return api.post<AuthOkResponse>(
			'/api/v1/auth/email-verification/resend',
			{ email },
			opts
		);
	},

	/**
	 * Confirms an email-verification token. Throws `ApiError` with `problemType`
	 * ending in `/email-verification-token-invalid` (400) on failure.
	 */
	confirmEmailVerification(token: string, opts?: RequestOptions): Promise<AuthOkResponse> {
		return api.post<AuthOkResponse>(
			'/api/v1/auth/email-verification/confirm',
			{ token },
			opts
		);
	}
};

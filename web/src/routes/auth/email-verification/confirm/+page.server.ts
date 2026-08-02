import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { authApi } from '$lib/api/auth';
import { ApiError } from '$lib/types/api-error';

/**
 * Auto-confirms the email-verification token from the URL query string.
 * On success → redirect to / with email_verified toast.
 * On missing/invalid token → return an error for the Svelte page to render.
 */
export const load: PageServerLoad = async ({ url, fetch }) => {
	const token = url.searchParams.get('token') ?? '';

	if (!token) {
		return { error: 'missing_token' };
	}

	try {
		await authApi.confirmEmailVerification(token, { fetch });
	} catch (e) {
		if (e instanceof ApiError && e.problemType?.endsWith('/email-verification-token-invalid')) {
			return { error: 'token_invalid' };
		}
		return { error: 'server_error' };
	}

	throw redirect(303, '/?toast=email_verified');
};

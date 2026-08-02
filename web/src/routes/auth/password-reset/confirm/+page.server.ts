import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { authApi } from '$lib/api/auth';
import { ApiError } from '$lib/types/api-error';

/** Extracts the reset token from the query string and signals whether one is present. */
export const load: PageServerLoad = async ({ url }) => {
	const token = url.searchParams.get('token') ?? '';
	return { token, hasToken: token.length > 0 };
};

export const actions: Actions = {
	/**
	 * Submits a password-reset confirmation.
	 * - 200 → redirects to /login?toast=password_reset_success
	 * - 422 password-too-weak → fail(422) with inline score for the UI
	 * - 400 token-invalid → fail(400) with token_invalid error key
	 * - missing inputs → fail(400) with missing_field
	 */
	default: async ({ request, fetch }) => {
		const data = await request.formData();
		const token = String(data.get('token') ?? '');
		const password = String(data.get('new_password') ?? '');

		if (!token || !password) {
			return fail(400, { error: 'missing_field' });
		}

		try {
			await authApi.confirmPasswordReset(token, password, { fetch });
		} catch (e) {
			if (e instanceof ApiError) {
				if (e.problemType?.endsWith('/password-too-weak')) {
					return fail(422, {
						error: 'weak_password',
						score: Number(e.details?.score ?? 0)
					});
				}
				if (e.problemType?.endsWith('/password-reset-token-invalid')) {
					return fail(400, { error: 'token_invalid' });
				}
			}
			return fail(500, { error: 'server_error' });
		}

		throw redirect(303, '/login?toast=password_reset_success');
	}
};

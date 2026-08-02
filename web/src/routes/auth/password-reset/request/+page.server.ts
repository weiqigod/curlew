import type { Actions, PageServerLoad } from './$types';
import { authApi } from '$lib/api/auth';

/** No load logic needed; the form is rendered unconditionally. */
export const load: PageServerLoad = async () => {
	return {};
};

export const actions: Actions = {
	/**
	 * POSTs the email to the password-reset/request endpoint.
	 * Always returns `{ submitted: true }` regardless of the backend response
	 * to prevent email-address enumeration.
	 */
	default: async ({ request, fetch }) => {
		const data = await request.formData();
		const email = String(data.get('email') ?? '');
		try {
			await authApi.requestPasswordReset(email, { fetch });
		} catch {
			/* intentional: enumeration defense — never expose whether email exists */
		}
		return { submitted: true };
	}
};

import type { PageServerLoad } from './$types';

/**
 * SSR loader for /account/data.
 * The export request list is loaded client-side after the initial POST.
 * This loader only verifies authentication and passes the access token.
 */
export const load: PageServerLoad = async ({ locals }) => {
	return {
		isAuthenticated: !!locals.accessToken,
	};
};

import type { PageServerLoad } from './$types';

/**
 * SSR loader for /account/data/cancel-deletion.
 * Verifies the user is authenticated; the deletion status is loaded client-side.
 */
export const load: PageServerLoad = async ({ locals }) => {
	return {
		isAuthenticated: !!locals.accessToken,
	};
};

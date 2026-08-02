import type { LayoutServerLoad } from './$types';
import { organizationsApi } from '$lib/api/organizations';

/** Loads the current user's organizations for the app sub-navigation. */
export const load: LayoutServerLoad = async ({ locals, fetch }) => {
	if (!locals.accessToken) {
		return { organizations: [] };
	}
	try {
		const organizations = await organizationsApi.list({ token: locals.accessToken, fetch });
		return { organizations };
	} catch {
		return { organizations: [] };
	}
};

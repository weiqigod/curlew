import type { PageServerLoad } from './$types';
import { organizationsApi } from '$lib/api/organizations';

/** Minimal loader for the org overview page. */
export const load: PageServerLoad = async ({ locals, params, fetch }) => {
	if (!locals.accessToken) {
		return { org: null };
	}
	try {
		const org = await organizationsApi.findBySlug(params.slug, {
			token: locals.accessToken,
			fetch
		});
		return { org };
	} catch {
		return { org: null };
	}
};

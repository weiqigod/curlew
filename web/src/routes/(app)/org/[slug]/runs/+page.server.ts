// M14-021: /org/<slug>/runs is an alias for /org/<slug>/results.
// The observable publishes /org/acme/runs; this 307 redirect satisfies it
// without renaming the well-known /results route.
import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageServerLoad = ({ params }) => {
	throw redirect(307, `/org/${params.slug}/results`);
};

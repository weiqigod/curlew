import { error } from '@sveltejs/kit';
import { getMeta, getAllSlugs, getNeighbors } from '$lib/content';

export const prerender = true;

// Enumerate every scenario so adapter-static prerenders /examples/<slug>/.
export function entries() {
	return getAllSlugs().map((slug) => ({ slug }));
}

export function load({ params }: { params: { slug: string } }) {
	const meta = getMeta(params.slug);
	if (!meta) throw error(404, `Unknown example: ${params.slug}`);
	const { prev, next } = getNeighbors(params.slug);
	return { meta, prev, next };
}

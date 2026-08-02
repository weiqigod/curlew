import { FEATURE_IDS, GROUP_IDS } from './taxonomy';

/** Structured metadata for one example scenario (the `.svx` frontmatter). */
export interface ExampleMeta {
	/** URL slug — derived from the filename, not the frontmatter. */
	slug: string;
	/** Short scenario title. */
	title: string;
	/** One-sentence "what this proves" hook, shown on cards and the page header. */
	intent: string;
	/** Group id from taxonomy (core | resilience | security | automation). */
	group: string;
	/** Feature ids from taxonomy — drive badges and filtering. */
	features: string[];
	/** The headline command, shown on cards. */
	command: string;
	/** Sort order within a group. */
	order: number;
	/** True when the captured output came from a real run against a free public endpoint. */
	runnable?: boolean;
}

function fail(slug: string, msg: string): never {
	throw new Error(`Example "${slug}": ${msg}`);
}

/**
 * Validate raw mdsvex frontmatter into a typed ExampleMeta. Throws at build
 * time (during prerender) on any malformed or unknown value — so a typo in a
 * feature id never ships.
 */
export function validateFrontmatter(slug: string, raw: Record<string, unknown>): ExampleMeta {
	const str = (key: string): string => {
		const v = raw[key];
		if (typeof v !== 'string' || v.trim() === '') fail(slug, `missing string field "${key}"`);
		return (v as string).trim();
	};

	const title = str('title');
	const intent = str('intent');
	const command = str('command');

	const group = str('group');
	if (!GROUP_IDS.has(group)) fail(slug, `unknown group "${group}"`);

	const features = raw.features;
	if (!Array.isArray(features) || features.length === 0) {
		fail(slug, 'field "features" must be a non-empty array');
	}
	for (const f of features) {
		if (typeof f !== 'string' || !FEATURE_IDS.has(f)) fail(slug, `unknown feature "${String(f)}"`);
	}

	const order = raw.order;
	if (typeof order !== 'number') fail(slug, 'field "order" must be a number');

	return {
		slug,
		title,
		intent,
		group,
		features: features as string[],
		command,
		order,
		runnable: raw.runnable === true
	};
}

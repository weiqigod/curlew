import type { ComponentType } from 'svelte';
import { validateFrontmatter, type ExampleMeta } from './schema';
import { GROUPS, type GroupInfo } from './taxonomy';

export type { ExampleMeta } from './schema';

interface SvxModule {
	default: ComponentType;
	metadata?: Record<string, unknown>;
}

// Eager glob: every scenario is bundled, so the compiled component is available
// synchronously for prerendering (no client-side fetch, no loading flash).
// Adding a new `.svx` here auto-registers it everywhere — index, route, prerender.
const modules = import.meta.glob<SvxModule>('/src/content/examples/*.svx', { eager: true });

interface Entry {
	meta: ExampleMeta;
	Component: ComponentType;
}

function slugFromPath(path: string): string {
	return path.split('/').pop()!.replace(/\.svx$/, '');
}

const entries: Entry[] = Object.entries(modules)
	.map(([path, mod]) => {
		const slug = slugFromPath(path);
		const meta = validateFrontmatter(slug, mod.metadata ?? {});
		return { meta, Component: mod.default };
	})
	.sort((a, b) => a.meta.order - b.meta.order || a.meta.title.localeCompare(b.meta.title));

const bySlug = new Map(entries.map((e) => [e.meta.slug, e]));

/** All scenario metadata, sorted by order — safe to ship to the client. */
export function getAllExamples(): ExampleMeta[] {
	return entries.map((e) => e.meta);
}

export function getMeta(slug: string): ExampleMeta | undefined {
	return bySlug.get(slug)?.meta;
}

/** The compiled scenario component. Resolved from the eager glob (not via load). */
export function getComponent(slug: string): ComponentType | undefined {
	return bySlug.get(slug)?.Component;
}

export function getAllSlugs(): string[] {
	return entries.map((e) => e.meta.slug);
}

/** Examples grouped by taxonomy group, in group order, for the index page. */
export function getGroupedExamples(): Array<GroupInfo & { examples: ExampleMeta[] }> {
	return GROUPS.map((g) => ({
		...g,
		examples: entries.filter((e) => e.meta.group === g.id).map((e) => e.meta)
	})).filter((g) => g.examples.length > 0);
}

/** Previous / next within the global order — for scenario-page footer nav. */
export function getNeighbors(slug: string): { prev?: ExampleMeta; next?: ExampleMeta } {
	const i = entries.findIndex((e) => e.meta.slug === slug);
	if (i === -1) return {};
	return {
		prev: i > 0 ? entries[i - 1].meta : undefined,
		next: i < entries.length - 1 ? entries[i + 1].meta : undefined
	};
}

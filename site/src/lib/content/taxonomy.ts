// Single source of truth for feature tags and scenario groups.
// Badge classes are written as complete literal strings so Tailwind's JIT
// scanner keeps them (this file is covered by the `content` glob).

export interface FeatureTag {
	id: string;
	label: string;
	blurb: string;
	/** Complete light+dark Tailwind classes for the badge pill. */
	badge: string;
}

export interface GroupInfo {
	id: string;
	label: string;
	blurb: string;
}

export const FEATURES: FeatureTag[] = [
	{
		id: 'auth',
		label: 'Dynamic auth',
		blurb: 'Run a login collection, cache the token, refresh it automatically on 401.',
		badge: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/40 dark:text-cyan-300'
	},
	{
		id: 'chaining',
		label: 'Chaining & extract',
		blurb: 'Pull values out of one response with JSONPath and feed them into the next request.',
		badge: 'bg-sky-100 text-sky-800 dark:bg-sky-900/40 dark:text-sky-300'
	},
	{
		id: 'setup-teardown',
		label: 'Setup / teardown',
		blurb: 'Seed fixtures before the run and clean them up afterwards — even on failure.',
		badge: 'bg-slate-100 text-slate-700 dark:bg-slate-800/70 dark:text-slate-300'
	},
	{
		id: 'parallel',
		label: 'Parallel waves',
		blurb: 'Declare dependencies; Curlew topologically sorts requests into concurrent waves.',
		badge: 'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300'
	},
	{
		id: 'data-driven',
		label: 'Data-driven',
		blurb: 'Run one request once per row of a CSV / JSON / YAML dataset.',
		badge: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/40 dark:text-indigo-300'
	},
	{
		id: 'faker',
		label: 'Dynamic & faker',
		blurb: 'Generate UUIDs, timestamps, and realistic fake data inline — seedable for determinism.',
		badge: 'bg-violet-100 text-violet-800 dark:bg-violet-900/40 dark:text-violet-300'
	},
	{
		id: 'cel',
		label: 'CEL assertions',
		blurb: 'Cross-field invariants and aggregates with Google CEL, type-checked at validate time.',
		badge: 'bg-fuchsia-100 text-fuchsia-800 dark:bg-fuchsia-900/40 dark:text-fuchsia-300'
	},
	{
		id: 'retry',
		label: 'Retry & backoff',
		blurb: 'Exponential / linear / constant backoff, jitter, and Retry-After handling.',
		badge: 'bg-teal-100 text-teal-800 dark:bg-teal-900/40 dark:text-teal-300'
	},
	{
		id: 'rate-limit',
		label: 'Rate limiting',
		blurb: 'A token bucket paces requests across the whole collection or per data-driven run.',
		badge: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300'
	},
	{
		id: 'graphql',
		label: 'GraphQL',
		blurb: 'First-class GraphQL with external queries, fragments, and partial-error handling.',
		badge: 'bg-pink-100 text-pink-800 dark:bg-pink-900/40 dark:text-pink-300'
	},
	{
		id: 'websocket',
		label: 'WebSocket',
		blurb: 'Scripted send / expect / wait / close flows with heartbeats and reconnect.',
		badge: 'bg-rose-100 text-rose-800 dark:bg-rose-900/40 dark:text-rose-300'
	},
	{
		id: 'signing',
		label: 'Request signing',
		blurb: 'Built-in AWS Signature V4 and OAuth 1.0a signers — no plugin required.',
		badge: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300'
	},
	{
		id: 'vault',
		label: 'Vaults & secrets',
		blurb: 'Pull credentials from a vault or shell command; never commit a secret.',
		badge: 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-300'
	},
	{
		id: 'redaction',
		label: 'Redaction',
		blurb: 'Sensitive values are masked in terminal, JSON, and markdown output.',
		badge: 'bg-stone-100 text-stone-700 dark:bg-stone-800/70 dark:text-stone-300'
	},
	{
		id: 'ci',
		label: 'CI / CD',
		blurb: 'JUnit / TAP output and a clean exit-code contract for any CI system.',
		badge: 'bg-lime-100 text-lime-800 dark:bg-lime-900/40 dark:text-lime-300'
	},
	{
		id: 'reporting',
		label: 'Reporting & PR checks',
		blurb: 'Write local reports and derive a pass/fail verdict for your CI system.',
		badge: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300'
	},
	{
		id: 'perf',
		label: 'Load & performance',
		blurb: 'Drive sustained load and capture latency percentiles.',
		badge: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300'
	},
	{
		id: 'openapi',
		label: 'OpenAPI import',
		blurb: 'Turn an OpenAPI 3.x spec into a starting-point collection.',
		badge: 'bg-neutral-100 text-neutral-700 dark:bg-neutral-800/70 dark:text-neutral-300'
	}
];

export const GROUPS: GroupInfo[] = [
	{
		id: 'core',
		label: 'Core authoring at scale',
		blurb: 'Realistic flows that chain, parameterize, and parallelize real request graphs.'
	},
	{
		id: 'resilience',
		label: 'Resilience & beyond REST',
		blurb: 'Survive flaky networks; test GraphQL and WebSocket APIs as first-class citizens.'
	},
	{
		id: 'security',
		label: 'Security & secrets',
		blurb: 'Signed requests, vault-backed credentials, and a redaction contract you can trust.'
	},
	{
		id: 'automation',
		label: 'CI and local automation',
		blurb: 'Write reports, inspect results, and run bounded local load tests.'
	}
];

const FEATURE_MAP = new Map(FEATURES.map((f) => [f.id, f]));
const GROUP_MAP = new Map(GROUPS.map((g) => [g.id, g]));

export function feature(id: string): FeatureTag {
	return (
		FEATURE_MAP.get(id) ?? {
			id,
			label: id,
			blurb: '',
			badge: 'bg-slate-100 text-slate-700 dark:bg-slate-800/70 dark:text-slate-300'
		}
	);
}

export function group(id: string): GroupInfo {
	return GROUP_MAP.get(id) ?? { id, label: id, blurb: '' };
}

export const FEATURE_IDS = new Set(FEATURE_MAP.keys());
export const GROUP_IDS = new Set(GROUP_MAP.keys());

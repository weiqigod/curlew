<script lang="ts">
	import type { ExampleMeta } from '$lib/content/schema';
	import { codeCopy } from '$lib/actions/codeCopy';
	import FeatureBadge from './FeatureBadge.svelte';

	export let meta: ExampleMeta;
	export let prev: ExampleMeta | undefined = undefined;
	export let next: ExampleMeta | undefined = undefined;
</script>

<article class="mx-auto max-w-3xl px-5 py-10">
	<a
		href="/examples"
		class="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--text-soft)] transition hover:text-brand-500"
	>
		<span aria-hidden="true">←</span> All examples
	</a>

	<header class="mt-5 border-b border-[color:var(--border)] pb-7">
		<div class="flex flex-wrap items-center gap-2">
			{#if meta.runnable}
				<span
					class="inline-flex items-center gap-1 text-[0.65rem] font-semibold uppercase tracking-wide text-emerald-600 dark:text-emerald-400"
				>
					<span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span> Local example
				</span>
			{/if}
		</div>

		<h1 class="mt-3 text-3xl font-bold tracking-tight text-[var(--text)]">{meta.title}</h1>
		<p class="mt-2 text-lg leading-relaxed text-[var(--text-soft)]">{meta.intent}</p>

		<div class="mt-4 flex flex-wrap gap-1.5">
			{#each meta.features as f (f)}
				<FeatureBadge id={f} link />
			{/each}
		</div>

		<div
			class="mt-5 overflow-x-auto rounded-lg border border-[color:var(--border)] bg-[var(--code-bg)] px-3.5 py-2.5 font-mono text-sm text-[var(--text)]"
		>
			<span class="select-none text-[var(--text-faint)]">$ </span>{meta.command}
		</div>
	</header>

	<div
		class="prose prose-slate mt-8 max-w-none dark:prose-invert prose-headings:scroll-mt-24 prose-headings:text-[var(--text)] prose-a:text-brand-600 prose-a:no-underline hover:prose-a:underline dark:prose-a:text-brand-400 prose-strong:text-[var(--text)]"
		use:codeCopy
	>
		<slot />
	</div>

	<nav
		class="mt-12 grid gap-3 border-t border-[color:var(--border)] pt-7 sm:grid-cols-2"
		aria-label="More examples"
	>
		{#if prev}
			<a
				href={`/examples/${prev.slug}`}
				class="group rounded-xl border border-[color:var(--border)] p-4 transition hover:border-brand-500/60"
			>
				<div class="text-xs uppercase tracking-wide text-[var(--text-faint)]">← Previous</div>
				<div class="mt-1 font-medium text-[var(--text)] group-hover:text-brand-500">
					{prev.title}
				</div>
			</a>
		{:else}
			<span></span>
		{/if}
		{#if next}
			<a
				href={`/examples/${next.slug}`}
				class="group rounded-xl border border-[color:var(--border)] p-4 text-right transition hover:border-brand-500/60"
			>
				<div class="text-xs uppercase tracking-wide text-[var(--text-faint)]">Next →</div>
				<div class="mt-1 font-medium text-[var(--text)] group-hover:text-brand-500">
					{next.title}
				</div>
			</a>
		{/if}
	</nav>
</article>

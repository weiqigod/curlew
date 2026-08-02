<script lang="ts">
	import type { ExampleMeta } from '$lib/content/schema';
	import FeatureBadge from './FeatureBadge.svelte';

	export let meta: ExampleMeta;
</script>

<a
	href={`/examples/${meta.slug}`}
	class="group flex h-full flex-col gap-3 rounded-xl border border-[color:var(--border)] bg-[var(--bg-elevated)] p-5 transition hover:-translate-y-0.5 hover:border-brand-500/60 hover:shadow-lg hover:shadow-brand-500/5"
>
	<div class="flex items-center justify-end gap-2">
		{#if meta.runnable}
			<span
				class="inline-flex items-center gap-1 text-[0.65rem] font-semibold uppercase tracking-wide text-emerald-600 dark:text-emerald-400"
			>
				<span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span>
				Live output
			</span>
		{/if}
	</div>

	<div>
		<h3 class="text-base font-semibold leading-snug text-[var(--text)] group-hover:text-brand-500">
			{meta.title}
		</h3>
		<p class="mt-1.5 text-sm leading-relaxed text-[var(--text-soft)]">{meta.intent}</p>
	</div>

	<div class="mt-auto flex flex-wrap gap-1.5 pt-1">
		{#each meta.features.slice(0, 4) as f (f)}
			<FeatureBadge id={f} />
		{/each}
		{#if meta.features.length > 4}
			<span class="text-xs font-medium text-[var(--text-faint)]"
				>+{meta.features.length - 4}</span
			>
		{/if}
	</div>

	<code
		class="truncate rounded-md bg-[var(--code-bg)] px-2.5 py-1.5 font-mono text-xs text-[var(--text-soft)]"
		title={meta.command}
	>
		<span class="text-[var(--text-faint)]">$</span>
		{meta.command}
	</code>
</a>

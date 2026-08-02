<script lang="ts">
	import { FEATURES } from '$lib/content/taxonomy';

	export let availableFeatures: string[] = [];
	export let selectedFeatures: string[] = [];
	export let query = '';
	export let resultCount = 0;
	export let totalCount = 0;

	function toggleFeature(id: string) {
		selectedFeatures = selectedFeatures.includes(id)
			? selectedFeatures.filter((f) => f !== id)
			: [...selectedFeatures, id];
	}

	function clearAll() {
		selectedFeatures = [];
		query = '';
	}

	$: featureList = FEATURES.filter((f) => availableFeatures.includes(f.id));
	$: anyActive = selectedFeatures.length > 0 || query.trim() !== '';
</script>

<div class="flex flex-col gap-4 rounded-xl border border-[color:var(--border)] bg-[var(--bg-soft)] p-4">
	<div class="flex flex-wrap items-center gap-3">
		<div class="relative flex-1 min-w-[12rem]">
			<svg
				class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-faint)]"
				width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"
				stroke-width="2" stroke-linecap="round" aria-hidden="true"
			>
				<circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" />
			</svg>
			<input
				type="search"
				bind:value={query}
				placeholder="Search examples…"
				aria-label="Search examples"
				class="w-full rounded-lg border border-[color:var(--border)] bg-[var(--bg)] py-2 pl-9 pr-3 text-sm text-[var(--text)] outline-none focus:border-brand-500"
			/>
		</div>
		<span class="text-sm text-[var(--text-faint)]">
			{resultCount} of {totalCount}
		</span>
		{#if anyActive}
			<button
				type="button"
				on:click={clearAll}
				class="rounded-lg px-2.5 py-1 text-xs font-medium text-[var(--text-soft)] underline-offset-2 hover:text-[var(--text)] hover:underline"
			>
				Clear filters
			</button>
		{/if}
	</div>

	<div class="flex flex-wrap gap-1.5">
		{#each featureList as f (f.id)}
			<button
				type="button"
				on:click={() => toggleFeature(f.id)}
				aria-pressed={selectedFeatures.includes(f.id)}
				title={f.blurb}
				class="rounded-full px-2.5 py-0.5 text-xs font-medium transition {selectedFeatures.includes(
					f.id
				)
					? f.badge
					: 'bg-transparent text-[var(--text-soft)] ring-1 ring-inset ring-[color:var(--border)] hover:text-[var(--text)]'}"
			>
				{f.label}
			</button>
		{/each}
	</div>
</div>

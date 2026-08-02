<script lang="ts">
	import { onMount } from 'svelte';
	import type { PageData } from './$types';
	import type { ExampleMeta } from '$lib/content/schema';
	import ExampleCard from '$lib/components/ExampleCard.svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';

	export let data: PageData;

	let selectedFeatures: string[] = [];
	let query = '';

	const availableFeatures = Array.from(
		new Set(data.examples.flatMap((e) => e.features))
	);

	function matches(e: ExampleMeta): boolean {
		if (selectedFeatures.length && !selectedFeatures.every((f) => e.features.includes(f))) {
			return false;
		}
		const q = query.trim().toLowerCase();
		if (q) {
			const hay = `${e.title} ${e.intent} ${e.command} ${e.features.join(' ')}`.toLowerCase();
			if (!hay.includes(q)) return false;
		}
		return true;
	}

	$: filtered = data.examples.filter(matches);
	$: filtering = selectedFeatures.length > 0 || query.trim() !== '';

	onMount(() => {
		// Support shareable, badge-driven filters like /examples#feature=graphql
		const m = location.hash.match(/feature=([\w-]+)/);
		if (m && availableFeatures.includes(m[1])) {
			selectedFeatures = [m[1]];
		}
	});
</script>

<svelte:head>
	<title>Examples — ApiTool cookbook</title>
	<meta
		name="description"
		content="A filterable gallery of sophisticated, end-to-end ApiTool test scenarios."
	/>
</svelte:head>

<div class="mx-auto max-w-6xl px-5 py-12">
	<header class="max-w-2xl">
		<h1 class="text-3xl font-bold tracking-tight text-[var(--text)]">The cookbook</h1>
		<p class="mt-3 text-lg leading-relaxed text-[var(--text-soft)]">
			{data.examples.length} end-to-end scenarios. Each one combines several features the way you'd really
			use them — annotated YAML, the exact command, and the output it produces.
		</p>
	</header>

	<div class="mt-8">
		<FilterBar
			{availableFeatures}
			bind:selectedFeatures
			bind:query
			resultCount={filtered.length}
			totalCount={data.examples.length}
		/>
	</div>

	{#if filtering}
		{#if filtered.length}
			<div class="mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
				{#each filtered as meta (meta.slug)}
					<ExampleCard {meta} />
				{/each}
			</div>
		{:else}
			<p class="mt-16 text-center text-[var(--text-soft)]">
				No examples match those filters yet.
			</p>
		{/if}
	{:else}
		{#each data.groups as g (g.id)}
			<section class="mt-12">
				<h2 class="text-xl font-bold tracking-tight text-[var(--text)]">{g.label}</h2>
				<p class="mt-1 text-[var(--text-soft)]">{g.blurb}</p>
				<div class="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
					{#each g.examples as meta (meta.slug)}
						<ExampleCard {meta} />
					{/each}
				</div>
			</section>
		{/each}
	{/if}
</div>

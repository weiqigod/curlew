<script lang="ts">
	import type { PageData } from './$types';
	import { FEATURES } from '$lib/content/taxonomy';
	import FeatureBadge from '$lib/components/FeatureBadge.svelte';

	export let data: PageData;

	$: byFeature = FEATURES.map((f) => ({
		...f,
		examples: data.examples.filter((e) => e.features.includes(f.id))
	}));
</script>

<svelte:head>
	<title>Features — ApiTool cookbook</title>
	<meta
		name="description"
		content="Every ApiTool capability and the example scenarios that demonstrate it."
	/>
</svelte:head>

<div class="mx-auto max-w-5xl px-5 py-12">
	<header class="max-w-2xl">
		<h1 class="text-3xl font-bold tracking-tight text-[var(--text)]">Features</h1>
		<p class="mt-3 text-lg leading-relaxed text-[var(--text-soft)]">
			The whole surface of the tool — from a first <code>GET</code> to distributed runs and
			compliance tooling. Each capability links to the scenarios that put it to work.
		</p>
	</header>

	<!-- Capability cross-index -->
	<section class="mt-10">
		<h2 class="text-sm font-semibold uppercase tracking-wider text-[var(--text-faint)]">
			Capabilities
		</h2>
		<div class="mt-4 divide-y divide-[color:var(--border)] overflow-hidden rounded-xl border border-[color:var(--border)]">
			{#each byFeature as f (f.id)}
				<div class="flex flex-col gap-3 p-5 sm:flex-row sm:items-start sm:gap-6">
					<div class="sm:w-64 sm:shrink-0">
						<FeatureBadge id={f.id} link />
						<p class="mt-2 text-sm leading-snug text-[var(--text-soft)]">{f.blurb}</p>
					</div>
					<div class="flex flex-1 flex-wrap items-center gap-2">
						{#if f.examples.length}
							{#each f.examples as e (e.slug)}
								<a
									href={`/examples/${e.slug}`}
									class="rounded-md border border-[color:var(--border)] px-2.5 py-1 text-xs font-medium text-[var(--text-soft)] transition hover:border-brand-500/60 hover:text-brand-500"
								>
									{e.title}
								</a>
							{/each}
						{:else}
							<span class="text-xs italic text-[var(--text-faint)]">Documented in the manual</span>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	</section>
</div>

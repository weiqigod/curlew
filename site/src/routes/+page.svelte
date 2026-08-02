<script lang="ts">
	import type { PageData } from './$types';
	import { codeCopy } from '$lib/actions/codeCopy';
	import ExampleCard from '$lib/components/ExampleCard.svelte';

	export let data: PageData;

	const highlights = [
		{
			title: 'Chain real request graphs',
			body: 'Extract a token here, feed it into the next call there. Declare dependencies and Curlew runs independent requests in parallel waves.'
		},
		{
			title: 'Data-driven by default',
			body: 'Run one request once per row of a CSV, JSON, or YAML dataset — filtered, rate-limited, and aggregated.'
		},
		{
			title: 'Assertions that scale',
			body: 'Thirteen body operators, JSONPath, and a Google-CEL escape hatch for cross-field invariants — all type-checked before a single request fires.'
		},
		{
			title: 'Beyond REST',
			body: 'First-class GraphQL (with fragments) and scripted WebSocket flows, sharing the same request envelope.'
		},
		{
			title: 'Secrets done right',
			body: 'Pull credentials from a vault or shell command, sign requests with SigV4 / OAuth1, and trust a redaction contract that masks them everywhere.'
		},
		{
			title: 'Built for the pipeline',
			body: 'JUnit / TAP output, a precise exit-code contract, dashboard upload, PR checks, distributed workers, and compliance tooling.'
		}
	];
</script>

<svelte:head>
	<title>Curlew — the cookbook</title>
	<meta
		name="description"
		content="Sophisticated, end-to-end examples that show the full power of Curlew — the file-based API testing CLI."
	/>
</svelte:head>

<!-- Hero -->
<section class="relative overflow-hidden border-b border-[color:var(--border)]">
	<div
		class="pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(60%_50%_at_70%_0%,rgba(6,182,212,0.12),transparent)]"
	></div>
	<div class="mx-auto grid max-w-6xl items-center gap-10 px-5 py-16 lg:grid-cols-2 lg:py-24">
		<div>
			<p class="text-sm font-semibold uppercase tracking-wider text-brand-500">
				The Curlew cookbook
			</p>
			<h1 class="mt-3 text-4xl font-bold leading-tight tracking-tight text-[var(--text)] sm:text-5xl">
				The full power of your API test suite — in YAML you can read and diff.
			</h1>
			<p class="mt-5 max-w-xl text-lg leading-relaxed text-[var(--text-soft)]">
				Curlew is a file-based API testing tool — a Postman replacement for developers who live in
				files, version control, and the CLI. This is the cookbook: real, end-to-end scenarios that
				go far past the basics.
			</p>
			<div class="mt-8 flex flex-wrap gap-3">
				<a
					href="/examples"
					class="rounded-lg bg-brand-600 px-5 py-2.5 text-sm font-semibold text-white transition hover:bg-brand-500"
				>
					Browse the examples →
				</a>
				<a
					href="/features"
					class="rounded-lg border border-[color:var(--border)] px-5 py-2.5 text-sm font-semibold text-[var(--text)] transition hover:border-brand-500/60"
				>
					See every feature
				</a>
			</div>
			<div class="mt-8 text-sm" use:codeCopy>
				<!-- Build-time Shiki output from a static constant — safe. -->
				<!-- eslint-disable-next-line svelte/no-at-html-tags -->
				{@html data.installHtml}
			</div>
		</div>

		<div class="text-sm" use:codeCopy>
			<div class="mb-2 flex items-center gap-2 text-xs font-medium text-[var(--text-faint)]">
				<span class="h-2.5 w-2.5 rounded-full bg-red-400/70"></span>
				<span class="h-2.5 w-2.5 rounded-full bg-amber-400/70"></span>
				<span class="h-2.5 w-2.5 rounded-full bg-emerald-400/70"></span>
				<span class="ml-2">collections/checkout.yaml</span>
			</div>
			<!-- Build-time Shiki output from a static constant — safe. -->
			<!-- eslint-disable-next-line svelte/no-at-html-tags -->
			{@html data.heroHtml}
		</div>
	</div>
</section>

<!-- Highlights -->
<section class="mx-auto max-w-6xl px-5 py-16">
	<h2 class="text-2xl font-bold tracking-tight text-[var(--text)]">Why this isn't just curl in a loop</h2>
	<p class="mt-2 max-w-2xl text-[var(--text-soft)]">
		Every capability below is demonstrated end-to-end in the examples — combined the way you'd
		actually use them, not one feature at a time.
	</p>
	<div class="mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
		{#each highlights as h (h.title)}
			<div class="rounded-xl border border-[color:var(--border)] bg-[var(--bg-elevated)] p-5">
				<h3 class="font-semibold text-[var(--text)]">{h.title}</h3>
				<p class="mt-2 text-sm leading-relaxed text-[var(--text-soft)]">{h.body}</p>
			</div>
		{/each}
	</div>
</section>

<!-- Featured scenarios -->
<section class="mx-auto max-w-6xl px-5 pb-8">
	<div class="flex items-end justify-between gap-4">
		<div>
			<h2 class="text-2xl font-bold tracking-tight text-[var(--text)]">Featured scenarios</h2>
			<p class="mt-2 text-[var(--text-soft)]">A taste of the {data.examples.length}-strong cookbook.</p>
		</div>
		<a
			href="/examples"
			class="shrink-0 text-sm font-semibold text-brand-500 hover:underline">All examples →</a
		>
	</div>
	<div class="mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
		{#each data.featured as meta (meta.slug)}
			<ExampleCard {meta} />
		{/each}
	</div>
</section>

<!-- CTA -->
<section class="mx-auto max-w-6xl px-5 py-16">
	<div
		class="rounded-2xl border border-[color:var(--border)] bg-[var(--bg-soft)] px-8 py-12 text-center"
	>
		<h2 class="text-2xl font-bold tracking-tight text-[var(--text)]">
			Ready to see how deep it goes?
		</h2>
		<p class="mx-auto mt-2 max-w-xl text-[var(--text-soft)]">
			The examples are copy-runnable. Several run against free public endpoints, so you can paste and
			go.
		</p>
		<a
			href="/examples"
			class="mt-6 inline-block rounded-lg bg-brand-600 px-6 py-2.5 text-sm font-semibold text-white transition hover:bg-brand-500"
		>
			Open the cookbook →
		</a>
	</div>
</section>

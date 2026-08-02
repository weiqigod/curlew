<script lang="ts">
	import type { PageData } from './$types';
	import SummaryCards from '$lib/components/results/SummaryCards.svelte';
	import TrendChart from '$lib/components/results/TrendChart.svelte';
	import RecentRunsTable from '$lib/components/results/RecentRunsTable.svelte';
	import TimeRangePicker from '$lib/components/results/TimeRangePicker.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';
	import { page } from '$app/stores';

	export let data: PageData;
</script>

<svelte:head>
	<title>{data.org.name} — Test Results</title>
</svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Test Results</h1>
		<TimeRangePicker current={data.range} slug={data.org.slug} />
	</div>

	{#if data.error}
		<ErrorState
			message={data.error}
			retryUrl={$page.url.pathname + $page.url.search}
		/>
	{:else if data.summary.total_runs === 0}
		<EmptyState message="Run curlew and upload results to get started" />
	{:else}
		<div class="space-y-6">
			<SummaryCards summary={data.summary} />
			<div>
				<h2 class="mb-2 text-lg font-semibold text-gray-700">Trend</h2>
				<TrendChart trend={data.trend} />
			</div>
			<div>
				<h2 class="mb-2 text-lg font-semibold text-gray-700">Recent Runs</h2>
				<RecentRunsTable results={data.results} />
			</div>
		</div>
	{/if}
</div>

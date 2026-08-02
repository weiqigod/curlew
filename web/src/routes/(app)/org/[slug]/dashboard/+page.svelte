<script lang="ts">
	import type { PageData } from './$types';
	import OverviewCards from '$lib/components/dashboard/OverviewCards.svelte';
	import PassRateTrendChart from '$lib/components/dashboard/PassRateTrendChart.svelte';
	import FailingEndpointsList from '$lib/components/dashboard/FailingEndpointsList.svelte';
	import DashboardWindowPicker from '$lib/components/dashboard/DashboardWindowPicker.svelte';
	import TierGatePrompt from '$lib/components/dashboard/TierGatePrompt.svelte';
	import RecentRunsTable from '$lib/components/results/RecentRunsTable.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';
	import { page } from '$app/stores';
	import type { DashboardWindow } from '$lib/types/dashboard';

	export let data: PageData;

	$: currentWindow = data.window as DashboardWindow;
</script>

<svelte:head><title>{data.org.name} — Dashboard</title></svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Dashboard</h1>
		{#if !data.tierGate && !data.windowError && !data.loadError}
			<DashboardWindowPicker current={currentWindow} slug={data.org.slug} />
		{/if}
	</div>

	{#if data.tierGate}
		<TierGatePrompt slug={data.org.slug} />
	{:else if data.windowError}
		<ErrorState message={data.windowError} retryUrl="/org/{data.org.slug}/dashboard" />
	{:else if data.loadError}
		<ErrorState
			message={data.loadError}
			retryUrl={$page.url.pathname + $page.url.search}
		/>
	{:else if data.stats && data.stats.totals.runs === 0}
		<EmptyState message="No runs in this window" />
	{:else if data.stats}
		<div class="space-y-6">
			<OverviewCards totals={data.stats.totals} />
			<div>
				<h2 class="mb-2 text-lg font-semibold text-gray-700">Pass-rate trend</h2>
				<PassRateTrendChart trend={data.stats.trend} />
			</div>
			<div>
				<h2 class="mb-2 text-lg font-semibold text-gray-700">Frequently failing endpoints</h2>
				<FailingEndpointsList items={data.failures?.items ?? []} />
			</div>
			<div>
				<h2 class="mb-2 text-lg font-semibold text-gray-700">Recent runs</h2>
				<RecentRunsTable results={data.recentRuns} />
			</div>
		</div>
	{/if}
</div>

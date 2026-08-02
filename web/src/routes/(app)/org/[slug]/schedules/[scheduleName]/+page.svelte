<script lang="ts">
	import type { PageData } from './$types';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { computeDuration } from '$lib/schedules/duration';
	import { page } from '$app/stores';

	export let data: PageData;

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'completed': return 'bg-green-100 text-green-800';
			case 'failed':    return 'bg-red-100 text-red-800';
			case 'running':   return 'bg-blue-100 text-blue-800';
			case 'queued':    return 'bg-yellow-100 text-yellow-800';
			default:          return 'bg-gray-100 text-gray-600';
		}
	}
</script>

<svelte:head>
	<title>{data.org.name} — Schedule: {$page.params.scheduleName}</title>
</svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8">
	<div class="mb-6">
		<a
			href="/org/{data.org.slug}/schedules"
			class="text-sm text-indigo-600 hover:underline"
		>
			← All schedules
		</a>
		<h1 class="mt-2 text-2xl font-bold text-gray-900">
			{$page.params.scheduleName}
		</h1>
		{#if data.schedule}
			<p class="mt-1 text-sm text-gray-500">
				<span class="font-mono">{data.schedule.cron_expression}</span>
				· {data.schedule.timezone}
			</p>
		{/if}
	</div>

	{#if data.error}
		<ErrorState message={data.error} retryUrl={$page.url.pathname} />
	{:else if data.runs.length === 0}
		<EmptyState message="No runs yet — click 'Run now' on the schedules list to enqueue one." />
	{:else}
		<div class="overflow-x-auto rounded-lg border border-gray-200">
			<table class="min-w-full divide-y divide-gray-200 text-sm">
				<thead class="bg-gray-50">
					<tr>
						<th class="px-4 py-3 text-left font-medium text-gray-600">Run ID</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600">Status</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600">Duration</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600">Queued at</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600">Result</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-100 bg-white">
					{#each data.runs as run}
						<tr class="hover:bg-gray-50">
							<td class="px-4 py-3 font-mono text-xs text-gray-600">{run.run_id}</td>
							<td class="px-4 py-3">
								<span
									class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium {statusBadgeClass(run.status)}"
								>
									{run.status}
								</span>
							</td>
							<td class="px-4 py-3 text-gray-600">
								{computeDuration(run.started_at, run.completed_at)}
							</td>
							<td class="px-4 py-3 text-gray-600">
								{new Date(run.created_at).toLocaleString()}
							</td>
							<td class="px-4 py-3">
								{#if run.result_id}
									<a
										href="/org/{data.org.slug}/results?run_id={encodeURIComponent(run.result_id)}"
										class="text-indigo-600 hover:underline"
									>
										View result
									</a>
								{:else}
									<span class="text-gray-400">—</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

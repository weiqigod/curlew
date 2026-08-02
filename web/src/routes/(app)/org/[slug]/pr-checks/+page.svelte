<script lang="ts">
	import type { PageData } from './$types';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';
	import { page } from '$app/stores';

	export let data: PageData;

	function formatDate(isoDate: string): string {
		return new Date(isoDate).toLocaleString();
	}
</script>

<svelte:head>
	<title>{data.org.name} — PR Checks</title>
</svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">PR Checks</h1>
	</div>

	{#if data.error}
		<ErrorState
			message={data.error}
			retryUrl={$page.url.pathname + $page.url.search}
		/>
	{:else if data.checks.length === 0}
		<EmptyState message="No PR checks yet. Run apitest with --report-upload --pr to post a check." />
	{:else}
		<div class="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm">
			<table class="min-w-full divide-y divide-gray-200" data-testid="pr-checks-table">
				<thead class="bg-gray-50">
					<tr>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Repo</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">PR</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">State</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Result</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Created</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-100 bg-white">
					{#each data.checks as check (check.id)}
						<tr data-testid="pr-check-row" class="hover:bg-gray-50">
							<td class="px-4 py-3 text-sm text-gray-900">{check.repo}</td>
							<td class="px-4 py-3 text-sm text-gray-900">#{check.pr}</td>
							<td class="px-4 py-3 text-sm">
								{#if check.state === 'success'}
									<span
										data-testid="pr-check-state-success"
										class="inline-flex items-center rounded-full bg-green-100 px-2.5 py-0.5 text-xs font-medium text-green-800"
									>
										success
									</span>
								{:else}
									<span
										data-testid="pr-check-state-failure"
										class="inline-flex items-center rounded-full bg-red-100 px-2.5 py-0.5 text-xs font-medium text-red-800"
									>
										failure
									</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-sm text-gray-500">
								{#if check.result_id}
									<a
										href="/results/{check.result_id}"
										class="font-mono text-blue-600 hover:underline"
									>
										{check.result_id.slice(0, 12)}…
									</a>
								{:else}
									—
								{/if}
							</td>
							<td class="px-4 py-3 text-sm text-gray-500">{formatDate(check.created_at)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

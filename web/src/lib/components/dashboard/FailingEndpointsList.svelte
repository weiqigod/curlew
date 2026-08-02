<script lang="ts">
	import type { FailureGroup } from '$lib/types/dashboard';
	import { formatRelativeTime } from '$lib/dashboard/format';

	/** Top-N failure groups from GET /results/failures. */
	export let items: FailureGroup[];
</script>

<div data-testid="dashboard-failing-endpoints">
	{#if items.length === 0}
		<p class="text-sm text-gray-500">No failures in this window.</p>
	{:else}
		<table class="min-w-full divide-y divide-gray-200">
			<thead class="bg-gray-50">
				<tr>
					<th class="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">Method</th>
					<th class="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">Path</th>
					<th class="px-4 py-2 text-right text-xs font-medium uppercase text-gray-500">Failures</th>
					<th class="px-4 py-2 text-right text-xs font-medium uppercase text-gray-500"
						>Last seen</th
					>
				</tr>
			</thead>
			<tbody class="divide-y divide-gray-200 bg-white">
				{#each items as item}
					<tr data-testid="failing-endpoint-row">
						<td
							class="px-4 py-2 text-sm font-mono text-gray-700"
							data-testid="failing-endpoint-method">{item.method}</td
						>
						<td
							class="px-4 py-2 text-sm font-mono text-gray-900"
							data-testid="failing-endpoint-path">{item.path_template}</td
						>
						<td
							class="px-4 py-2 text-right text-sm font-medium text-red-600"
							data-testid="failing-endpoint-count">{item.failure_count}</td
						>
						<td
							class="px-4 py-2 text-right text-sm text-gray-500"
							data-testid="failing-endpoint-last-seen">{formatRelativeTime(item.last_seen_at)}</td
						>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>

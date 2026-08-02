<script lang="ts">
	import type { Result } from '$lib/types/results';

	export let results: Result[];

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function formatDuration(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}
</script>

<div class="overflow-x-auto" data-testid="recent-runs-table">
	<table class="min-w-full divide-y divide-gray-200">
		<thead class="bg-gray-50">
			<tr>
				<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
					Date
				</th>
				<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
					File
				</th>
				<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
					Triggered By
				</th>
				<th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">
					Pass
				</th>
				<th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">
					Duration
				</th>
			</tr>
		</thead>
		<tbody class="divide-y divide-gray-200 bg-white">
			{#each results as result}
				<tr>
					<td class="whitespace-nowrap px-4 py-3 text-sm text-gray-900">
						{formatDate(result.run_at)}
					</td>
					<td class="px-4 py-3 text-sm font-medium text-gray-900">
						{result.collection_name}
					</td>
					<td class="px-4 py-3 text-sm text-gray-500">
						{result.triggered_by ?? '—'}
					</td>
					<td class="whitespace-nowrap px-4 py-3 text-right text-sm">
						<span class="font-medium text-green-600">{result.pass_count}</span>
						{#if result.fail_count > 0}
							/ <span class="font-medium text-red-600">{result.fail_count}</span>
						{/if}
					</td>
					<td class="whitespace-nowrap px-4 py-3 text-right text-sm text-gray-500">
						{formatDuration(result.duration_ms)}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

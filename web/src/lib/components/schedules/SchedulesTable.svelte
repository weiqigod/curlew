<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { Schedule, ScheduledRun } from '$lib/types/schedules';

	export let schedules: Schedule[];
	export let lastRuns: Array<ScheduledRun | null>;
	export let slug: string;

	const dispatch = createEventDispatcher<{ runNow: string }>();

	function statusBadgeClass(status: string | undefined): string {
		switch (status) {
			case 'completed': return 'bg-green-100 text-green-800';
			case 'failed':    return 'bg-red-100 text-red-800';
			case 'running':   return 'bg-blue-100 text-blue-800';
			case 'queued':    return 'bg-yellow-100 text-yellow-800';
			default:          return 'bg-gray-100 text-gray-600';
		}
	}

	function formatDatetime(iso: string | null): string {
		if (!iso) return '—';
		return new Date(iso).toLocaleString();
	}
</script>

<div class="overflow-x-auto rounded-lg border border-gray-200">
	<table class="min-w-full divide-y divide-gray-200 text-sm">
		<thead class="bg-gray-50">
			<tr>
				<th class="px-4 py-3 text-left font-medium text-gray-600">Name</th>
				<th class="px-4 py-3 text-left font-medium text-gray-600">Cron</th>
				<th class="px-4 py-3 text-left font-medium text-gray-600">Timezone</th>
				<th class="px-4 py-3 text-left font-medium text-gray-600">Next run</th>
				<th class="px-4 py-3 text-left font-medium text-gray-600">Last run</th>
				<th class="px-4 py-3 text-left font-medium text-gray-600">Status</th>
				<th class="px-4 py-3 text-right font-medium text-gray-600">Actions</th>
			</tr>
		</thead>
		<tbody class="divide-y divide-gray-100 bg-white">
			{#each schedules as schedule, i}
				{@const lastRun = lastRuns[i] ?? null}
				<tr class="hover:bg-gray-50">
					<td class="px-4 py-3">
						<a
							href="/org/{slug}/schedules/{encodeURIComponent(schedule.name)}"
							class="font-medium text-indigo-600 hover:underline"
							data-testid="schedule-name-link"
						>
							{schedule.name}
						</a>
					</td>
					<td class="px-4 py-3 font-mono text-xs">{schedule.cron_expression}</td>
					<td class="px-4 py-3">{schedule.timezone}</td>
					<td class="px-4 py-3 text-gray-600">{formatDatetime(schedule.next_run_at)}</td>
					<td class="px-4 py-3 text-gray-600">{formatDatetime(schedule.last_run_at)}</td>
					<td class="px-4 py-3">
						{#if lastRun}
							<span
								class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium {statusBadgeClass(lastRun.status)}"
								data-testid="last-run-status"
							>
								{lastRun.status}
							</span>
						{:else}
							<span class="text-gray-400">—</span>
						{/if}
					</td>
					<td class="px-4 py-3 text-right">
						<button
							type="button"
							on:click={() => dispatch('runNow', schedule.name)}
							class="rounded bg-indigo-600 px-3 py-1 text-xs text-white hover:bg-indigo-700"
							data-testid="run-now-btn"
						>
							Run now
						</button>
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

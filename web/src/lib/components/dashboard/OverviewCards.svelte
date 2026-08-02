<script lang="ts">
	import type { StatsTotals } from '$lib/types/dashboard';
	import { formatDuration, formatPassRate } from '$lib/dashboard/format';

	/** Aggregated totals from GET /results/stats. */
	export let totals: StatsTotals;
</script>

<div class="grid grid-cols-2 gap-4 md:grid-cols-5">
	<div class="rounded-lg border bg-white p-4 shadow-sm">
		<p class="text-sm font-medium text-gray-500">Total Runs</p>
		<p class="mt-1 text-2xl font-semibold" data-testid="dashboard-total-runs">{totals.runs}</p>
	</div>
	<div class="rounded-lg border bg-white p-4 shadow-sm">
		<p class="text-sm font-medium text-gray-500">Pass Rate</p>
		<p class="mt-1 text-2xl font-semibold" data-testid="dashboard-pass-rate">
			{formatPassRate(totals.pass_rate)}
		</p>
	</div>
	<div class="rounded-lg border bg-white p-4 shadow-sm">
		<p class="text-sm font-medium text-gray-500">Avg Duration</p>
		<p class="mt-1 text-2xl font-semibold" data-testid="dashboard-avg-duration">
			{formatDuration(totals.avg_duration_ms)}
		</p>
	</div>
	<div class="rounded-lg border bg-white p-4 shadow-sm">
		<p class="text-sm font-medium text-gray-500">p50 Duration</p>
		<p class="mt-1 text-2xl font-semibold" data-testid="dashboard-p50-duration">
			{formatDuration(totals.p50_duration_ms)}
		</p>
	</div>
	<div class="rounded-lg border bg-white p-4 shadow-sm">
		<p class="text-sm font-medium text-gray-500">p95 Duration</p>
		<p class="mt-1 text-2xl font-semibold" data-testid="dashboard-p95-duration">
			{formatDuration(totals.p95_duration_ms)}
		</p>
	</div>
</div>

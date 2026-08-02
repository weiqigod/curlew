<script lang="ts">
	import type { ResultSummary } from '$lib/types/results';

	export let summary: ResultSummary;

	function formatDuration(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}
</script>

<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
	<div class="rounded-lg border bg-white p-4 shadow-sm">
		<p class="text-sm font-medium text-gray-500">Total Runs</p>
		<p class="mt-1 text-2xl font-semibold" data-testid="summary-total-runs">
			{summary.total_runs}
		</p>
	</div>
	<div class="rounded-lg border bg-white p-4 shadow-sm">
		<p class="text-sm font-medium text-gray-500">Passed</p>
		<p class="mt-1 text-2xl font-semibold text-green-600" data-testid="summary-pass-count">
			{summary.total_pass}
		</p>
	</div>
	<div class="rounded-lg border bg-white p-4 shadow-sm">
		<p class="text-sm font-medium text-gray-500">Failed</p>
		<p class="mt-1 text-2xl font-semibold text-red-600" data-testid="summary-fail-count">
			{summary.total_fail}
		</p>
	</div>
	<div class="rounded-lg border bg-white p-4 shadow-sm">
		<p class="text-sm font-medium text-gray-500">Pass Rate</p>
		<p class="mt-1 text-2xl font-semibold" data-testid="summary-pass-rate">
			{summary.pass_rate}%
		</p>
	</div>
	<div class="rounded-lg border bg-white p-4 shadow-sm md:col-span-4">
		<p class="text-sm font-medium text-gray-500">Avg Duration</p>
		<p class="mt-1 text-2xl font-semibold" data-testid="summary-avg-duration">
			{formatDuration(summary.average_duration_ms)}
		</p>
	</div>
</div>

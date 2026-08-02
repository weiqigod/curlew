<script lang="ts">
	import type { TrendEntry } from '$lib/types/dashboard';

	/** Sparse daily pass-rate trend entries from GET /results/stats. */
	export let trend: TrendEntry[];

	const W = 600;
	const H = 160;
	const PAD = 24;

	$: points =
		trend.length === 0
			? ''
			: trend
					.map((t, i) => {
						const x = PAD + (i * (W - 2 * PAD)) / Math.max(1, trend.length - 1);
						const y = H - PAD - t.pass_rate * (H - 2 * PAD);
						return `${x},${y}`;
					})
					.join(' ');
</script>

<div class="overflow-x-auto" data-testid="dashboard-trend-chart">
	<svg
		width={W}
		height={H}
		viewBox="0 0 {W} {H}"
		role="img"
		aria-label="Daily pass-rate trend"
	>
		<!-- y-axis reference labels -->
		<text x={4} y={PAD + 4} class="fill-gray-400" font-size="10">100%</text>
		<text x={4} y={H / 2 + 4} class="fill-gray-400" font-size="10">50%</text>
		<text x={4} y={H - PAD + 4} class="fill-gray-400" font-size="10">0%</text>
		<!-- baseline -->
		<line x1={PAD} y1={H - PAD} x2={W - PAD} y2={H - PAD} stroke="#e5e7eb" />
		{#if trend.length > 0}
			<polyline fill="none" stroke="#3b82f6" stroke-width="2" points={points} />
			{#each trend as t, i}
				<circle
					cx={PAD + (i * (W - 2 * PAD)) / Math.max(1, trend.length - 1)}
					cy={H - PAD - t.pass_rate * (H - 2 * PAD)}
					r="3"
					fill="#3b82f6"
					data-date={t.date}
				/>
			{/each}
		{/if}
	</svg>
</div>

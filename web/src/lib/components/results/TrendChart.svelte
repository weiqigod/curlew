<script lang="ts">
	import type { TrendPoint } from '$lib/types/results';

	export let trend: TrendPoint[];

	const WIDTH = 600;
	const HEIGHT = 120;
	const BAR_GAP = 2;

	$: maxVal = trend.reduce((m, p) => Math.max(m, p.pass + p.fail), 1);
	$: barWidth = trend.length > 0 ? (WIDTH - BAR_GAP * (trend.length - 1)) / trend.length : WIDTH;

	function barHeight(val: number): number {
		return (val / maxVal) * HEIGHT;
	}

	function barX(i: number): number {
		return i * (barWidth + BAR_GAP);
	}
</script>

<div class="overflow-x-auto" data-testid="trend-chart">
	<svg
		width={WIDTH}
		height={HEIGHT}
		viewBox="0 0 {WIDTH} {HEIGHT}"
		role="img"
		aria-label="Pass/fail trend"
	>
		{#each trend as point, i}
			{@const passH = barHeight(point.pass)}
			{@const failH = barHeight(point.fail)}
			{@const totalH = passH + failH}
			<rect
				x={barX(i)}
				y={HEIGHT - totalH}
				width={barWidth}
				height={totalH}
				fill="#ef4444"
			/>
			<rect
				x={barX(i)}
				y={HEIGHT - passH}
				width={barWidth}
				height={passH}
				fill="#22c55e"
			/>
		{/each}
	</svg>
</div>

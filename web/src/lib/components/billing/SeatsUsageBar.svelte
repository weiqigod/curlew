<script lang="ts">
	/** Current seat count (used seats). */
	export let seatCount: number;
	/** Maximum seat limit. */
	export let seatLimit: number;

	$: pct = seatLimit > 0 ? Math.min(100, Math.round((seatCount / seatLimit) * 100)) : 0;
	$: overLimit = seatCount > seatLimit;
</script>

<div class="space-y-1">
	<div class="flex justify-between text-xs text-gray-500">
		<span>{seatCount} / {seatLimit} seats used</span>
		<span>{pct}%</span>
	</div>
	<div
		role="progressbar"
		aria-valuenow={seatCount}
		aria-valuemin={0}
		aria-valuemax={seatLimit}
		aria-label="Seat usage"
		data-testid="seats-usage-bar"
		class="h-2 w-full overflow-hidden rounded-full bg-gray-200"
	>
		<div
			class="h-full rounded-full transition-all {overLimit
				? 'bg-red-500'
				: pct >= 90
					? 'bg-amber-500'
					: 'bg-blue-500'}"
			style="width: {pct}%"
		></div>
	</div>
</div>

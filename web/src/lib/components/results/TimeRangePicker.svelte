<script lang="ts">
	import type { TimeRange } from '$lib/types/results';

	export let current: TimeRange;
	export let slug: string;

	const ranges: { label: string; value: TimeRange }[] = [
		{ label: '24h', value: '24h' },
		{ label: '7d', value: '7d' },
		{ label: '30d', value: '30d' },
		{ label: 'All', value: 'all' }
	];
</script>

<div class="flex gap-1" aria-label="Time range picker">
	{#each ranges as range}
		<a
			href="/org/{slug}/results?range={range.value}"
			class="rounded px-3 py-1 text-sm font-medium transition-colors
				{current === range.value
				? 'bg-blue-600 text-white'
				: 'bg-gray-100 text-gray-700 hover:bg-gray-200'}"
			data-testid="range-picker-{range.value}"
			aria-current={current === range.value ? 'page' : undefined}
		>
			{range.label}
		</a>
	{/each}
</div>

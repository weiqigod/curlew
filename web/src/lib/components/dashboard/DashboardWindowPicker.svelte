<script lang="ts">
	import type { DashboardWindow } from '$lib/types/dashboard';

	/** The currently active window value (controls active state on the tab group). */
	export let current: DashboardWindow;
	/** Org slug used to build the href for each tab anchor. */
	export let slug: string;

	const windows: { label: string; value: DashboardWindow }[] = [
		{ label: '7d', value: '7d' },
		{ label: '30d', value: '30d' },
		{ label: '90d', value: '90d' }
	];
</script>

<div class="flex gap-1" aria-label="Window picker">
	{#each windows as w}
		<a
			href="/org/{slug}/dashboard?window={w.value}"
			data-testid="dashboard-window-{w.value}"
			aria-current={current === w.value ? 'page' : undefined}
			class="rounded px-3 py-1 text-sm font-medium transition-colors {current === w.value
				? 'bg-blue-600 text-white'
				: 'bg-gray-100 text-gray-700 hover:bg-gray-200'}"
		>
			{w.label}
		</a>
	{/each}
</div>

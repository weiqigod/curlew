<script lang="ts">
	export let message: string;
	export let retryUrl: string;
</script>

<div
	class="flex flex-col items-center justify-center rounded-lg border border-red-200 bg-red-50 p-12 text-center"
	data-testid="error-state"
>
	<svg
		class="mb-4 h-12 w-12 text-red-400"
		fill="none"
		viewBox="0 0 24 24"
		stroke="currentColor"
		aria-hidden="true"
	>
		<path
			stroke-linecap="round"
			stroke-linejoin="round"
			stroke-width="1.5"
			d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
		/>
	</svg>
	<p class="mb-4 text-sm font-medium text-red-700">{message}</p>
	<a
		href={retryUrl}
		class="rounded bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
		data-testid="error-retry-button"
	>
		Retry
	</a>
</div>

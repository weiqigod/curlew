<script lang="ts">
	/** Whether the toast is visible. Bindable. */
	export let open: boolean = false;
	/** The message to display. */
	export let message: string = '';
	/** Visual variant of the toast. */
	export let variant: 'success' | 'error' | 'info' = 'success';

	const variantClasses: Record<typeof variant, string> = {
		success: 'border-green-300 bg-green-50 text-green-800',
		error: 'border-red-300 bg-red-50 text-red-800',
		info: 'border-blue-300 bg-blue-50 text-blue-800'
	};

	function dismiss() {
		open = false;
	}

	// Auto-dismiss after 4 seconds
	let timer: ReturnType<typeof setTimeout>;
	$: if (open) {
		clearTimeout(timer);
		timer = setTimeout(() => {
			open = false;
		}, 4000);
	}
</script>

{#if open}
	<div
		data-testid="toast"
		role="alert"
		aria-live="polite"
		class="fixed inset-x-0 top-4 z-50 mx-auto max-w-md rounded-lg border px-4 py-3 shadow-md {variantClasses[variant]}"
	>
		<div class="flex items-center justify-between gap-4">
			<p data-testid="toast-message" class="text-sm font-medium">{message}</p>
			<button
				type="button"
				data-testid="toast-dismiss"
				aria-label="Dismiss notification"
				class="hover:opacity-70"
				on:click={dismiss}
			>
				&times;
			</button>
		</div>
	</div>
{/if}

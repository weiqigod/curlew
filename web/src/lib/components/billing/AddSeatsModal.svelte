<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { ProrationResult } from '$lib/types/subscriptions';

	/** Whether the modal is visible. */
	export let open = false;
	/** Whether a submission is in progress (disables submit button). */
	export let submitting = false;
	/** API-level error message to show below the form. */
	export let error: string | null = null;
	/** Current seat count — used as the baseline for the "+N" input. */
	export let currentSeatCount: number;
	/** Proration result returned after a successful PATCH. */
	export let proration: ProrationResult | null = null;

	const dispatch = createEventDispatcher<{
		submit: { seatCount: number };
		close: void;
	}>();

	let addCount = 3;
	let validationError: string | null = null;

	function resetForm() {
		addCount = 3;
		validationError = null;
	}

	function handleClose() {
		if (submitting) return;
		resetForm();
		dispatch('close');
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') handleClose();
	}

	function validate(): boolean {
		if (!Number.isInteger(addCount) || addCount < 1) {
			validationError = 'Add at least 1 seat.';
			return false;
		}
		validationError = null;
		return true;
	}

	function handleSubmit() {
		if (!validate()) return;
		dispatch('submit', { seatCount: currentSeatCount + addCount });
	}

	/** Format cents as dollars with 2 decimal places. */
	function formatCents(cents: number): string {
		return `$${(Math.abs(cents) / 100).toFixed(2)}`;
	}
</script>

{#if open}
	<div
		class="fixed inset-0 z-40 flex items-center justify-center bg-black/50"
		role="presentation"
		on:click|self={handleClose}
		on:keydown={handleKeydown}
	>
		<div
			data-testid="add-seats-modal"
			role="dialog"
			aria-modal="true"
			aria-labelledby="add-seats-modal-title"
			class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl"
		>
			<div class="mb-4 flex items-center justify-between">
				<h2 id="add-seats-modal-title" class="text-lg font-semibold text-gray-900">Add seats</h2>
				<button
					type="button"
					data-testid="add-seats-close"
					aria-label="Close modal"
					class="text-gray-400 hover:text-gray-600"
					on:click={handleClose}
				>
					&times;
				</button>
			</div>

			{#if proration !== null}
				<div
					data-testid="add-seats-proration"
					role="status"
					class="mb-4 rounded-md border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800"
				>
					{#if proration.net === 0}
						<p>No additional charge — seats added.</p>
					{:else}
						<p>
							Seats updated. Proration charge: <strong>{formatCents(proration.net)}</strong>
						</p>
					{/if}
				</div>
				<div class="flex justify-end">
					<button
						type="button"
						class="rounded-md bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200"
						on:click={handleClose}
					>
						Close
					</button>
				</div>
			{:else}
				<div class="space-y-4">
					<div>
						<label for="add-seats-delta" class="block text-sm font-medium text-gray-700">
							Seats to add
						</label>
						<input
							id="add-seats-delta"
							data-testid="add-seats-delta"
							type="number"
							min="1"
							bind:value={addCount}
							class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none"
						/>
					</div>

					{#if validationError}
						<p data-testid="add-seats-error" class="text-sm text-red-600" role="alert">
							{validationError}
						</p>
					{:else if error}
						<p data-testid="add-seats-error" class="text-sm text-red-600" role="alert">
							{error}
						</p>
					{/if}

					<div class="flex justify-end gap-3 pt-2">
						<button
							type="button"
							class="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
							on:click={handleClose}
						>
							Cancel
						</button>
						<button
							type="button"
							data-testid="add-seats-submit"
							disabled={submitting}
							class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
							on:click={handleSubmit}
						>
							{submitting ? 'Updating…' : 'Add seats'}
						</button>
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}

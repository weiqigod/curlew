<script lang="ts">
	import CronPreview from './CronPreview.svelte';
	import type { CreateScheduleRequest } from '$lib/types/schedules';
	import { createEventDispatcher } from 'svelte';

	const dispatch = createEventDispatcher<{
		submit: CreateScheduleRequest;
		cancel: void;
	}>();

	/** External error message set by the parent after a failed API call. */
	export let apiError = '';

	let name = '';
	let cron = '0 9 * * *';
	let timezone = 'UTC';
	let collectionRef = '';
	let submitting = false;
	let localError = '';

	$: errorMessage = localError || apiError;

	/** Common IANA timezone suggestions. */
	const COMMON_TIMEZONES = [
		'UTC',
		'Europe/Stockholm',
		'Europe/London',
		'Europe/Paris',
		'America/New_York',
		'America/Chicago',
		'America/Denver',
		'America/Los_Angeles',
		'Asia/Tokyo',
		'Asia/Shanghai',
		'Australia/Sydney'
	];

	function handleSubmit() {
		localError = '';
		if (!name.trim()) { localError = 'Name is required.'; return; }
		if (!cron.trim()) { localError = 'Cron expression is required.'; return; }
		if (!collectionRef.trim()) { localError = 'Collection ref is required.'; return; }

		submitting = true;
		dispatch('submit', {
			name: name.trim(),
			cron: cron.trim(),
			timezone: timezone || 'UTC',
			collection_ref: collectionRef.trim()
		});
	}

	/** Called by the parent when submission is complete (success or error). */
	export function resetSubmitting() {
		submitting = false;
	}
</script>

<div
	role="dialog"
	aria-modal="true"
	aria-label="Create schedule"
	class="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
>
	<div class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
		<h2 class="mb-4 text-lg font-semibold text-gray-900">Create schedule</h2>

		<form on:submit|preventDefault={handleSubmit} class="space-y-4">
			<div>
				<label class="block text-sm font-medium text-gray-700" for="sched-name">Name</label>
				<input
					id="sched-name"
					type="text"
					bind:value={name}
					placeholder="nightly"
					class="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
					data-testid="sched-name"
				/>
			</div>

			<div>
				<label class="block text-sm font-medium text-gray-700" for="sched-cron">Cron expression</label>
				<input
					id="sched-cron"
					type="text"
					bind:value={cron}
					placeholder="0 9 * * *"
					class="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
					data-testid="sched-cron"
				/>
				{#if cron.trim()}
					<div class="mt-1">
						<p class="text-xs font-medium text-gray-500">Next 5 firings ({timezone})</p>
						<CronPreview {cron} {timezone} />
					</div>
				{/if}
			</div>

			<div>
				<label class="block text-sm font-medium text-gray-700" for="sched-tz">Timezone</label>
				<select
					id="sched-tz"
					bind:value={timezone}
					class="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
					data-testid="sched-tz"
				>
					{#each COMMON_TIMEZONES as tz}
						<option value={tz}>{tz}</option>
					{/each}
				</select>
			</div>

			<div>
				<label class="block text-sm font-medium text-gray-700" for="sched-ref">Collection ref</label>
				<input
					id="sched-ref"
					type="text"
					bind:value={collectionRef}
					placeholder="smoke.yaml"
					class="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
					data-testid="sched-ref"
				/>
			</div>

			{#if errorMessage}
				<p class="text-sm text-red-600" data-testid="sched-form-error">{errorMessage}</p>
			{/if}

			<div class="flex justify-end gap-3 pt-2">
				<button
					type="button"
					on:click={() => dispatch('cancel')}
					class="rounded px-4 py-2 text-sm text-gray-600 hover:bg-gray-100"
					disabled={submitting}
				>
					Cancel
				</button>
				<button
					type="submit"
					class="rounded bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-700 disabled:opacity-50"
					disabled={submitting}
					data-testid="sched-submit"
				>
					{submitting ? 'Creating…' : 'Create'}
				</button>
			</div>
		</form>
	</div>
</div>

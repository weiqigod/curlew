<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { NotificationChannel, NotificationEventName, CreateRuleRequest } from '$lib/types/notifications';

	export let open: boolean = false;
	export let submitting: boolean = false;
	export let error: string | null = null;

	const dispatch = createEventDispatcher<{
		submit: CreateRuleRequest;
		close: void;
	}>();

	let channel: NotificationChannel = 'slack';
	let target = '';
	let onRunFailed = false;
	let onFlaky = false;
	let validationError: string | null = null;

	function resetForm() {
		channel = 'slack';
		target = '';
		onRunFailed = false;
		onFlaky = false;
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
		validationError = null;

		if (!target.trim()) {
			validationError = 'Target is required.';
			return false;
		}

		if (channel === 'slack' && !target.startsWith('https://')) {
			validationError = 'Slack target must start with https://';
			return false;
		}

		const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
		if (channel === 'email' && !emailPattern.test(target)) {
			validationError = 'Email target must be a valid email address.';
			return false;
		}

		const events: NotificationEventName[] = [];
		if (onRunFailed) events.push('run_failed');
		if (onFlaky) events.push('flaky');
		if (events.length === 0) {
			validationError = 'Select at least one event.';
			return false;
		}

		return true;
	}

	function handleSubmit() {
		if (!validate()) return;

		const events: NotificationEventName[] = [];
		if (onRunFailed) events.push('run_failed');
		if (onFlaky) events.push('flaky');

		dispatch('submit', { channel, target, on: events });
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
			data-testid="notif-modal"
			role="dialog"
			aria-modal="true"
			aria-labelledby="notif-modal-title"
			class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl"
		>
			<div class="mb-4 flex items-center justify-between">
				<h2 id="notif-modal-title" class="text-lg font-semibold text-gray-900">
					Add notification rule
				</h2>
				<button
					type="button"
					data-testid="notif-modal-close"
					aria-label="Close modal"
					class="text-gray-400 hover:text-gray-600"
					on:click={handleClose}
				>
					&times;
				</button>
			</div>

			<div class="space-y-4">
				<div>
					<label for="notif-modal-channel" class="block text-sm font-medium text-gray-700">
						Channel
					</label>
					<select
						id="notif-modal-channel"
						data-testid="notif-modal-channel"
						bind:value={channel}
						class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none"
					>
						<option value="slack">Slack</option>
						<option value="email">Email</option>
					</select>
				</div>

				<div>
					<label for="notif-modal-target" class="block text-sm font-medium text-gray-700">
						{channel === 'slack' ? 'Webhook URL' : 'Email address'}
					</label>
					<input
						id="notif-modal-target"
						data-testid="notif-modal-target"
						type="text"
						bind:value={target}
						placeholder={channel === 'slack' ? 'https://hooks.slack.com/...' : 'alerts@example.com'}
						class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none"
					/>
				</div>

				<fieldset>
					<legend class="block text-sm font-medium text-gray-700">Trigger on</legend>
					<div class="mt-2 space-y-2">
						<label class="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								data-testid="notif-modal-on-run_failed"
								bind:checked={onRunFailed}
							/>
							Run failed
						</label>
						<label class="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								data-testid="notif-modal-on-flaky"
								bind:checked={onFlaky}
							/>
							Flaky test
						</label>
					</div>
				</fieldset>

				{#if validationError}
					<p data-testid="notif-modal-error" class="text-sm text-red-600" role="alert">
						{validationError}
					</p>
				{:else if error}
					<p data-testid="notif-modal-error" class="text-sm text-red-600" role="alert">
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
						data-testid="notif-modal-submit"
						disabled={submitting}
						class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
						on:click={handleSubmit}
					>
						{submitting ? 'Saving…' : 'Add rule'}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

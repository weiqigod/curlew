<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { CreateInvitationRequest, InvitationRole } from '$lib/types/invitations';

	/** Whether the modal is visible. */
	export let open = false;
	/** Whether a submission is in progress (disables submit button). */
	export let submitting = false;
	/** API-level error message to show below the form. */
	export let error: string | null = null;

	const dispatch = createEventDispatcher<{ submit: CreateInvitationRequest; close: void }>();

	let email = '';
	let role: InvitationRole = 'member';
	let validationError: string | null = null;

	const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

	function resetForm() {
		email = '';
		role = 'member';
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

	function handleSubmit() {
		if (!EMAIL_RE.test(email)) {
			validationError = 'Enter a valid email address.';
			return;
		}
		validationError = null;
		dispatch('submit', { email, role });
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
			data-testid="invite-modal"
			role="dialog"
			aria-modal="true"
			aria-labelledby="invite-modal-title"
			class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl"
		>
			<div class="mb-4 flex items-center justify-between">
				<h2 id="invite-modal-title" class="text-lg font-semibold text-gray-900">
					Invite team member
				</h2>
				<button
					type="button"
					data-testid="invite-modal-close"
					aria-label="Close modal"
					class="text-gray-400 hover:text-gray-600"
					on:click={handleClose}
				>
					&times;
				</button>
			</div>

			<div class="space-y-4">
				<div>
					<label for="invite-modal-email" class="block text-sm font-medium text-gray-700">
						Email address
					</label>
					<input
						id="invite-modal-email"
						data-testid="invite-modal-email"
						type="email"
						bind:value={email}
						placeholder="colleague@example.com"
						class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none"
					/>
				</div>

				<div>
					<label for="invite-modal-role" class="block text-sm font-medium text-gray-700">
						Role
					</label>
					<select
						id="invite-modal-role"
						data-testid="invite-modal-role"
						bind:value={role}
						class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none"
					>
						<option value="member">Member</option>
						<option value="admin">Admin</option>
					</select>
				</div>

				{#if validationError}
					<p data-testid="invite-modal-error" class="text-sm text-red-600" role="alert">
						{validationError}
					</p>
				{:else if error}
					<p data-testid="invite-modal-error" class="text-sm text-red-600" role="alert">
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
						data-testid="invite-modal-submit"
						disabled={submitting}
						class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
						on:click={handleSubmit}
					>
						{submitting ? 'Inviting…' : 'Send invite'}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

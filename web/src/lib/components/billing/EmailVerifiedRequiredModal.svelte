<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { authApi } from '$lib/api/auth';

	/** Whether the modal is open. */
	export let open = false;

	const dispatch = createEventDispatcher<{ close: void }>();

	let email = '';
	let submitting = false;
	let resent = false;
	let resendError: string | null = null;

	async function handleResend() {
		if (!email) return;
		submitting = true;
		resendError = null;
		try {
			await authApi.resendEmailVerification(email);
			resent = true;
		} catch {
			resendError = 'Failed to send verification email. Please try again.';
		} finally {
			submitting = false;
		}
	}

	function close() {
		open = false;
		resent = false;
		resendError = null;
		email = '';
		dispatch('close');
	}
</script>

{#if open}
	<!-- Backdrop -->
	<div
		class="fixed inset-0 z-40 bg-black/50"
		role="presentation"
		on:click={close}
		on:keydown={(e) => e.key === 'Escape' && close()}
	></div>

	<!-- Modal -->
	<div
		data-testid="email-verified-required-modal"
		role="dialog"
		aria-modal="true"
		aria-labelledby="email-verified-modal-title"
		class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border border-gray-200 bg-white p-6 shadow-xl"
	>
		<h2 id="email-verified-modal-title" class="mb-2 text-lg font-semibold text-gray-900">
			Email verification required
		</h2>
		<p class="mb-4 text-sm text-gray-600">
			You must verify your email address before managing your subscription. Enter your email
			address below to receive a new verification link.
		</p>

		{#if resent}
			<div
				data-testid="verification-resent-confirmation"
				class="rounded-md bg-green-50 p-3 text-sm text-green-800"
				role="alert"
			>
				Verification email sent. Check your inbox (and spam folder).
			</div>
			<div class="mt-4 flex justify-end">
				<button
					type="button"
					on:click={close}
					class="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
				>
					Close
				</button>
			</div>
		{:else}
			{#if resendError}
				<p class="mb-3 text-sm text-red-600" role="alert">{resendError}</p>
			{/if}

			<form on:submit|preventDefault={handleResend}>
				<div class="mb-4">
					<label for="verify-email" class="mb-1 block text-sm font-medium text-gray-700">
						Email address
					</label>
					<input
						id="verify-email"
						name="email"
						type="email"
						bind:value={email}
						required
						autocomplete="email"
						class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
						placeholder="you@example.com"
					/>
				</div>

				<div class="flex justify-end gap-2">
					<button
						type="button"
						on:click={close}
						class="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
					>
						Cancel
					</button>
					<button
						type="submit"
						data-testid="resend-verification-button"
						disabled={submitting}
						class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
					>
						{submitting ? 'Sending…' : 'Resend verification email'}
					</button>
				</div>
			</form>
		{/if}
	</div>
{/if}

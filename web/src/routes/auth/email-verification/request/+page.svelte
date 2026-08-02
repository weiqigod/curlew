<script lang="ts">
	import { enhance } from '$app/forms';
	import type { ActionData } from './$types';

	export let form: ActionData;
</script>

<svelte:head>
	<title>Verify your email — ApiTool</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-gray-50 px-4">
	<div class="w-full max-w-md rounded-lg border border-gray-200 bg-white p-8 shadow-sm">
		<h1 class="mb-2 text-2xl font-bold text-gray-900">Resend verification email</h1>
		<p class="mb-6 text-sm text-gray-600">
			Enter your email address and we'll send you a new verification link.
		</p>

		{#if form?.submitted}
			<div
				data-testid="confirmation-message"
				class="rounded-md bg-green-50 p-4 text-sm text-green-800"
				role="alert"
			>
				If that email address is registered and unverified, you'll receive a verification
				link shortly. Check your inbox (and spam folder).
			</div>
		{:else}
			<form method="POST" use:enhance>
				<div class="mb-4">
					<label for="email" class="mb-1 block text-sm font-medium text-gray-700">
						Email address
					</label>
					<input
						id="email"
						name="email"
						type="email"
						required
						autocomplete="email"
						data-testid="email-input"
						class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
						placeholder="you@example.com"
					/>
				</div>

				<button
					type="submit"
					data-testid="submit-button"
					class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
				>
					Send verification link
				</button>
			</form>
		{/if}
	</div>
</div>

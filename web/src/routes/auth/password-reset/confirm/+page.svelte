<script lang="ts">
	import { enhance } from '$app/forms';
	import type { ActionData, PageData } from './$types';

	export let data: PageData;
	export let form: ActionData;
</script>

<svelte:head>
	<title>Set new password — Curlew</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-gray-50 px-4">
	<div class="w-full max-w-md rounded-lg border border-gray-200 bg-white p-8 shadow-sm">
		<h1 class="mb-6 text-2xl font-bold text-gray-900">Set a new password</h1>

		{#if !data.hasToken}
			<div class="rounded-md bg-yellow-50 p-4 text-sm text-yellow-800" role="alert">
				<p class="mb-2">No reset token found in the URL.</p>
				<a
					href="/auth/password-reset/request"
					data-testid="request-new-link"
					class="font-medium underline hover:no-underline"
				>
					Request a new password-reset link
				</a>
			</div>
		{:else}
			{#if form?.error === 'token_invalid'}
				<div
					data-testid="error-token-invalid"
					class="mb-4 rounded-md bg-red-50 p-4 text-sm text-red-800"
					role="alert"
				>
					<p class="mb-2">This reset link is invalid or has expired.</p>
					<a
						href="/auth/password-reset/request"
						data-testid="request-new-link"
						class="font-medium underline hover:no-underline"
					>
						Request a new password-reset link
					</a>
				</div>
			{/if}

			{#if form?.error === 'weak_password'}
				<div
					data-testid="error-weak-password"
					class="mb-4 rounded-md bg-orange-50 p-4 text-sm text-orange-800"
					role="alert"
				>
					Password strength {'score' in form ? form.score : 0}/4 — minimum 3/4 required. Please choose a stronger
					password.
				</div>
			{/if}

			{#if form?.error === 'server_error'}
				<div
					class="mb-4 rounded-md bg-red-50 p-4 text-sm text-red-800"
					role="alert"
				>
					Something went wrong. Please try again.
				</div>
			{/if}

			<form method="POST" use:enhance>
				<input type="hidden" name="token" value={data.token} />

				<div class="mb-4">
					<label for="new_password" class="mb-1 block text-sm font-medium text-gray-700">
						New password
					</label>
					<input
						id="new_password"
						name="new_password"
						type="password"
						required
						autocomplete="new-password"
						data-testid="password-input"
						class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
						placeholder="Choose a strong password"
					/>
					<p class="mt-1 text-xs text-gray-500">Minimum strength: 3/4</p>
				</div>

				<button
					type="submit"
					data-testid="submit-button"
					class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
				>
					Set new password
				</button>
			</form>
		{/if}
	</div>
</div>

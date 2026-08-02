<script lang="ts">
	import type { PageData } from './$types';

	export let data: PageData;
</script>

<svelte:head>
	<title>Verifying email — Curlew</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-gray-50 px-4">
	<div class="w-full max-w-md rounded-lg border border-gray-200 bg-white p-8 shadow-sm">
		{#if data.error === 'missing_token' || data.error === 'token_invalid'}
			<h1 class="mb-4 text-2xl font-bold text-gray-900">Verification link invalid</h1>
			<div
				data-testid="error-token-invalid"
				class="mb-4 rounded-md bg-red-50 p-4 text-sm text-red-800"
				role="alert"
			>
				This verification link is invalid, has already been used, or has expired.
			</div>
			<a
				href="/auth/email-verification/request"
				data-testid="request-new-link"
				class="text-sm font-medium text-blue-600 underline hover:no-underline"
			>
				Request a new verification email
			</a>
		{:else if data.error === 'server_error'}
			<h1 class="mb-4 text-2xl font-bold text-gray-900">Something went wrong</h1>
			<p class="text-sm text-gray-600">Please try again or request a new verification link.</p>
			<a
				href="/auth/email-verification/request"
				class="mt-4 block text-sm font-medium text-blue-600 underline hover:no-underline"
			>
				Request a new verification email
			</a>
		{:else}
			<h1 class="mb-4 text-2xl font-bold text-gray-900">Verifying your email…</h1>
			<p class="text-sm text-gray-600">Please wait while we verify your email address.</p>
		{/if}
	</div>
</div>

<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { OidcConfigView, OidcUpsertRequest } from '$lib/types/sso';

	/** Existing OIDC config for pre-population (null = not yet configured). */
	export let initial: OidcConfigView | null = null;
	/** Whether a submission is in progress. */
	export let submitting: boolean = false;
	/** Server-side error from the last submission. */
	export let serverError: { message: string; field?: string } | null = null;

	const dispatch = createEventDispatcher<{ submit: OidcUpsertRequest }>();

	let issuerUrl = initial?.issuer_url ?? '';
	let clientId = initial?.client_id ?? '';
	let clientSecret = '';
	let redirectUri = initial?.redirect_uri ?? '';
	let scopes = initial?.scopes ?? 'openid email profile';

	let validationError: string | null = null;
	let validationField: string | null = null;

	function validate(): boolean {
		validationError = null;
		validationField = null;
		if (!issuerUrl.trim()) {
			validationError = 'Issuer URL is required.';
			validationField = 'issuer_url';
			return false;
		}
		if (!clientId.trim()) {
			validationError = 'Client ID is required.';
			validationField = 'client_id';
			return false;
		}
		if (!clientSecret.trim()) {
			validationError = 'Client Secret is required.';
			validationField = 'client_secret';
			return false;
		}
		return true;
	}

	function handleSubmit() {
		if (!validate()) return;
		const req: OidcUpsertRequest = {
			issuer_url: issuerUrl.trim(),
			client_id: clientId.trim(),
			client_secret: clientSecret.trim()
		};
		if (redirectUri.trim()) req.redirect_uri = redirectUri.trim();
		if (scopes.trim()) req.scopes = scopes.trim();
		dispatch('submit', req);
	}

	$: fieldError = (field: string) =>
		(validationField === field ? validationError : null) ??
		(serverError?.field === field ? serverError.message : null);
</script>

<form on:submit|preventDefault={handleSubmit} class="space-y-4" data-testid="oidc-form">
	<div>
		<label for="oidc-issuer-url" class="block text-sm font-medium text-gray-700">
			Issuer URL <span aria-hidden="true" class="text-red-500">*</span>
		</label>
		<input
			id="oidc-issuer-url"
			data-testid="oidc-issuer-url"
			type="url"
			bind:value={issuerUrl}
			placeholder="https://login.example.com"
			class="mt-1 block w-full rounded-md border px-3 py-2 text-sm focus:outline-none
				{fieldError('issuer_url')
				? 'border-red-400 focus:border-red-500'
				: 'border-gray-300 focus:border-blue-500'}"
		/>
		{#if fieldError('issuer_url')}
			<p class="mt-1 text-xs text-red-600" role="alert">{fieldError('issuer_url')}</p>
		{/if}
	</div>

	<div>
		<label for="oidc-client-id" class="block text-sm font-medium text-gray-700">
			Client ID <span aria-hidden="true" class="text-red-500">*</span>
		</label>
		<input
			id="oidc-client-id"
			data-testid="oidc-client-id"
			type="text"
			bind:value={clientId}
			placeholder="your-client-id"
			class="mt-1 block w-full rounded-md border px-3 py-2 text-sm focus:outline-none
				{fieldError('client_id')
				? 'border-red-400 focus:border-red-500'
				: 'border-gray-300 focus:border-blue-500'}"
		/>
		{#if fieldError('client_id')}
			<p class="mt-1 text-xs text-red-600" role="alert">{fieldError('client_id')}</p>
		{/if}
	</div>

	<div>
		<label for="oidc-client-secret" class="block text-sm font-medium text-gray-700">
			Client Secret <span aria-hidden="true" class="text-red-500">*</span>
		</label>
		<input
			id="oidc-client-secret"
			data-testid="oidc-client-secret"
			type="password"
			bind:value={clientSecret}
			placeholder="your-client-secret"
			class="mt-1 block w-full rounded-md border px-3 py-2 text-sm focus:outline-none
				{fieldError('client_secret')
				? 'border-red-400 focus:border-red-500'
				: 'border-gray-300 focus:border-blue-500'}"
		/>
		{#if fieldError('client_secret')}
			<p class="mt-1 text-xs text-red-600" role="alert">{fieldError('client_secret')}</p>
		{/if}
	</div>

	<div>
		<label for="oidc-redirect-uri" class="block text-sm font-medium text-gray-700">
			Redirect URI <span class="text-gray-400">(optional)</span>
		</label>
		<input
			id="oidc-redirect-uri"
			data-testid="oidc-redirect-uri"
			type="url"
			bind:value={redirectUri}
			placeholder="https://app.example.com/oidc/callback"
			class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none"
		/>
	</div>

	<div>
		<label for="oidc-scopes" class="block text-sm font-medium text-gray-700">
			Scopes <span class="text-gray-400">(optional)</span>
		</label>
		<input
			id="oidc-scopes"
			data-testid="oidc-scopes"
			type="text"
			bind:value={scopes}
			placeholder="openid email profile"
			class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none"
		/>
	</div>

	{#if validationError && !validationField}
		<p class="text-sm text-red-600" role="alert">{validationError}</p>
	{:else if serverError && !serverError.field}
		<p data-testid="oidc-server-error" class="text-sm text-red-600" role="alert">
			{serverError.message}
		</p>
	{/if}

	<div class="flex justify-end pt-2">
		<button
			type="submit"
			data-testid="oidc-submit"
			disabled={submitting}
			class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
		>
			{submitting ? 'Saving…' : 'Save OIDC configuration'}
		</button>
	</div>
</form>

<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { SamlConfigView, SamlUpsertRequest } from '$lib/types/sso';

	/** Existing SAML config for pre-population (null = not yet configured). */
	export let initial: SamlConfigView | null = null;
	/** Whether a submission is in progress. */
	export let submitting: boolean = false;
	/** Server-side error from the last submission. */
	export let serverError: { message: string; field?: string } | null = null;

	const dispatch = createEventDispatcher<{ submit: SamlUpsertRequest }>();

	let idpMetadataUrl = initial?.idp_metadata_url ?? '';
	let acsUrl = initial?.acs_url ?? '';
	let entityId = initial?.entity_id ?? '';
	let idpSsoUrl = initial?.idp_sso_url ?? '';
	let idpCertPem = '';

	let validationError: string | null = null;
	let validationField: string | null = null;

	function validate(): boolean {
		validationError = null;
		validationField = null;
		if (!idpMetadataUrl.trim()) {
			validationError = 'IdP Metadata URL is required.';
			validationField = 'idp_metadata_url';
			return false;
		}
		if (!acsUrl.trim()) {
			validationError = 'ACS URL is required.';
			validationField = 'acs_url';
			return false;
		}
		if (!entityId.trim()) {
			validationError = 'Entity ID is required.';
			validationField = 'entity_id';
			return false;
		}
		return true;
	}

	function handleSubmit() {
		if (!validate()) return;
		const req: SamlUpsertRequest = {
			idp_metadata_url: idpMetadataUrl.trim(),
			acs_url: acsUrl.trim(),
			entity_id: entityId.trim()
		};
		if (idpSsoUrl.trim()) req.idp_sso_url = idpSsoUrl.trim();
		if (idpCertPem.trim()) req.idp_cert_pem = idpCertPem.trim();
		dispatch('submit', req);
	}

	$: fieldError = (field: string) =>
		(validationField === field ? validationError : null) ??
		(serverError?.field === field ? serverError.message : null);
</script>

<form on:submit|preventDefault={handleSubmit} class="space-y-4" data-testid="saml-form">
	<div>
		<label for="saml-idp-metadata-url" class="block text-sm font-medium text-gray-700">
			IdP Metadata URL <span aria-hidden="true" class="text-red-500">*</span>
		</label>
		<input
			id="saml-idp-metadata-url"
			data-testid="saml-idp-metadata-url"
			type="url"
			bind:value={idpMetadataUrl}
			placeholder="https://idp.example.com/metadata"
			class="mt-1 block w-full rounded-md border px-3 py-2 text-sm focus:outline-none
				{fieldError('idp_metadata_url')
				? 'border-red-400 focus:border-red-500'
				: 'border-gray-300 focus:border-blue-500'}"
		/>
		{#if fieldError('idp_metadata_url')}
			<p class="mt-1 text-xs text-red-600" role="alert">{fieldError('idp_metadata_url')}</p>
		{/if}
	</div>

	<div>
		<label for="saml-acs-url" class="block text-sm font-medium text-gray-700">
			ACS URL <span aria-hidden="true" class="text-red-500">*</span>
		</label>
		<input
			id="saml-acs-url"
			data-testid="saml-acs-url"
			type="url"
			bind:value={acsUrl}
			placeholder="https://sp.example.com/acs"
			class="mt-1 block w-full rounded-md border px-3 py-2 text-sm focus:outline-none
				{fieldError('acs_url')
				? 'border-red-400 focus:border-red-500'
				: 'border-gray-300 focus:border-blue-500'}"
		/>
		{#if fieldError('acs_url')}
			<p class="mt-1 text-xs text-red-600" role="alert">{fieldError('acs_url')}</p>
		{/if}
	</div>

	<div>
		<label for="saml-entity-id" class="block text-sm font-medium text-gray-700">
			Entity ID <span aria-hidden="true" class="text-red-500">*</span>
		</label>
		<input
			id="saml-entity-id"
			data-testid="saml-entity-id"
			type="text"
			bind:value={entityId}
			placeholder="https://sp.example.com"
			class="mt-1 block w-full rounded-md border px-3 py-2 text-sm focus:outline-none
				{fieldError('entity_id')
				? 'border-red-400 focus:border-red-500'
				: 'border-gray-300 focus:border-blue-500'}"
		/>
		{#if fieldError('entity_id')}
			<p class="mt-1 text-xs text-red-600" role="alert">{fieldError('entity_id')}</p>
		{/if}
	</div>

	<div>
		<label for="saml-idp-sso-url" class="block text-sm font-medium text-gray-700">
			IdP SSO URL <span class="text-gray-400">(optional)</span>
		</label>
		<input
			id="saml-idp-sso-url"
			data-testid="saml-idp-sso-url"
			type="url"
			bind:value={idpSsoUrl}
			placeholder="https://idp.example.com/sso"
			class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none"
		/>
	</div>

	<div>
		<label for="saml-idp-cert-pem" class="block text-sm font-medium text-gray-700">
			IdP Certificate PEM <span class="text-gray-400">(optional)</span>
		</label>
		<textarea
			id="saml-idp-cert-pem"
			data-testid="saml-idp-cert-pem"
			bind:value={idpCertPem}
			rows="4"
			placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
			class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 font-mono text-xs focus:border-blue-500 focus:outline-none"
		></textarea>
	</div>

	{#if validationError && !validationField}
		<p class="text-sm text-red-600" role="alert">{validationError}</p>
	{:else if serverError && !serverError.field}
		<p data-testid="saml-server-error" class="text-sm text-red-600" role="alert">
			{serverError.message}
		</p>
	{/if}

	<div class="flex justify-end pt-2">
		<button
			type="submit"
			data-testid="saml-submit"
			disabled={submitting}
			class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
		>
			{submitting ? 'Saving…' : 'Save SAML configuration'}
		</button>
	</div>
</form>

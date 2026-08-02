<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { PageData } from './$types';
	import { ssoApi } from '$lib/api/sso';
	import type { SamlUpsertRequest, OidcUpsertRequest } from '$lib/types/sso';
	import { ApiError } from '$lib/types/api-error';
	import Tabs from '$lib/components/sso/SsoTabs.svelte';
	import Toast from '$lib/components/ui/Toast.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';
	import SamlConfigForm from '$lib/components/sso/SamlConfigForm.svelte';
	import OidcConfigForm from '$lib/components/sso/OidcConfigForm.svelte';

	export let data: PageData;

	let active: 'saml' | 'oidc' = data.config.sso_provider === 'oidc' ? 'oidc' : 'saml';
	let submitting = false;
	let serverError: { message: string; field?: string } | null = null;
	let toast: { open: boolean; message: string; variant: 'success' | 'error' } = {
		open: false,
		message: '',
		variant: 'success'
	};

	function toServerError(err: unknown): { message: string; field?: string } {
		if (err instanceof ApiError) {
			return { message: err.message, field: err.field };
		}
		return { message: err instanceof Error ? err.message : 'Request failed.' };
	}

	async function handleSamlSubmit(e: CustomEvent<SamlUpsertRequest>) {
		submitting = true;
		serverError = null;
		try {
			await ssoApi.upsertSaml(data.org.id, e.detail);
			toast = { open: true, message: 'SSO enabled', variant: 'success' };
			await invalidateAll();
		} catch (err) {
			serverError = toServerError(err);
		} finally {
			submitting = false;
		}
	}

	async function handleOidcSubmit(e: CustomEvent<OidcUpsertRequest>) {
		submitting = true;
		serverError = null;
		try {
			await ssoApi.upsertOidc(data.org.id, e.detail);
			toast = { open: true, message: 'SSO enabled', variant: 'success' };
			await invalidateAll();
		} catch (err) {
			serverError = toServerError(err);
		} finally {
			submitting = false;
		}
	}

	$: testLoginUrl =
		data.config.sso_enabled && data.config.sso_provider
			? `/api/v1/sso/${data.config.sso_provider}/${data.org.id}/login`
			: null;
</script>

<svelte:head>
	<title>{data.org.name} — SSO Settings</title>
</svelte:head>

<div class="mx-auto max-w-4xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 data-testid="sso-heading" class="text-2xl font-bold text-gray-900">SSO Settings</h1>
		{#if data.config.sso_enabled && data.config.sso_provider}
			<span
				data-testid="sso-provider-badge"
				class="rounded bg-green-100 px-2 py-1 text-xs font-medium text-green-800"
			>
				{data.config.sso_provider.toUpperCase()} enabled
			</span>
		{/if}
	</div>

	{#if data.error}
		<ErrorState message={data.error} retryUrl="?reload=1" />
	{:else}
		<Tabs tabs={[{ id: 'saml', label: 'SAML' }, { id: 'oidc', label: 'OIDC' }]} bind:active>
			<div slot="saml">
				<SamlConfigForm
					initial={data.config.saml_config}
					{submitting}
					{serverError}
					on:submit={handleSamlSubmit}
				/>
			</div>
			<div slot="oidc">
				<OidcConfigForm
					initial={data.config.oidc_config}
					{submitting}
					{serverError}
					on:submit={handleOidcSubmit}
				/>
			</div>
		</Tabs>

		{#if testLoginUrl}
			<a
				data-testid="sso-test-login"
				href={testLoginUrl}
				target="_blank"
				rel="noopener noreferrer"
				class="mt-6 inline-block rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
			>
				Test SSO login
			</a>
		{/if}
	{/if}
</div>

<Toast bind:open={toast.open} message={toast.message} variant={toast.variant} />

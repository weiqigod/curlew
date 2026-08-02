<script lang="ts">
	import type { PageData } from './$types';
	import ConnectGitLabModal from '$lib/components/gitlab/ConnectGitLabModal.svelte';
	import type { GitLabInstallation } from '$lib/types/gitlab-integrations';
	import { gitlabIntegrationsApi } from '$lib/api/gitlab-integrations';
	import { ApiError } from '$lib/types/api-error';

	export let data: PageData;

	const { org } = data;

	let installations: GitLabInstallation[] = data.installations;
	let loadError = data.loadError;

	let showConnectModal = false;
	let rePasteInstId: string | null = null;
	let rePasteProjectPath = '';
	let apiError = '';
	let apiFieldErrors: Record<string, string> = {};
	let modalRef: ConnectGitLabModal | undefined;
	let disconnectingId: string | null = null;
	let showConfirmId: string | null = null;

	/** Relative-time formatting. */
	function relativeTime(iso: string): string {
		const diff = Date.now() - new Date(iso).getTime();
		const seconds = Math.floor(diff / 1000);
		if (seconds < 60) return `${seconds}s ago`;
		const minutes = Math.floor(seconds / 60);
		if (minutes < 60) return `${minutes}m ago`;
		const hours = Math.floor(minutes / 60);
		if (hours < 24) return `${hours}h ago`;
		return new Date(iso).toLocaleDateString();
	}

	function openConnect() {
		rePasteInstId = null;
		rePasteProjectPath = '';
		apiError = '';
		apiFieldErrors = {};
		showConnectModal = true;
	}

	function openRePaste(inst: GitLabInstallation) {
		rePasteInstId = inst.id;
		rePasteProjectPath = inst.project_path;
		apiError = '';
		apiFieldErrors = {};
		showConnectModal = true;
	}

	async function handleConnect(event: CustomEvent<{ project_path: string; access_token: string; gitlab_base_url?: string; gitlab_ca_bundle?: string }>) {
		apiError = '';
		apiFieldErrors = {};

		try {
			// Re-paste flow: soft-delete old row first.
			if (rePasteInstId) {
				await gitlabIntegrationsApi.remove(org.id, rePasteInstId);
				installations = installations.filter((i) => i.id !== rePasteInstId);
			}

			const created = await gitlabIntegrationsApi.create(org.id, event.detail);
			installations = [...installations, created];
			showConnectModal = false;
		} catch (e) {
			modalRef?.resetSubmitting();
			if (e instanceof ApiError) {
				if (e.code === 'project_not_found') {
					apiFieldErrors = { project_path: 'Project not found or PAT lacks access.' };
				} else if (e.code === 'pat_unauthorized') {
					apiFieldErrors = { access_token: 'Access token rejected by GitLab. Check it is valid.' };
				} else if (e.code === 'insecure_base_url') {
					apiFieldErrors = { gitlab_base_url: 'Only https:// URLs are permitted.' };
				} else if (e.code === 'duplicate_integration') {
					apiFieldErrors = { project_path: 'An active integration for this project already exists.' };
				} else {
					apiError = e.message;
				}
			} else {
				apiError = e instanceof Error ? e.message : 'Failed to connect GitLab integration.';
			}
		}
	}

	function handleDisconnect(id: string) {
		showConfirmId = id;
	}

	async function confirmDisconnect(id: string) {
		disconnectingId = id;
		showConfirmId = null;
		try {
			await gitlabIntegrationsApi.remove(org.id, id);
			installations = installations.filter((i) => i.id !== id);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Failed to disconnect integration.';
		} finally {
			disconnectingId = null;
		}
	}
</script>

<svelte:head>
	<title>GitLab Integration — {org.name}</title>
</svelte:head>

<div class="mx-auto max-w-4xl space-y-6 px-4 py-8" data-testid="gitlab-integration-page">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-semibold text-gray-900">GitLab Integration</h1>
			<p class="text-sm text-gray-500">Organization: {org.name}</p>
		</div>
		<button
			type="button"
			class="rounded bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-700"
			on:click={openConnect}
			data-testid="connect-gitlab-button-header"
		>
			Connect GitLab
		</button>
	</div>

	{#if loadError}
		<div
			class="rounded border border-red-300 bg-red-50 p-4 text-red-700"
			data-testid="gitlab-load-error"
		>
			{loadError}
		</div>
	{/if}

	{#if installations.length === 0 && !loadError}
		<div
			class="rounded border border-gray-200 bg-gray-50 p-8 text-center"
			data-testid="gitlab-empty-state"
		>
			<p class="text-gray-600">No GitLab integrations connected yet.</p>
			<button
				type="button"
				class="mt-3 rounded bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-700"
				on:click={openConnect}
				data-testid="connect-gitlab-button"
			>
				Connect GitLab
			</button>
		</div>
	{:else if installations.length > 0}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white" data-testid="gitlab-integrations-table">
			<table class="min-w-full divide-y divide-gray-200">
				<thead class="bg-gray-50">
					<tr>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Project</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Instance</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Last Status Post</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Token</th>
						<th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">Actions</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 bg-white">
					{#each installations as inst (inst.id)}
						<tr data-testid="gitlab-row-{inst.id}">
							<td class="px-4 py-3 text-sm text-gray-900">{inst.project_path}</td>
							<td class="px-4 py-3 text-sm text-gray-500">{inst.gitlab_base_url}</td>
							<td class="px-4 py-3 text-sm text-gray-500">
								{inst.last_status_post_at ? relativeTime(inst.last_status_post_at) : '—'}
							</td>
							<td class="px-4 py-3">
								{#if inst.access_token_revoked_at}
									<span
										class="inline-flex items-center rounded-full bg-red-100 px-2.5 py-0.5 text-xs font-medium text-red-800"
										data-testid="gitlab-token-revoked-badge"
									>
										token revoked
									</span>
									<button
										type="button"
										class="ml-2 text-xs text-indigo-600 hover:text-indigo-800"
										on:click={() => openRePaste(inst)}
										data-testid="gitlab-repaste-pat-button"
									>
										Re-paste PAT
									</button>
								{:else}
									<span class="inline-flex items-center rounded-full bg-green-100 px-2.5 py-0.5 text-xs font-medium text-green-800">
										connected
									</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-right">
								{#if showConfirmId === inst.id}
									<span class="text-sm text-gray-600">Confirm?</span>
									<button
										type="button"
										class="ml-2 text-sm text-red-600 hover:text-red-800"
										on:click={() => confirmDisconnect(inst.id)}
										data-testid="disconnect-confirm-button"
										disabled={!!disconnectingId}
									>
										Yes, disconnect
									</button>
									<button
										type="button"
										class="ml-2 text-sm text-gray-500 hover:text-gray-700"
										on:click={() => (showConfirmId = null)}
									>
										Cancel
									</button>
								{:else}
									<button
										type="button"
										class="text-sm text-gray-500 hover:text-red-600 disabled:opacity-50"
										on:click={() => handleDisconnect(inst.id)}
										disabled={disconnectingId === inst.id}
										data-testid="disconnect-gitlab-button"
									>
										{disconnectingId === inst.id ? 'Disconnecting…' : 'Disconnect'}
									</button>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

{#if showConnectModal}
	<ConnectGitLabModal
		bind:this={modalRef}
		initialProjectPath={rePasteProjectPath}
		{apiError}
		{apiFieldErrors}
		on:submit={handleConnect}
		on:cancel={() => (showConnectModal = false)}
	/>
{/if}

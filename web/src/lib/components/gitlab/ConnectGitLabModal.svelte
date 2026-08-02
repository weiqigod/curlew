<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { CreateGitLabIntegrationRequest } from '$lib/types/gitlab-integrations';

	const dispatch = createEventDispatcher<{
		submit: CreateGitLabIntegrationRequest;
		cancel: void;
	}>();

	/** Pre-fill the project path (used for Re-paste PAT flow). */
	export let initialProjectPath = '';

	let gitlabBaseUrl = 'https://gitlab.com';
	let projectPath = initialProjectPath;
	let accessToken = '';
	let gitlabCaBundle = '';
	let showCaBundle = false;
	let submitting = false;
	let localError = '';
	let fieldErrors: Record<string, string> = {};

	/** External field errors set by parent after API response. */
	export let apiFieldErrors: Record<string, string> = {};
	/** External general error set by parent after API response. */
	export let apiError = '';

	$: generalError = localError || apiError;

	function handleSubmit() {
		localError = '';
		fieldErrors = {};

		if (!projectPath.trim()) {
			fieldErrors.project_path = 'Project path is required (e.g. group/project).';
			return;
		}
		if (projectPath.includes('%')) {
			fieldErrors.project_path = 'Enter a decoded path without URL-encoding (e.g. group/project, not group%2Fproject).';
			return;
		}
		if (!accessToken.trim()) {
			fieldErrors.access_token = 'Access token is required.';
			return;
		}
		if (!gitlabBaseUrl.trim()) {
			fieldErrors.gitlab_base_url = 'GitLab base URL is required.';
			return;
		}

		submitting = true;
		dispatch('submit', {
			project_path: projectPath.trim(),
			access_token: accessToken.trim(),
			gitlab_base_url: gitlabBaseUrl.trim(),
			gitlab_ca_bundle: gitlabCaBundle.trim() || undefined
		});
	}

	/** Called by parent when submission is complete. */
	export function resetSubmitting() {
		submitting = false;
	}
</script>

<div
	role="dialog"
	aria-modal="true"
	aria-label="Connect GitLab"
	class="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
	data-testid="gitlab-modal"
>
	<div class="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl">
		<h2 class="mb-4 text-lg font-semibold text-gray-900">Connect GitLab Project</h2>

		<form on:submit|preventDefault={handleSubmit} class="space-y-4">
			<!-- GitLab Base URL -->
			<div>
				<label class="block text-sm font-medium text-gray-700" for="gitlab-modal-base-url">
					GitLab Base URL
				</label>
				<input
					id="gitlab-modal-base-url"
					type="url"
					bind:value={gitlabBaseUrl}
					placeholder="https://gitlab.com"
					class="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
					data-testid="gitlab-modal-base-url"
				/>
				{#if fieldErrors.gitlab_base_url || apiFieldErrors.gitlab_base_url}
					<p class="mt-1 text-sm text-red-600" data-testid="gitlab-modal-error-gitlab_base_url">
						{fieldErrors.gitlab_base_url || apiFieldErrors.gitlab_base_url}
					</p>
				{/if}
			</div>

			<!-- Project Path -->
			<div>
				<label class="block text-sm font-medium text-gray-700" for="gitlab-modal-path">
					Project Path <span class="text-gray-400">(e.g. group/project)</span>
				</label>
				<input
					id="gitlab-modal-path"
					type="text"
					bind:value={projectPath}
					placeholder="group/project"
					class="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
					data-testid="gitlab-modal-path"
				/>
				{#if fieldErrors.project_path || apiFieldErrors.project_path}
					<p class="mt-1 text-sm text-red-600" data-testid="gitlab-modal-error-project_path">
						{fieldErrors.project_path || apiFieldErrors.project_path}
					</p>
				{/if}
			</div>

			<!-- Access Token -->
			<div>
				<label class="block text-sm font-medium text-gray-700" for="gitlab-modal-pat">
					Project Access Token
				</label>
				<input
					id="gitlab-modal-pat"
					type="password"
					bind:value={accessToken}
					placeholder="glpat-xxxxxxxxxxxxxxxxxxxx"
					autocomplete="new-password"
					class="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
					data-testid="gitlab-modal-pat"
				/>
				{#if fieldErrors.access_token || apiFieldErrors.access_token}
					<p class="mt-1 text-sm text-red-600" data-testid="gitlab-modal-error-access_token">
						{fieldErrors.access_token || apiFieldErrors.access_token}
					</p>
				{/if}
				<p class="mt-1 text-xs text-gray-500">
					Requires <code>api</code> scope. The token is stored encrypted and never returned.
				</p>
			</div>

			<!-- CA Bundle (collapsible) -->
			<div>
				<button
					type="button"
					class="text-sm text-indigo-600 hover:text-indigo-800"
					on:click={() => (showCaBundle = !showCaBundle)}
				>
					{showCaBundle ? 'Hide' : 'Advanced: Custom CA Bundle'} ↕
				</button>
				{#if showCaBundle}
					<div class="mt-2">
						<label class="block text-sm font-medium text-gray-700" for="gitlab-modal-ca">
							CA Bundle (PEM, optional)
						</label>
						<textarea
							id="gitlab-modal-ca"
							bind:value={gitlabCaBundle}
							rows="4"
							placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
							class="mt-1 block w-full rounded border-gray-300 font-mono text-xs shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
							data-testid="gitlab-modal-ca"
						></textarea>
					</div>
				{/if}
			</div>

			<!-- General error -->
			{#if generalError}
				<div
					class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700"
					data-testid="gitlab-modal-error-general"
				>
					{generalError}
				</div>
			{/if}

			<!-- Actions -->
			<div class="flex justify-end gap-3 pt-2">
				<button
					type="button"
					class="rounded border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
					on:click={() => dispatch('cancel')}
					disabled={submitting}
				>
					Cancel
				</button>
				<button
					type="submit"
					class="rounded bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-700 disabled:opacity-50"
					disabled={submitting}
					data-testid="gitlab-modal-submit"
				>
					{submitting ? 'Connecting…' : 'Connect'}
				</button>
			</div>
		</form>
	</div>
</div>

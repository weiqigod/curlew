<script lang="ts">
	import type { PageData } from './$types';
	import { vaultConfigApi } from '$lib/api/vault-config';
	import type { VaultConfigResponse } from '$lib/types/vault-config';
	import type { AuditLogEntry } from '$lib/types/audit-log';
	import { ApiError } from '$lib/types/api-error';

	export let data: PageData;

	let { org, currentConfig } = data;
	let auditEntries: AuditLogEntry[] = data.auditEntries;

	let editorValue: string = currentConfig?.template ?? '';
	let saving = false;
	let deleting = false;
	let showDeleteConfirm = false;
	let showCliSnippet = false;
	let toast: { message: string; kind: 'success' | 'warning' | 'error' } | null = null;
	let suspiciousPaths: string[] = [];

	function showToast(message: string, kind: 'success' | 'warning' | 'error' = 'success') {
		toast = { message, kind };
		setTimeout(() => {
			toast = null;
		}, 5000);
	}

	async function handleSave() {
		saving = true;
		suspiciousPaths = [];
		try {
			const result: VaultConfigResponse = await vaultConfigApi.put(org.id, editorValue);
			editorValue = result.template;
			currentConfig = result;
			if (result.warnings && result.warnings.length > 0) {
				showToast(
					`Updated to version ${result.version} (warnings: ${result.warnings.join(', ')})`,
					'warning'
				);
			} else {
				showToast(`Updated to version ${result.version}`);
			}
		} catch (err) {
			if (err instanceof ApiError && err.status === 422) {
				const paths = (err.details?.offending_paths as string[] | undefined) ?? [];
				suspiciousPaths = paths;
			} else {
				const message = err instanceof Error ? err.message : 'Failed to save';
				showToast(message, 'error');
			}
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		deleting = true;
		try {
			await vaultConfigApi.remove(org.id);
			editorValue = '';
			currentConfig = null;
			showDeleteConfirm = false;
			showToast('Vault configuration deleted');
		} catch (err) {
			const message = err instanceof Error ? err.message : 'Failed to delete';
			showToast(message, 'error');
		} finally {
			deleting = false;
		}
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}

	// The CLI is entirely local: it never fetches this template. Developers save it
	// to a file and point CURLEW_TEAM_CONFIG at it; --env picks the environment whose
	// aliases back the collection's {{secrets.X}} tokens.
	const CLI_SNIPPET = `export CURLEW_TEAM_CONFIG=~/team/curlew-team-config.yaml
curlew run collections/users.yaml --env staging`;
</script>

<div class="mx-auto max-w-4xl px-4 py-8">
	<!-- Header -->
	<div class="mb-6">
		<h1 class="text-2xl font-bold text-gray-900">Vault Configuration</h1>
		<p class="mt-1 text-sm text-gray-600">{org.name} — shared vault template</p>
	</div>

	<!-- Toast notification -->
	{#if toast}
		<div
			class="mb-4 rounded-md px-4 py-3 text-sm font-medium {toast.kind === 'success'
				? 'bg-green-50 text-green-800'
				: toast.kind === 'warning'
					? 'bg-yellow-50 text-yellow-800'
					: 'bg-red-50 text-red-800'}"
			role="alert"
		>
			{toast.message}
		</div>
	{/if}

	<!-- Suspicious-value warning panel -->
	{#if suspiciousPaths.length > 0}
		<div
			class="mb-4 rounded-md border border-red-200 bg-red-50 p-4"
			data-testid="vault-suspicious-warning"
		>
			<h3 class="text-sm font-semibold text-red-800">Suspicious values detected</h3>
			<p class="mt-1 text-sm text-red-700">
				The following fields appear to contain literal secrets. Replace them with provider
				coordinate references (ARN, vault:// URI, etc.).
			</p>
			<ul class="mt-2 list-inside list-disc text-sm font-mono text-red-700">
				{#each suspiciousPaths as path}
					<li>{path}</li>
				{/each}
			</ul>
		</div>
	{/if}

	<!-- YAML editor -->
	<div class="mb-4">
		<label for="vault-editor" class="mb-1 block text-sm font-medium text-gray-700">
			Vault template YAML
		</label>
		<textarea
			id="vault-editor"
			data-testid="vault-editor"
			class="w-full rounded-md border border-gray-300 p-3 font-mono text-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
			rows="25"
			placeholder="team_secrets:&#10;  provider: aws-secrets-manager&#10;  keys:&#10;    api_key: prod/api-key"
			bind:value={editorValue}
		></textarea>
	</div>

	<!-- Action buttons -->
	<div class="mb-8 flex items-center gap-3">
		<button
			data-testid="vault-save-button"
			on:click={handleSave}
			disabled={saving}
			class="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
		>
			{saving ? 'Saving…' : 'Save'}
		</button>

		<button
			data-testid="vault-cli-snippet-button"
			on:click={() => (showCliSnippet = !showCliSnippet)}
			class="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
		>
			Generate CLI snippet
		</button>

		<button
			data-testid="vault-delete-button"
			on:click={() => (showDeleteConfirm = true)}
			disabled={deleting}
			class="ml-auto rounded-md border border-red-300 px-4 py-2 text-sm font-medium text-red-700 hover:bg-red-50 disabled:opacity-50"
		>
			Delete
		</button>
	</div>

	<!-- CLI snippet panel -->
	{#if showCliSnippet}
		<div class="mb-6 rounded-md border border-gray-200 bg-gray-50 p-4">
			<p class="mb-2 text-sm text-gray-700">
				The CLI reads shared vault templates from a local file — it never downloads this
				one. Save the template above on each developer and CI machine, then point
				<code class="rounded bg-gray-100 px-1 py-0.5 font-mono text-xs">CURLEW_TEAM_CONFIG</code>
				at it before running. <code class="rounded bg-gray-100 px-1 py-0.5 font-mono text-xs"
					>--env</code
				>
				selects which environment in the template backs the collection's
				<code class="rounded bg-gray-100 px-1 py-0.5 font-mono text-xs">{'{{secrets.X}}'}</code>
				tokens:
			</p>
			<div class="flex items-center gap-2">
				<code
					data-testid="vault-cli-snippet"
					class="flex-1 overflow-x-auto whitespace-pre rounded bg-gray-100 px-3 py-2 font-mono text-sm"
					>{CLI_SNIPPET}</code
				>
				<button
					on:click={() => navigator.clipboard.writeText(CLI_SNIPPET)}
					class="rounded border border-gray-300 px-3 py-2 text-xs text-gray-600 hover:bg-gray-100"
				>
					Copy
				</button>
			</div>
		</div>
	{/if}

	<!-- Delete confirmation -->
	{#if showDeleteConfirm}
		<div class="mb-6 rounded-md border border-red-200 bg-red-50 p-4">
			<p class="text-sm text-red-800">
				Are you sure you want to delete the vault configuration? This action cannot be undone.
			</p>
			<div class="mt-3 flex gap-2">
				<button
					data-testid="vault-delete-confirm"
					on:click={handleDelete}
					disabled={deleting}
					class="rounded-md bg-red-600 px-3 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
				>
					{deleting ? 'Deleting…' : 'Confirm delete'}
				</button>
				<button
					on:click={() => (showDeleteConfirm = false)}
					class="rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
				>
					Cancel
				</button>
			</div>
		</div>
	{/if}

	<!-- Audit log -->
	<div>
		<h2 class="mb-3 text-lg font-semibold text-gray-900">Audit Log</h2>
		<div data-testid="vault-audit-log">
			{#if auditEntries.length === 0}
				<p class="text-sm text-gray-500">No vault configuration events yet.</p>
			{:else}
				<table class="w-full border-collapse text-sm">
					<thead>
						<tr class="border-b border-gray-200 text-left text-gray-600">
							<th class="pb-2 pr-4">Event</th>
							<th class="pb-2 pr-4">Actor</th>
							<th class="pb-2">When</th>
						</tr>
					</thead>
					<tbody>
						{#each auditEntries as entry}
							<tr class="border-b border-gray-100">
								<td class="py-2 pr-4 font-mono text-xs">{entry.event_type}</td>
								<td class="py-2 pr-4">{entry.user_email ?? entry.user_id ?? '—'}</td>
								<td class="py-2 text-gray-500">{formatDate(entry.created_at)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	</div>
</div>

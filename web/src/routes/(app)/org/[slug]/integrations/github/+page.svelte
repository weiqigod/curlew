<script lang="ts">
	import type { PageData } from './$types';

	export let data: PageData;

	const { org, installation, installationError, lastCheck } = data;
</script>

<svelte:head>
	<title>GitHub Integration — {org.name}</title>
</svelte:head>

<div class="mx-auto max-w-3xl space-y-6 px-4 py-8" data-testid="github-integration-page">
	<h1 class="text-2xl font-semibold">GitHub Integration</h1>
	<p class="text-sm text-gray-500">Organization: {org.name}</p>

	{#if installationError}
		<div class="rounded border border-red-300 bg-red-50 p-4 text-red-700" data-testid="installation-error">
			{installationError}
		</div>
	{:else if installation}
		<div class="rounded border border-green-300 bg-green-50 p-4" data-testid="installation-card">
			<p class="font-medium text-green-800">GitHub App installed</p>
			<p class="text-sm text-green-700">Account: {installation.account_login}</p>
			<p class="text-sm text-green-700">Repos: {installation.repo_set.map((r) => `${r.owner}/${r.name}`).join(', ') || 'all'}</p>
			{#if installation.claimed_at}
				<p class="text-sm text-green-700">Claimed: {new Date(installation.claimed_at).toLocaleString()}</p>
			{/if}
		</div>
	{:else}
		<div class="rounded border border-gray-200 bg-gray-50 p-4" data-testid="installation-not-found">
			<p class="text-gray-600">No GitHub App installation found for this organization.</p>
		</div>
	{/if}

	{#if lastCheck}
		<div class="rounded border border-gray-200 bg-white p-4" data-testid="last-pr-check">
			<h2 class="mb-2 font-medium">Most Recent PR Check</h2>
			<p class="text-sm">Repo: <span data-testid="check-repo">{lastCheck.repo}</span></p>
			<p class="text-sm">PR: <span data-testid="check-pr">#{lastCheck.pr}</span></p>
			<p class="text-sm">State: <span data-testid="check-state">{lastCheck.state}</span></p>
			{#if lastCheck.posted_at}
				<p class="text-sm">
					Posted at: <span data-testid="check-posted-at">{new Date(lastCheck.posted_at).toLocaleString()}</span>
				</p>
			{:else}
				<p class="text-sm text-gray-500" data-testid="check-not-posted">Not yet posted to GitHub</p>
			{/if}
			{#if lastCheck.check_run_id}
				<p class="text-sm">
					Check Run ID: <span data-testid="check-run-id">{lastCheck.check_run_id}</span>
				</p>
			{/if}
		</div>
	{/if}
</div>

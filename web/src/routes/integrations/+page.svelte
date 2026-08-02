<script lang="ts">
	import type { PageData } from './$types';
	export let data: PageData;

	/** True when the install is webhook-first and pending the user's claim. */
	$: isPendingClaim = !!data.installation && data.installation.claimed_at === null;

	/** True when the install is suspended on GitHub's side. */
	$: isSuspended = !!data.installation && data.installation.suspended_at !== null;

	/** True when there is a connected, claimed, non-suspended install. */
	$: isConnected =
		!!data.installation && data.installation.claimed_at !== null && !isSuspended;

	/** Number of repositories covered by the install. */
	$: repoCount = data.installation?.repo_set?.length ?? 0;

	let toastDismissed = false;
</script>

<svelte:head><title>Integrations — Curlew</title></svelte:head>

<div class="mx-auto max-w-3xl px-4 py-8">
	<h1 class="mb-6 text-2xl font-bold text-gray-900">Integrations</h1>

	{#if data.showInstalledToast && !toastDismissed}
		<div
			data-testid="toast-github-connected"
			role="alert"
			class="mb-4 rounded-lg border border-green-300 bg-green-50 px-4 py-3 text-sm font-medium text-green-800"
		>
			<div class="flex items-center justify-between gap-4">
				<span>GitHub connected.</span>
				<button
					type="button"
					aria-label="Dismiss notification"
					class="text-green-700 hover:text-green-900"
					on:click={() => (toastDismissed = true)}>&times;</button
				>
			</div>
		</div>
	{/if}

	<section
		data-testid="github-integration-card"
		class="rounded-lg border border-gray-200 bg-white p-6 shadow-sm"
	>
		<div class="flex items-center justify-between">
			<h2 class="text-lg font-semibold text-gray-900">GitHub Checks</h2>
			{#if isSuspended}
				<span
					data-testid="github-suspended-badge"
					class="inline-flex items-center rounded-full bg-yellow-100 px-2.5 py-0.5 text-xs font-medium text-yellow-800"
				>
					Suspended on GitHub — unsuspend to resume PR checks
				</span>
			{/if}
		</div>

		{#if isConnected && data.installation}
			<p data-testid="github-install-status" class="mt-3 text-sm text-gray-700">
				GitHub connected — {data.installation.account_login}, {repoCount} repos covered
			</p>
		{:else if isSuspended && data.installation}
			<!--
				Claimed + suspended: install exists but is suspended on GitHub's side.
				Do NOT show the Connect button — the user must unsuspend on GitHub first.
			-->
			<p data-testid="github-suspended-description" class="mt-3 text-sm text-gray-700">
				Your GitHub install (<strong>{data.installation.account_login}</strong>) is suspended —
				unsuspend it on GitHub to resume PR checks.
			</p>
		{:else if isPendingClaim && data.isAdmin && data.installation}
			<p class="mt-3 text-sm text-gray-700">
				A GitHub App install for <strong>{data.installation.account_login}</strong>
				is pending your claim.
			</p>
			<button
				type="button"
				data-testid="github-claim-button"
				class="mt-3 inline-flex items-center rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700"
			>
				Claim this install
			</button>
		{:else if data.isAdmin}
			<p class="mt-3 text-sm text-gray-600">
				Connect your GitHub organisation to post Curlew results as PR checks.
			</p>
			{#if data.installUrl}
				<a
					data-testid="github-connect-button"
					href={data.installUrl}
					rel="noopener"
					class="mt-3 inline-flex items-center rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800"
				>
					Connect GitHub
				</a>
				<!--
					Help text — explains the two install paths per
					docs/SPECIFICATION.md:8413–8417 (DoD: "Help/onboarding text in
					the dashboard explains the difference between
					dashboard-initiated and webhook-first install paths").
				-->
				<p data-testid="github-install-help" class="mt-4 text-xs text-gray-500">
					Two ways to connect: <strong>dashboard-initiated</strong> (this button — preferred;
					we link the install to your org automatically) or <strong>webhook-first</strong> (install
					from GitHub directly, then return here to claim it).
				</p>
			{/if}
		{:else}
			<p class="mt-3 text-sm text-gray-600">
				GitHub Checks integration is not yet configured for this organisation.
			</p>
			<p data-testid="github-admin-only-tooltip" class="mt-3 text-xs italic text-gray-500">
				Only owners and admins can connect GitHub.
			</p>
		{/if}

		{#if data.stateError}
			<p class="mt-3 text-xs text-red-600">{data.stateError}</p>
		{/if}
	</section>
</div>

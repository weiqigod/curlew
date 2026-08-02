<script lang="ts">
	import { userDataApi } from '$lib/api/user-data';
	import type { UserExportRequest } from '$lib/types/user-data';
	import { TERMINAL_STATUSES } from '$lib/types/user-data';
	import { ApiError } from '$lib/types/api-error';
	import DeleteAccountPanel from './DeleteAccountPanel.svelte';

	let currentRequest: UserExportRequest | null = null;
	let isRequesting = false;
	let errorMessage: string | null = null;
	let rateLimitRetryAfter: number | null = null;

	const POLL_INTERVAL_MS = 5000;

	async function requestExport() {
		if (isRequesting) return;
		isRequesting = true;
		errorMessage = null;
		rateLimitRetryAfter = null;

		try {
			currentRequest = await userDataApi.createExportRequest();
			if (!TERMINAL_STATUSES.has(currentRequest.status)) {
				schedulePoll();
			}
		} catch (err) {
			if (err instanceof ApiError && err.status === 429) {
				// Extract Retry-After from the error or use a default.
				rateLimitRetryAfter = 86400; // 24h default
				errorMessage =
					'You can request one export per 24 hours. Please try again later.';
			} else {
				errorMessage = 'Failed to request data export. Please try again.';
			}
		} finally {
			isRequesting = false;
		}
	}

	function schedulePoll() {
		setTimeout(async () => {
			if (!currentRequest) return;
			try {
				currentRequest = await userDataApi.getExportRequest(currentRequest.id);
				if (!TERMINAL_STATUSES.has(currentRequest.status)) {
					schedulePoll();
				}
			} catch {
				// Best-effort polling; will not retry on error.
			}
		}, POLL_INTERVAL_MS);
	}

	function statusLabel(status: string): string {
		return (
			{
				queued: 'Queued',
				building: 'Building',
				ready: 'Ready',
				failed: 'Failed',
				expired: 'Expired',
			}[status] ?? status
		);
	}
</script>

<svelte:head>
	<title>Your Data — Curlew</title>
</svelte:head>

<main class="account-data">
	<h1>Your Data</h1>
	<p>
		Request a copy of all the data Curlew holds about you, as a downloadable JSON bundle.
		You can make one request per 24-hour window.
	</p>

	<section class="export-section">
		<h2>Request data export</h2>

		<button
			type="button"
			class="btn-primary"
			data-testid="request-export-button"
			disabled={isRequesting || (currentRequest !== null && !TERMINAL_STATUSES.has(currentRequest.status))}
			on:click={requestExport}
		>
			{isRequesting ? 'Requesting…' : 'Request data export'}
		</button>

		{#if errorMessage}
			<div class="alert alert-error" role="alert">
				{errorMessage}
				{#if rateLimitRetryAfter !== null}
					<small>Rate limit: one export per 24 hours.</small>
				{/if}
			</div>
		{/if}

		{#if currentRequest}
			<div class="request-status" data-status={currentRequest.status}>
				<span class="status-badge status-{currentRequest.status}">
					{statusLabel(currentRequest.status)}
				</span>
				<span class="requested-at">Requested at {new Date(currentRequest.created_at).toLocaleString()}</span>

				{#if currentRequest.status === 'ready' && currentRequest.signed_url}
					<a
						href={currentRequest.signed_url}
						class="btn-download"
						download="curlew-export.json"
					>
						Download bundle
					</a>
					{#if currentRequest.expires_at}
						<small class="expires">
							Download link expires {new Date(currentRequest.expires_at).toLocaleString()}
						</small>
					{/if}
				{/if}

				{#if currentRequest.status === 'failed'}
					<p class="failure-reason">
						Export failed ({currentRequest.failure_reason ?? 'unknown error'}). Please try again.
					</p>
				{/if}
			</div>
		{/if}
	</section>

	<DeleteAccountPanel />

	<footer class="data-footer">
		<p>
			For a full inventory of what data Curlew collects and retains, see the
			<a href="/docs/security/data-inventory.md" target="_blank" rel="noopener noreferrer">
				data inventory document
			</a>.
		</p>
	</footer>
</main>

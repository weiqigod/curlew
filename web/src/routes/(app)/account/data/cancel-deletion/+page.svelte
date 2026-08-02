<script lang="ts">
	import { onMount } from 'svelte';
	import { userDeletionApi } from '$lib/api/user-deletion';
	import type { UserDeletionStatus } from '$lib/types/user-deletion';

	let status: UserDeletionStatus | null = null;
	let isLoading = true;
	let isCancelling = false;
	let cancelled = false;
	let errorMessage: string | null = null;

	onMount(async () => {
		try {
			status = await userDeletionApi.getStatus();
		} catch {
			status = null;
		} finally {
			isLoading = false;
		}
	});

	async function cancelDeletion() {
		if (isCancelling) return;
		isCancelling = true;
		errorMessage = null;
		try {
			await userDeletionApi.cancelDeletion();
			cancelled = true;
			status = null;
		} catch {
			errorMessage = 'Failed to cancel deletion. Please try again.';
		} finally {
			isCancelling = false;
		}
	}
</script>

<svelte:head>
	<title>Cancel Account Deletion — ApiTool</title>
</svelte:head>

<main class="cancel-deletion">
	<h1>Cancel Account Deletion</h1>

	{#if isLoading}
		<p>Loading…</p>

	{:else if cancelled}
		<div class="success" role="status">
			<p>Your account deletion request has been cancelled.</p>
			<a href="/account/data">Back to Your Data</a>
		</div>

	{:else if status?.pending_deletion_at}
		<p>
			Your account deletion is scheduled for
			<strong>
				{status.finalizes_at ? new Date(status.finalizes_at).toLocaleDateString() : 'unknown'}
			</strong>.
		</p>
		<p>You can cancel this request before that date.</p>

		{#if errorMessage}
			<div class="alert alert-error" role="alert">{errorMessage}</div>
		{/if}

		<button
			type="button"
			class="btn-secondary"
			disabled={isCancelling}
			on:click={cancelDeletion}
		>
			{isCancelling ? 'Cancelling…' : 'Cancel deletion'}
		</button>

	{:else}
		<div class="no-pending" role="status">
			<p>No pending account deletion request found.</p>
			<a href="/account/data">Back to Your Data</a>
		</div>
	{/if}
</main>

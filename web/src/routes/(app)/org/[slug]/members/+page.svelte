<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { PageData } from './$types';
	import type { CreateInvitationRequest } from '$lib/types/invitations';
	import { invitationsApi } from '$lib/api/invitations';
	import InviteMemberModal from '$lib/components/members/InviteMemberModal.svelte';
	import MembersTable from '$lib/components/members/MembersTable.svelte';
	import InvitationsTable from '$lib/components/members/InvitationsTable.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';

	export let data: PageData;

	let modalOpen = false;
	let submitting = false;
	let submitError: string | null = null;

	/** Invitation id awaiting cancel confirmation. */
	let pendingCancelId: string | null = null;
	let cancelError: string | null = null;

	function openModal() {
		submitError = null;
		modalOpen = true;
	}

	async function handleInvite(e: CustomEvent<CreateInvitationRequest>) {
		submitting = true;
		submitError = null;
		try {
			await invitationsApi.create(data.org.id, e.detail);
			modalOpen = false;
			await invalidateAll();
		} catch (err) {
			submitError = err instanceof Error ? err.message : 'Failed to send invitation.';
		} finally {
			submitting = false;
		}
	}

	function handleCancel(e: CustomEvent<{ invitationId: string }>) {
		cancelError = null;
		pendingCancelId = e.detail.invitationId;
	}

	async function confirmCancel() {
		if (!pendingCancelId) return;
		const id = pendingCancelId;
		pendingCancelId = null;
		try {
			await invitationsApi.revoke(data.org.id, id);
			await invalidateAll();
		} catch (err) {
			cancelError = err instanceof Error ? err.message : 'Failed to cancel invitation.';
		}
	}

	function dismissCancel() {
		pendingCancelId = null;
	}
</script>

<svelte:head>
	<title>{data.org.name} — Members</title>
</svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 data-testid="members-heading" class="text-2xl font-bold text-gray-900">Members</h1>
		<button
			type="button"
			data-testid="invite-button"
			class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
			on:click={openModal}
		>
			Invite member
		</button>
	</div>

	{#if data.error}
		<ErrorState message={data.error} retryUrl="?reload=1" />
	{:else}
		<div class="space-y-8">
			{#if pendingCancelId}
				<div
					data-testid="cancel-confirm-banner"
					role="alertdialog"
					aria-live="polite"
					class="rounded-md border border-red-300 bg-red-50 px-4 py-3 flex items-center justify-between gap-4"
				>
					<p class="text-sm text-red-800">Cancel this invitation? The invite link will be revoked.</p>
					<div class="flex gap-2">
						<button
							type="button"
							data-testid="cancel-confirm-yes"
							class="rounded bg-red-600 px-3 py-1 text-sm font-medium text-white hover:bg-red-700"
							on:click={confirmCancel}
						>
							Cancel invite
						</button>
						<button
							type="button"
							data-testid="cancel-confirm-no"
							class="rounded border border-gray-300 bg-white px-3 py-1 text-sm font-medium text-gray-700 hover:bg-gray-50"
							on:click={dismissCancel}
						>
							Keep
						</button>
					</div>
				</div>
			{/if}

			{#if cancelError}
				<div
					data-testid="cancel-error-banner"
					role="alert"
					class="rounded-md border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800"
				>
					{cancelError}
				</div>
			{/if}

			<section>
				<h2 class="mb-3 text-lg font-semibold text-gray-700">Active members</h2>
				<MembersTable members={data.members} />
			</section>

			<section>
				<h2 class="mb-3 text-lg font-semibold text-gray-700">Pending invitations</h2>
				<InvitationsTable invitations={data.invitations} on:cancel={handleCancel} />
			</section>
		</div>
	{/if}
</div>

<InviteMemberModal
	open={modalOpen}
	{submitting}
	error={submitError}
	on:submit={handleInvite}
	on:close={() => (modalOpen = false)}
/>

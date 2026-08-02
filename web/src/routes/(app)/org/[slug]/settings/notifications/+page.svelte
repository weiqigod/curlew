<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { PageData } from './$types';
	import type { CreateRuleRequest } from '$lib/types/notifications';
	import { notificationsApi } from '$lib/api/notifications';
	import NotificationRuleModal from '$lib/components/notifications/NotificationRuleModal.svelte';
	import RulesTable from '$lib/components/notifications/RulesTable.svelte';
	import DeliveryLog from '$lib/components/notifications/DeliveryLog.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';

	export let data: PageData;

	let modalOpen = false;
	let submitting = false;
	let submitError: string | null = null;

	/** Rule id awaiting delete confirmation; null when no confirmation is pending. */
	let pendingDeleteId: string | null = null;
	/** Error message shown inline when a delete request fails. */
	let deleteError: string | null = null;

	async function handleSubmit(e: CustomEvent<CreateRuleRequest>) {
		submitting = true;
		submitError = null;
		try {
			await notificationsApi.createRule(data.org.id, e.detail);
			modalOpen = false;
			await invalidateAll();
		} catch (err) {
			submitError = err instanceof Error ? err.message : 'Failed to create rule.';
		} finally {
			submitting = false;
		}
	}

	function handleDelete(e: CustomEvent<{ ruleId: string }>) {
		deleteError = null;
		pendingDeleteId = e.detail.ruleId;
	}

	async function confirmDelete() {
		if (!pendingDeleteId) return;
		const id = pendingDeleteId;
		pendingDeleteId = null;
		try {
			await notificationsApi.deleteRule(data.org.id, id);
			await invalidateAll();
		} catch (err) {
			deleteError = err instanceof Error ? err.message : 'Failed to delete rule.';
		}
	}

	function cancelDelete() {
		pendingDeleteId = null;
	}

	function openModal() {
		submitError = null;
		modalOpen = true;
	}
</script>

<svelte:head>
	<title>{data.org.name} — Notification Settings</title>
</svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Notification Settings</h1>
		<button
			type="button"
			data-testid="add-rule-button"
			class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
			on:click={openModal}
		>
			Add rule
		</button>
	</div>

	{#if data.error}
		<ErrorState message={data.error} retryUrl="?reload=1" />
	{:else}
		<div class="space-y-8">
			{#if pendingDeleteId}
				<div
					data-testid="delete-confirm-banner"
					role="alertdialog"
					aria-live="polite"
					class="rounded-md border border-red-300 bg-red-50 px-4 py-3 flex items-center justify-between gap-4"
				>
					<p class="text-sm text-red-800">Delete this notification rule? This cannot be undone.</p>
					<div class="flex gap-2">
						<button
							type="button"
							data-testid="delete-confirm-yes"
							class="rounded bg-red-600 px-3 py-1 text-sm font-medium text-white hover:bg-red-700"
							on:click={confirmDelete}
						>
							Delete
						</button>
						<button
							type="button"
							data-testid="delete-confirm-no"
							class="rounded border border-gray-300 bg-white px-3 py-1 text-sm font-medium text-gray-700 hover:bg-gray-50"
							on:click={cancelDelete}
						>
							Cancel
						</button>
					</div>
				</div>
			{/if}

			{#if deleteError}
				<div
					data-testid="delete-error-banner"
					role="alert"
					class="rounded-md border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800"
				>
					{deleteError}
				</div>
			{/if}

			<section>
				<h2 class="mb-3 text-lg font-semibold text-gray-700">Notification Rules</h2>
				<RulesTable rules={data.rules} on:delete={handleDelete} />
			</section>

			<section>
				<h2 class="mb-3 text-lg font-semibold text-gray-700">Delivery Log</h2>
				<DeliveryLog deliveries={data.deliveries} />
			</section>
		</div>
	{/if}
</div>

<NotificationRuleModal
	open={modalOpen}
	{submitting}
	error={submitError}
	on:submit={handleSubmit}
	on:close={() => (modalOpen = false)}
/>

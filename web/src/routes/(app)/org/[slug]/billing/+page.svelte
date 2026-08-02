<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { PageData } from './$types';
	import { subscriptionsApi } from '$lib/api/subscriptions';
	import { ApiError } from '$lib/types/api-error';
	import AddSeatsModal from '$lib/components/billing/AddSeatsModal.svelte';
	import EmailVerifiedRequiredModal from '$lib/components/billing/EmailVerifiedRequiredModal.svelte';
	import SeatsUsageBar from '$lib/components/billing/SeatsUsageBar.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';
	import type { ProrationResult } from '$lib/types/subscriptions';

	export let data: PageData;

	/** Pricebook (USD). Backend has no price field; keep it simple. */
	const PRICES: Record<string, Record<string, string>> = {
		team: { month: '$39/mo', year: '$390/yr' }
	};

	function getPrice(tier: string, interval: string): string {
		return PRICES[tier]?.[interval] ?? '—';
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		});
	}

	let modalOpen = false;
	let submitting = false;
	let submitError: string | null = null;
	let proration: ProrationResult | null = null;

	let portalLoading = false;
	let portalError: string | null = null;

	/** Tracks whether the email-verification required modal is open. */
	let verifyModalOpen = false;

	function openModal() {
		submitError = null;
		proration = null;
		modalOpen = true;
	}

	/** Returns true if the error is an email-not-verified 403 problem detail. */
	function isEmailNotVerified(err: unknown): boolean {
		return (
			err instanceof ApiError && err.problemType?.endsWith('/email-not-verified') === true
		);
	}

	async function handleAddSeats(e: CustomEvent<{ seatCount: number }>) {
		if (!data.subscription) return;
		submitting = true;
		submitError = null;
		try {
			const result = await subscriptionsApi.update(data.subscription.id, {
				seat_count: e.detail.seatCount
			});
			proration = result.proration;
			await invalidateAll();
		} catch (err) {
			if (isEmailNotVerified(err)) {
				verifyModalOpen = true;
			} else {
				submitError = err instanceof Error ? err.message : 'Failed to update seats.';
			}
		} finally {
			submitting = false;
		}
	}

	async function handleManageBilling(event: MouseEvent) {
		event.preventDefault();
		portalLoading = true;
		portalError = null;
		try {
			const { portal_url } = await subscriptionsApi.portal(
				data.org.id,
				window.location.href
			);
			window.location.href = portal_url;
		} catch (err) {
			if (isEmailNotVerified(err)) {
				verifyModalOpen = true;
			} else {
				portalError = err instanceof Error ? err.message : 'Failed to open billing portal.';
			}
			portalLoading = false;
		}
	}
</script>

<svelte:head>
	<title>{data.org.name} — Billing</title>
</svelte:head>

<div class="mx-auto max-w-4xl px-4 py-8">
	<h1 data-testid="billing-heading" class="mb-6 text-2xl font-bold text-gray-900">Billing</h1>

	{#if data.error}
		<ErrorState message={data.error} retryUrl="?reload=1" />
	{:else if !data.subscription}
		<!-- No subscription yet -->
		<div
			data-testid="no-subscription-card"
			class="rounded-lg border border-gray-200 bg-white p-6 shadow-sm"
		>
			<p class="text-gray-600">No active subscription found. Upgrade to unlock team features.</p>
		</div>
	{:else}
		{@const sub = data.subscription}
		<div
			data-testid="subscription-card"
			class="rounded-lg border border-gray-200 bg-white p-6 shadow-sm space-y-4"
		>
			<div class="flex items-start justify-between">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider text-gray-400">Plan</p>
					<p data-testid="subscription-tier" class="text-xl font-semibold capitalize text-gray-900">
						{sub.tier}
					</p>
				</div>
				<div class="text-right">
					<p class="text-xs font-medium uppercase tracking-wider text-gray-400">Price</p>
					<p data-testid="subscription-price" class="text-xl font-semibold text-gray-900">
						{getPrice(sub.tier, sub.interval)}
					</p>
				</div>
			</div>

			<div>
				<p class="mb-1 text-xs font-medium uppercase tracking-wider text-gray-400">
					Next renewal
				</p>
				<p data-testid="subscription-renewal" class="text-sm text-gray-700">
					{formatDate(sub.current_period_end)}
				</p>
			</div>

			<div>
				<p class="mb-2 text-xs font-medium uppercase tracking-wider text-gray-400">Seat usage</p>
				<SeatsUsageBar seatCount={sub.seat_count} seatLimit={sub.seat_limit} />
			</div>

			{#if portalError}
				<p role="alert" class="text-sm text-red-600">{portalError}</p>
			{/if}

			<div class="flex gap-3 pt-2">
				<button
					type="button"
					data-testid="add-seats-button"
					class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
					on:click={openModal}
				>
					Add seats
				</button>
				<!-- Portal link: rel=noopener per a11y DoD -->
				<a
					data-testid="manage-billing-button"
					href="#billing-portal"
					rel="noopener noreferrer"
					class="inline-flex items-center rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 {portalLoading
						? 'pointer-events-none opacity-50'
						: ''}"
					on:click={handleManageBilling}
				>
					{portalLoading ? 'Opening…' : 'Manage billing'}
				</a>
			</div>
		</div>
	{/if}
</div>

<AddSeatsModal
	open={modalOpen}
	{submitting}
	error={submitError}
	currentSeatCount={data.subscription?.seat_count ?? 0}
	{proration}
	on:submit={handleAddSeats}
	on:close={() => {
		modalOpen = false;
		proration = null;
	}}
/>

<EmailVerifiedRequiredModal
	open={verifyModalOpen}
	on:close={() => {
		verifyModalOpen = false;
	}}
/>

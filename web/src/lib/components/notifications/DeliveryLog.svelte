<script lang="ts">
	import type { NotificationDelivery, NotificationDeliveryStatus } from '$lib/types/notifications';

	export let deliveries: NotificationDelivery[] = [];

	function badgeClass(status: NotificationDeliveryStatus): string {
		switch (status) {
			case 'delivered':
				return 'bg-green-100 text-green-800';
			case 'failed':
				return 'bg-red-100 text-red-800';
			case 'pending':
			default:
				return 'bg-gray-100 text-gray-800';
		}
	}
</script>

<div data-testid="delivery-log">
	{#if deliveries.length === 0}
		<p class="text-sm text-gray-500 py-4">No deliveries yet.</p>
	{:else}
		<table class="w-full text-sm text-left">
			<thead class="text-xs text-gray-500 uppercase border-b border-gray-200">
				<tr>
					<th class="py-2 px-3">Status</th>
					<th class="py-2 px-3">Channel</th>
					<th class="py-2 px-3">Attempts</th>
					<th class="py-2 px-3">Error</th>
					<th class="py-2 px-3">Attempted at</th>
				</tr>
			</thead>
			<tbody>
				{#each deliveries as delivery (delivery.id)}
					<tr
						data-testid="delivery-row-{delivery.id}"
						class="border-b border-gray-100 hover:bg-gray-50"
					>
						<td class="py-2 px-3">
							<span
								data-testid="delivery-status-{delivery.id}"
								class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium {badgeClass(delivery.status)}"
							>
								{delivery.status}
							</span>
						</td>
						<td class="py-2 px-3 capitalize">{delivery.channel}</td>
						<td class="py-2 px-3">{delivery.attempt_count}</td>
						<td class="py-2 px-3 text-gray-500 text-xs">
							{delivery.error_message ?? '—'}
						</td>
						<td class="py-2 px-3 text-gray-500">
							{new Date(delivery.attempted_at).toLocaleString()}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>

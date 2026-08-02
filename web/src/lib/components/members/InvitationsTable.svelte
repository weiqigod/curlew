<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { Invitation } from '$lib/types/invitations';

	export let invitations: Invitation[];

	const dispatch = createEventDispatcher<{ cancel: { invitationId: string } }>();

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	function handleCancel(id: string) {
		dispatch('cancel', { invitationId: id });
	}
</script>

<div data-testid="invitations-table" class="overflow-hidden rounded-lg border border-gray-200">
	{#if invitations.length === 0}
		<p class="px-4 py-6 text-center text-sm text-gray-500">No pending invitations.</p>
	{:else}
		<table class="w-full text-sm">
			<thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
				<tr>
					<th class="px-4 py-3">Email</th>
					<th class="px-4 py-3">Role</th>
					<th class="px-4 py-3">Expires</th>
					<th class="px-4 py-3">Actions</th>
				</tr>
			</thead>
			<tbody class="divide-y divide-gray-100 bg-white">
				{#each invitations as invitation}
					<tr data-testid="invitation-row-{invitation.id}" class="hover:bg-gray-50">
						<td class="px-4 py-3 text-gray-700">{invitation.email}</td>
						<td class="px-4 py-3">
							<span
								class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium
								{invitation.role === 'admin'
									? 'bg-blue-100 text-blue-700'
									: 'bg-gray-100 text-gray-700'}"
							>
								{invitation.role}
							</span>
						</td>
						<td class="px-4 py-3 text-gray-500">{formatDate(invitation.expires_at)}</td>
						<td class="px-4 py-3">
							<button
								type="button"
								data-testid="invitation-cancel-{invitation.id}"
								class="text-xs font-medium text-red-600 hover:text-red-800"
								on:click={() => handleCancel(invitation.id)}
							>
								Cancel
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>

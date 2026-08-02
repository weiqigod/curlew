<script lang="ts">
	import type { Member } from '$lib/types/members';

	export let members: Member[];

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}
</script>

<div data-testid="members-table" class="overflow-hidden rounded-lg border border-gray-200">
	{#if members.length === 0}
		<p class="px-4 py-6 text-center text-sm text-gray-500">No members found.</p>
	{:else}
		<table class="w-full text-sm">
			<thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
				<tr>
					<th class="px-4 py-3">User ID</th>
					<th class="px-4 py-3">Role</th>
					<th class="px-4 py-3">Joined</th>
				</tr>
			</thead>
			<tbody class="divide-y divide-gray-100 bg-white">
				{#each members as member}
					<tr data-testid="member-row-{member.user_id}" class="hover:bg-gray-50">
						<td class="px-4 py-3 font-mono text-gray-700"
							>{member.user_id.slice(0, 8)}…</td
						>
						<td class="px-4 py-3">
							<span
								class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium
								{member.role === 'owner'
									? 'bg-purple-100 text-purple-700'
									: member.role === 'admin'
										? 'bg-blue-100 text-blue-700'
										: 'bg-gray-100 text-gray-700'}"
							>
								{member.role}
							</span>
						</td>
						<td class="px-4 py-3 text-gray-500">{formatDate(member.joined_at)}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>

<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { NotificationRule } from '$lib/types/notifications';

	export let rules: NotificationRule[] = [];

	const dispatch = createEventDispatcher<{
		delete: { ruleId: string };
	}>();

	function handleDelete(ruleId: string) {
		dispatch('delete', { ruleId });
	}

	function formatEvents(on: string[]): string {
		return on.join(', ');
	}
</script>

<div data-testid="rules-table">
	{#if rules.length === 0}
		<p class="text-sm text-gray-500 py-4">
			No rules yet. Click <strong>Add rule</strong> to get started.
		</p>
	{:else}
		<table class="w-full text-sm text-left">
			<thead class="text-xs text-gray-500 uppercase border-b border-gray-200">
				<tr>
					<th class="py-2 px-3">Channel</th>
					<th class="py-2 px-3">Target</th>
					<th class="py-2 px-3">Triggers on</th>
					<th class="py-2 px-3">Created</th>
					<th class="py-2 px-3 sr-only">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each rules as rule (rule.id)}
					<tr
						data-testid="rule-row-{rule.id}"
						class="border-b border-gray-100 hover:bg-gray-50"
					>
						<td class="py-2 px-3 capitalize">{rule.channel}</td>
						<td class="py-2 px-3 font-mono text-xs break-all">{rule.target}</td>
						<td class="py-2 px-3">{formatEvents(rule.on)}</td>
						<td class="py-2 px-3 text-gray-500">
							{new Date(rule.created_at).toLocaleDateString()}
						</td>
						<td class="py-2 px-3">
							<button
								type="button"
								data-testid="rule-delete-{rule.id}"
								class="text-red-600 hover:text-red-800 text-xs font-medium"
								on:click={() => handleDelete(rule.id)}
							>
								Delete
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>

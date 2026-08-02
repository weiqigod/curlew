<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';
	import { AUDIT_EVENT_TYPES } from '$lib/types/audit-log';

	export let data: PageData;

	function fmtDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}
	function target(e: { target_type: string | null; target_id: string | null }): string {
		return [e.target_type, e.target_id].filter(Boolean).join(':') || '—';
	}

	async function applyFilter(patch: Record<string, string | null>) {
		const u = new URL($page.url);
		for (const [k, v] of Object.entries(patch)) {
			if (v === null || v === '') u.searchParams.delete(k);
			else u.searchParams.set(k, v);
		}
		u.searchParams.delete('page'); // reset to page 1 on filter change
		await goto(u.pathname + u.search, { keepFocus: true, noScroll: true });
	}

	function onEventTypeChange(e: Event) {
		const select = e.currentTarget as HTMLSelectElement;
		applyFilter({ event_type: select.value || null });
	}

	function pickRange(preset: '24h' | '7d' | '30d' | 'all') {
		if (preset === 'all') {
			applyFilter({ from: null, to: null });
			return;
		}
		const now = new Date();
		const ms =
			preset === '24h' ? 86_400_000 : preset === '7d' ? 7 * 86_400_000 : 30 * 86_400_000;
		const from = new Date(now.getTime() - ms).toISOString();
		const to = now.toISOString();
		applyFilter({ from, to });
	}

	function goToPage(n: number) {
		const u = new URL($page.url);
		u.searchParams.set('page', String(n));
		goto(u.pathname + u.search, { keepFocus: true, noScroll: true });
	}

	$: exportHref = (() => {
		const u = new URL($page.url);
		u.pathname = `/org/${data.org.slug}/audit-log/export`;
		u.searchParams.delete('page');
		return u.pathname + u.search;
	})();
</script>

<svelte:head><title>{data.org.name} — Audit Log</title></svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8">
	<div class="mb-4 flex items-center justify-between">
		<h1 data-testid="audit-log-heading" class="text-2xl font-bold text-gray-900">Audit Log</h1>
		<a
			href={exportHref}
			data-testid="audit-log-export"
			download
			class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
		>Export CSV</a>
	</div>

	<!-- filters -->
	<div class="mb-4 flex flex-wrap items-center gap-3" data-testid="audit-log-filters">
		<label class="text-sm">
			Event type
			<select
				data-testid="audit-log-event-filter"
				value={data.filter.event_type ?? ''}
				on:change={onEventTypeChange}
			>
				<option value="">All</option>
				{#each AUDIT_EVENT_TYPES as t}<option value={t}>{t}</option>{/each}
			</select>
		</label>
		<div class="flex gap-1" data-testid="audit-log-range-picker">
			<button type="button" data-testid="range-24h" on:click={() => pickRange('24h')}>24h</button>
			<button type="button" data-testid="range-7d" on:click={() => pickRange('7d')}>7d</button>
			<button type="button" data-testid="range-30d" on:click={() => pickRange('30d')}>30d</button>
			<button type="button" data-testid="range-all" on:click={() => pickRange('all')}>All</button>
		</div>
	</div>

	{#if data.error}
		<ErrorState message={data.error} retryUrl={$page.url.pathname + $page.url.search} />
	{:else if data.entries.length === 0}
		<EmptyState message="No audit-log entries for the selected filter." />
	{:else}
		<table data-testid="audit-log-table" class="min-w-full divide-y divide-gray-200">
			<thead class="bg-gray-50">
				<tr>
					<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
						>Event type</th
					>
					<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
						>User</th
					>
					<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
						>Target</th
					>
					<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
						>Status</th
					>
					<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
						>Timestamp</th
					>
					<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
						>IP</th
					>
				</tr>
			</thead>
			<tbody class="divide-y divide-gray-200 bg-white">
				{#each data.entries as entry}
					<tr class="hover:bg-gray-50">
						<td class="whitespace-nowrap px-4 py-3 text-sm font-medium text-gray-900"
							>{entry.event_type}</td
						>
						<td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500"
							>{entry.user_email ?? entry.user_id ?? '—'}</td
						>
						<td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500">{target(entry)}</td>
						<td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
							{#if entry.success}
								<span class="text-green-700">success</span>
							{:else}
								<span class="text-red-700"
									>failure{entry.failure_reason ? ` (${entry.failure_reason})` : ''}</span
								>
							{/if}
						</td>
						<td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500"
							>{fmtDate(entry.created_at)}</td
						>
						<td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500"
							>{entry.ip_address ?? '—'}</td
						>
					</tr>
				{/each}
			</tbody>
		</table>

		<div class="mt-4 flex items-center justify-between" data-testid="audit-log-pagination">
			<span class="text-sm text-gray-500">Page {data.page} of {data.totalPages}</span>
			<div class="flex gap-2">
				<button
					type="button"
					data-testid="audit-log-prev"
					disabled={data.page <= 1}
					on:click={() => goToPage(data.page - 1)}
					class="rounded border border-gray-300 bg-white px-3 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
					>Prev</button
				>
				<button
					type="button"
					data-testid="audit-log-next"
					disabled={data.page >= data.totalPages}
					on:click={() => goToPage(data.page + 1)}
					class="rounded border border-gray-300 bg-white px-3 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
					>Next</button
				>
			</div>
		</div>
	{/if}
</div>

<script lang="ts">
	import type { PageData } from './$types';
	import SchedulesTable from '$lib/components/schedules/SchedulesTable.svelte';
	import CreateScheduleModal from '$lib/components/schedules/CreateScheduleModal.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';
	import { page } from '$app/stores';
	import { schedulesApi } from '$lib/api/schedules';
	import type { Schedule, ScheduledRun, CreateScheduleRequest } from '$lib/types/schedules';

	export let data: PageData;

	let schedules: Schedule[] = data.schedules;
	let lastRuns: Array<ScheduledRun | null> = data.lastRuns;
	let showCreateModal = false;
	let runNowFeedback: Record<string, string> = {};
	let createModalError = '';
	let createModalRef: { resetSubmitting: () => void } | undefined;

	async function handleRunNow(name: string) {
		try {
			const result = await schedulesApi.runNow(data.org.id, name, {
				token: $page.data.accessToken
			});
			runNowFeedback = { ...runNowFeedback, [name]: result.status };
		} catch {
			runNowFeedback = { ...runNowFeedback, [name]: 'error' };
		}
	}

	async function handleCreateSchedule(event: CustomEvent<CreateScheduleRequest>) {
		createModalError = '';
		try {
			const created = await schedulesApi.create(data.org.id, event.detail, {
				token: $page.data.accessToken
			});
			schedules = [...schedules, created];
			lastRuns = [...lastRuns, null];
			showCreateModal = false;
		} catch (e) {
			createModalError = e instanceof Error ? e.message : 'Failed to create schedule.';
			createModalRef?.resetSubmitting();
		}
	}
</script>

<svelte:head>
	<title>{data.org.name} — Schedules</title>
</svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Schedules</h1>
		<button
			type="button"
			on:click={() => (showCreateModal = true)}
			class="rounded bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700"
			data-testid="new-schedule-btn"
		>
			New schedule
		</button>
	</div>

	{#if data.error}
		<ErrorState message={data.error} retryUrl={$page.url.pathname} />
	{:else if schedules.length === 0}
		<EmptyState message="No schedules yet — create your first schedule to automate test runs." />
	{:else}
		<SchedulesTable
			{schedules}
			{lastRuns}
			slug={data.org.slug}
			on:runNow={(e) => handleRunNow(e.detail)}
		/>

		{#if Object.keys(runNowFeedback).length > 0}
			<div class="mt-4 space-y-1">
				{#each Object.entries(runNowFeedback) as [name, status]}
					<p class="text-sm text-gray-600">
						<strong>{name}</strong>: run
						<span
							class="rounded-full px-1.5 py-0.5 text-xs font-medium
								{status === 'queued' ? 'bg-yellow-100 text-yellow-800' : 'bg-red-100 text-red-800'}"
							data-testid="run-now-feedback"
						>
							{status}
						</span>
					</p>
				{/each}
			</div>
		{/if}
	{/if}
</div>

{#if showCreateModal}
	<CreateScheduleModal
		bind:this={createModalRef}
		apiError={createModalError}
		on:submit={handleCreateSchedule}
		on:cancel={() => { showCreateModal = false; createModalError = ''; }}
	/>
{/if}

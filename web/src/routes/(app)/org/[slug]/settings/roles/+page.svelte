<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { PageData } from './$types';
	import { rolesApi } from '$lib/api/roles';
	import { ApiError } from '$lib/types/api-error';
	import type { RoleView } from '$lib/types/roles';
	import Toast from '$lib/components/ui/Toast.svelte';
	import ErrorState from '$lib/components/ui/ErrorState.svelte';
	import PermissionPicker from '$lib/components/roles/PermissionPicker.svelte';

	export let data: PageData;

	type ModalMode = { kind: 'create' } | { kind: 'edit'; role: RoleView };

	let modal: ModalMode | null = null;
	let formName = '';
	let formPerms: Set<string> = new Set();
	let submitting = false;
	let formError: string | null = null;
	let toast: { open: boolean; message: string; variant: 'success' | 'error' } = {
		open: false,
		message: '',
		variant: 'success'
	};

	function openCreate() {
		modal = { kind: 'create' };
		formName = '';
		formPerms = new Set();
		formError = null;
	}

	function openEdit(role: RoleView) {
		modal = { kind: 'edit', role };
		formName = role.name;
		formPerms = new Set(role.permissions);
		formError = null;
	}

	function closeModal() {
		modal = null;
	}

	async function submitForm() {
		if (!modal) return;
		submitting = true;
		formError = null;
		try {
			if (modal.kind === 'create') {
				await rolesApi.create(data.org.id, {
					name: formName.trim(),
					permissions: [...formPerms]
				});
				toast = { open: true, message: 'Role created', variant: 'success' };
			} else {
				await rolesApi.update(data.org.id, modal.role.id, {
					name: formName.trim(),
					permissions: [...formPerms]
				});
				toast = { open: true, message: 'Role updated', variant: 'success' };
			}
			modal = null;
			await invalidateAll();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Request failed.';
		} finally {
			submitting = false;
		}
	}

	async function confirmDelete(role: RoleView) {
		if (!confirm(`Delete "${role.name}"?`)) return;
		try {
			await rolesApi.remove(data.org.id, role.id);
			toast = { open: true, message: 'Role deleted', variant: 'success' };
			await invalidateAll();
		} catch (e) {
			if (e instanceof ApiError && e.code === 'role_in_use') {
				const n = Number(
					(e.details as { member_count?: unknown } | undefined)?.member_count ?? 0
				);
				toast = {
					open: true,
					message: `Role is assigned to ${n} members`,
					variant: 'error'
				};
			} else {
				toast = {
					open: true,
					message: e instanceof Error ? e.message : 'Delete failed',
					variant: 'error'
				};
			}
		}
	}
</script>

<svelte:head>
	<title>{data.org.name} — Roles</title>
</svelte:head>

<div class="mx-auto max-w-5xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 data-testid="roles-heading" class="text-2xl font-bold text-gray-900">Roles</h1>
		<button
			data-testid="roles-create-button"
			type="button"
			class="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
			on:click={openCreate}
		>
			Create role
		</button>
	</div>

	{#if data.error}
		<ErrorState message={data.error} retryUrl="?reload=1" />
	{:else}
		<table data-testid="roles-table" class="min-w-full divide-y divide-gray-200">
			<thead class="bg-gray-50">
				<tr>
					<th
						scope="col"
						class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
						>Name</th
					>
					<th
						scope="col"
						class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
						>Permissions</th
					>
					<th
						scope="col"
						class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
						>Members</th
					>
					<th scope="col" class="relative px-6 py-3"
						><span class="sr-only">Actions</span></th
					>
				</tr>
			</thead>
			<tbody class="divide-y divide-gray-200 bg-white">
				{#each data.roles as role}
					<tr data-testid="role-row-{role.id}">
						<td class="whitespace-nowrap px-6 py-4 text-sm text-gray-900">
							{role.name}
							{#if role.is_builtin}
								<span
									class="ml-2 rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-600"
								>
									Built-in
								</span>
							{/if}
						</td>
						<td class="whitespace-nowrap px-6 py-4 text-sm text-gray-500">
							{role.permissions.length}
						</td>
						<td class="whitespace-nowrap px-6 py-4 text-sm text-gray-500">
							{data.memberCounts[role.id] ?? 0}
						</td>
						<td class="whitespace-nowrap px-6 py-4 text-right text-sm">
							{#if !role.is_builtin}
								<button
									data-testid="role-edit-{role.id}"
									type="button"
									class="mr-3 text-indigo-600 hover:text-indigo-900"
									on:click={() => openEdit(role)}
								>
									Edit
								</button>
								<button
									data-testid="role-delete-{role.id}"
									type="button"
									class="text-red-600 hover:text-red-900"
									on:click={() => confirmDelete(role)}
								>
									Delete
								</button>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>

{#if modal}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<!-- svelte-ignore a11y-no-static-element-interactions -->
	<div
		class="fixed inset-0 z-40 flex items-center justify-center bg-black bg-opacity-40"
		on:click|self={closeModal}
	>
		<div
			data-testid="role-modal"
			class="relative z-50 w-full max-w-2xl rounded-lg bg-white p-6 shadow-xl"
		>
			<h2 class="mb-4 text-lg font-semibold text-gray-900">
				{modal.kind === 'create' ? 'Create role' : 'Edit role'}
			</h2>

			<div class="mb-4">
				<label for="role-name-input" class="block text-sm font-medium text-gray-700">
					Role name
				</label>
				<input
					id="role-name-input"
					data-testid="role-name-input"
					type="text"
					bind:value={formName}
					class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-indigo-500"
					placeholder="e.g. qa-lead"
				/>
			</div>

			<div class="mb-4 max-h-80 overflow-y-auto">
				<p class="mb-2 text-sm font-medium text-gray-700">Permissions</p>
				<PermissionPicker bind:selected={formPerms} />
			</div>

			{#if formError}
				<p class="mb-3 text-sm text-red-600" role="alert">{formError}</p>
			{/if}

			<div class="flex justify-end gap-3">
				<button
					type="button"
					class="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
					on:click={closeModal}
				>
					Cancel
				</button>
				<button
					data-testid="role-submit"
					type="button"
					disabled={submitting}
					class="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
					on:click={submitForm}
				>
					{submitting ? 'Saving…' : 'Save'}
				</button>
			</div>
		</div>
	</div>
{/if}

<Toast bind:open={toast.open} message={toast.message} variant={toast.variant} />

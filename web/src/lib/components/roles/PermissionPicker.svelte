<script lang="ts">
	import { PERMISSION_CATALOGUE, PERMISSION_CATEGORIES, type PermissionCategory } from '$lib/types/roles';

	/** Currently selected permission keys. Two-way bound. */
	export let selected: Set<string>;
	/** When true, all checkboxes are disabled (e.g. for built-in role preview). */
	export let disabled = false;

	function toggle(key: string, on: boolean) {
		const next = new Set(selected);
		if (on) {
			next.add(key);
		} else {
			next.delete(key);
		}
		selected = next;
	}

	function permsForCategory(cat: PermissionCategory) {
		return PERMISSION_CATALOGUE.filter((p) => p.category === cat);
	}
</script>

<div data-testid="permission-picker">
	{#each PERMISSION_CATEGORIES as category}
		<fieldset
			class="mb-4"
			data-testid="perm-section-{category.toLowerCase().replace(/[^a-z]+/g, '-')}"
		>
			<legend class="text-sm font-semibold text-gray-700">{category}</legend>
			<div class="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
				{#each permsForCategory(category) as p}
					<label class="flex items-start gap-2">
						<input
							type="checkbox"
							data-testid="perm-{p.key}"
							{disabled}
							checked={selected.has(p.key)}
							on:change={(e) => toggle(p.key, e.currentTarget.checked)}
						/>
						<span>
							<span class="block text-sm text-gray-900">{p.label}</span>
							<code class="block text-xs text-gray-500">{p.key}</code>
						</span>
					</label>
				{/each}
			</div>
		</fieldset>
	{/each}
</div>

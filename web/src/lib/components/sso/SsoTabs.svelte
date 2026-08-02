<script context="module" lang="ts">
	/** A tab definition. */
	export interface Tab {
		id: string;
		label: string;
	}
</script>

<script lang="ts">
	/** The list of tabs to render. */
	export let tabs: Tab[];
	/** The id of the currently active tab. Bindable. */
	export let active: string = tabs[0]?.id ?? '';

	function select(id: string) {
		active = id;
	}

	function handleKeydown(e: KeyboardEvent, id: string) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			select(id);
		}
	}
</script>

<div>
	<div role="tablist" aria-label="SSO configuration tabs" class="flex border-b border-gray-200">
		{#each tabs as tab (tab.id)}
			<button
				role="tab"
				id="tab-{tab.id}"
				data-testid="tab-{tab.id}"
				aria-selected={active === tab.id}
				aria-controls="tabpanel-{tab.id}"
				tabindex={active === tab.id ? 0 : -1}
				class="px-4 py-2 text-sm font-medium transition-colors
					{active === tab.id
					? 'border-b-2 border-blue-600 text-blue-600'
					: 'text-gray-500 hover:text-gray-700'}"
				on:click={() => select(tab.id)}
				on:keydown={(e) => handleKeydown(e, tab.id)}
			>
				{tab.label}
			</button>
		{/each}
	</div>

	<div
		role="tabpanel"
		id="tabpanel-saml"
		data-testid="tabpanel-saml"
		aria-labelledby="tab-saml"
		hidden={active !== 'saml'}
		class="pt-6"
	>
		<slot name="saml" />
	</div>

	<div
		role="tabpanel"
		id="tabpanel-oidc"
		data-testid="tabpanel-oidc"
		aria-labelledby="tab-oidc"
		hidden={active !== 'oidc'}
		class="pt-6"
	>
		<slot name="oidc" />
	</div>
</div>

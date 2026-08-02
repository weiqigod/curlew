<script lang="ts">
	import { feature } from '$lib/content/taxonomy';

	export let id: string;
	/** When true, render a link to the filtered examples index. */
	export let link = false;

	$: f = feature(id);
	$: classes = `inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${f.badge}`;
</script>

{#if link}
	<a
		href={`/examples#feature=${f.id}`}
		class="{classes} transition hover:opacity-80"
		title={f.blurb}
	>
		{f.label}
	</a>
{:else}
	<span class={classes} title={f.blurb}>{f.label}</span>
{/if}

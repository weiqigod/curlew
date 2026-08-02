<script lang="ts">
	import type { PageData } from './$types';
	import { getComponent } from '$lib/content';
	import ScenarioLayout from '$lib/components/ScenarioLayout.svelte';

	export let data: PageData;

	$: Component = getComponent(data.meta.slug);
</script>

<svelte:head>
	<title>{data.meta.title} — Curlew cookbook</title>
	<meta name="description" content={data.meta.intent} />
</svelte:head>

<ScenarioLayout meta={data.meta} prev={data.prev} next={data.next}>
	<svelte:component this={Component} />
</ScenarioLayout>

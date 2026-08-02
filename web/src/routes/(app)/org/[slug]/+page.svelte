<script lang="ts">
	import type { PageData } from './$types';
	export let data: PageData;
</script>

<div class="mx-auto max-w-6xl px-4 py-8">
	<h1 class="text-2xl font-bold text-gray-900">{data.org?.name ?? 'Organization'}</h1>
	<p class="mt-2 text-gray-600">Select a section from the navigation above.</p>
</div>

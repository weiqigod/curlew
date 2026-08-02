<script lang="ts">
	import { page } from '$app/stores';
</script>

<svelte:head>
	<title>{$page.status} — ApiTool cookbook</title>
</svelte:head>

<div class="mx-auto flex max-w-xl flex-col items-center px-5 py-32 text-center">
	<p class="text-6xl font-bold text-brand-500">{$page.status}</p>
	<h1 class="mt-4 text-2xl font-bold text-[var(--text)]">
		{$page.error?.message ?? 'Something went wrong'}
	</h1>
	<a
		href="/examples"
		class="mt-8 rounded-lg bg-brand-600 px-5 py-2.5 text-sm font-semibold text-white transition hover:bg-brand-500"
	>
		Back to the cookbook
	</a>
</div>

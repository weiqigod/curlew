<script lang="ts">
	import { page } from '$app/stores';
	import ThemeToggle from './ThemeToggle.svelte';

	const links = [
		{ href: '/examples', label: 'Examples' },
		{ href: '/features', label: 'Features' }
	];

	$: pathname = $page.url.pathname;
	const active = (href: string, path: string) => path === href || path.startsWith(href + '/');
</script>

<header
	class="sticky top-0 z-30 border-b border-[color:var(--border)] bg-[var(--bg)]/85 backdrop-blur"
>
	<nav class="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4 px-5">
		<a href="/" class="flex items-center gap-2.5 font-semibold text-[var(--text)]">
			<img src="/favicon.svg" alt="" width="26" height="26" class="rounded-md" />
			<span>ApiTool</span>
			<span class="hidden text-xs font-normal text-[var(--text-faint)] sm:inline">cookbook</span>
		</a>

		<div class="flex items-center gap-1 sm:gap-2">
			{#each links as l (l.href)}
				<a
					href={l.href}
					class="rounded-lg px-3 py-1.5 text-sm font-medium transition {active(l.href, pathname)
						? 'text-brand-500'
						: 'text-[var(--text-soft)] hover:text-[var(--text)]'}"
				>
					{l.label}
				</a>
			{/each}
			<div class="ml-1">
				<ThemeToggle />
			</div>
		</div>
	</nav>
</header>

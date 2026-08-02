<script lang="ts">
	const year = 2026;
</script>

<footer class="mt-20 border-t border-[color:var(--border)]">
	<div
		class="mx-auto flex max-w-6xl flex-col gap-4 px-5 py-10 text-sm text-[var(--text-soft)] sm:flex-row sm:items-center sm:justify-between"
	>
		<div class="flex items-center gap-2.5">
			<img src="/favicon.svg" alt="" width="22" height="22" class="rounded" />
			<span>Curlew — file-based API testing for people who live in the terminal.</span>
		</div>
		<div class="flex items-center gap-5">
			<a href="/examples" class="transition hover:text-[var(--text)]">Examples</a>
			<a href="/features" class="transition hover:text-[var(--text)]">Features</a>
			<span class="text-[var(--text-faint)]">© {year}</span>
		</div>
	</div>
</footer>

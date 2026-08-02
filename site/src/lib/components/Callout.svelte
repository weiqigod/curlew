<script lang="ts">
	// Note / tip / warning / output disclaimer box for use inside scenario bodies.
	export let type: 'note' | 'tip' | 'warn' | 'output' = 'note';
	export let title = '';

	const STYLES: Record<string, { label: string; cls: string }> = {
		note: {
			label: 'Note',
			cls: 'border-sky-400/60 bg-sky-50 text-sky-950 dark:border-sky-500/40 dark:bg-sky-950/30 dark:text-sky-100'
		},
		tip: {
			label: 'Tip',
			cls: 'border-emerald-400/60 bg-emerald-50 text-emerald-950 dark:border-emerald-500/40 dark:bg-emerald-950/30 dark:text-emerald-100'
		},
		warn: {
			label: 'Watch out',
			cls: 'border-amber-400/60 bg-amber-50 text-amber-950 dark:border-amber-500/40 dark:bg-amber-950/30 dark:text-amber-100'
		},
		output: {
			label: 'Representative output',
			cls: 'border-zinc-300 bg-zinc-50 text-zinc-700 dark:border-zinc-700 dark:bg-zinc-900/40 dark:text-zinc-300'
		}
	};

	$: style = STYLES[type] ?? STYLES.note;
	$: heading = title || style.label;
</script>

<div class="not-prose my-5 rounded-lg border-l-4 px-4 py-3 text-sm leading-relaxed {style.cls}">
	<p class="mb-1 text-xs font-bold uppercase tracking-wide opacity-80">{heading}</p>
	<div class="[&>:last-child]:mb-0 [&>p]:mb-2">
		<slot />
	</div>
</div>

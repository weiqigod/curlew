<script lang="ts">
	import { CronExpressionParser } from 'cron-parser';

	/** Standard 5-field cron expression to preview. */
	export let cron: string;
	/** IANA timezone identifier (e.g. "Europe/Stockholm"). */
	export let timezone: string;

	let firings: string[] = [];
	let parseError = '';

	$: {
		try {
			const it = CronExpressionParser.parse(cron, { tz: timezone });
			firings = Array.from({ length: 5 }, () => it.next().toDate().toISOString());
			parseError = '';
		} catch (e) {
			firings = [];
			parseError = e instanceof Error ? e.message : 'invalid cron expression';
		}
	}
</script>

{#if parseError}
	<p data-testid="cron-preview-error" class="text-sm text-red-600">{parseError}</p>
{:else if firings.length > 0}
	<ul data-testid="cron-preview" class="mt-1 space-y-0.5 text-sm text-gray-600">
		{#each firings as f}
			<li>{f}</li>
		{/each}
	</ul>
{/if}

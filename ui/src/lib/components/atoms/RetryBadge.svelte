<script lang="ts">
  // `↻2` chip when retry_count > 0 (§10.5.3).
  export let count: number;
  $: tip = `${count} ${count === 1 ? 'retry' : 'retries'} — see Timing tab`;
</script>

{#if count > 0}
  <span class="at-chip" title={tip} aria-label={tip}>↻{count}</span>
{/if}

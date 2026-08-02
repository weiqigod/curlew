<script lang="ts">
  // Headers tab (§10.6.3.2): .at-kv grid, multi-value headers one row per
  // value, [REDACTED] → Redacted chip, copy-all "k: v" lines.
  import Redacted from '../atoms/Redacted.svelte';
  import CopyBtn from '../atoms/CopyBtn.svelte';

  /** Response headers (multi-valued) or request headers (single-valued). */
  export let headers: Record<string, string[] | string>;

  $: rows = Object.entries(headers).flatMap(([k, v]) =>
    Array.isArray(v) ? v.map((value) => [k, value] as const) : [[k, v] as const],
  );
  $: copyText = rows.map(([k, v]) => `${k}: ${v}`).join('\n');
</script>

<div class="ht">
  <div class="bar">
    <span class="count at-mono">{rows.length} header{rows.length === 1 ? '' : 's'}</span>
    <span class="sp"></span>
    <CopyBtn text={copyText} label="all headers" />
  </div>
  {#if rows.length > 0}
    <div class="at-kv grid">
      {#each rows as [k, v], i (k + i)}
        <div class="k">{k}</div>
        <div>
          {#if v === '[REDACTED]'}<Redacted />{:else}{v}{/if}
        </div>
      {/each}
    </div>
  {:else}
    <div class="empty at-mono">no headers</div>
  {/if}
</div>

<style>
  .ht {
    overflow: auto;
    flex: 1;
    padding: var(--pad);
  }
  .bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-bottom: var(--pad-sm);
  }
  .count {
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
  .sp {
    flex: 1;
  }
  .grid {
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    overflow: hidden;
    background: var(--bg1);
  }
  .empty {
    font-size: var(--fs-sm);
    color: var(--fg3);
  }
</style>

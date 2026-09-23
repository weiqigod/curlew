<script lang="ts">
  // Source snippet with line numbers + the request's own line highlighted
  // (§10.6.3.5).
  import type { SourceRef } from '../../types/run';

  export let source: SourceRef;
</script>

<div class="snip at-mono">
  {#each source.snippet ?? [] as line, i}
    {@const n = source.snippet_start_line + i}
    <div class="line" class:hl={n === source.line}>
      <span class="num">{n}</span>
      <span class="txt">{line}</span>
    </div>
  {/each}
</div>

<style>
  .snip {
    font-size: var(--fs-sm);
    line-height: 1.55;
    background: var(--bg1);
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    padding: 8px 0;
    max-width: 760px;
    overflow: auto;
  }
  .line {
    display: flex;
  }
  .line.hl {
    background: var(--acc-dim);
  }
  .num {
    width: 44px;
    flex: none;
    text-align: right;
    padding-right: 14px;
    color: var(--fg3);
    user-select: none;
  }
  .line.hl .num {
    color: var(--fg1);
  }
  .txt {
    white-space: pre;
    color: var(--fg1);
  }
</style>

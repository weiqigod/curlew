<script lang="ts">
  // "WAVE <i+1> · n req" group header with right-aligned wave duration
  // after completion (§10.6.2.3).
  import { fmtMs } from '../../format';

  export let index: number;
  export let count: number;
  export let durationMs: number | null = null;
</script>

<div class="at-group-h" role="presentation">
  <span>Wave {index + 1}</span>
  <span class="n at-mono">{count} req</span>
  <span class="sp"></span>
  {#if durationMs !== null}
    <span class="dur at-mono">{fmtMs(durationMs)}</span>
  {/if}
</div>

<style>
  .n {
    font-size: 10px;
    color: var(--fg3);
    text-transform: none;
    letter-spacing: 0;
  }
  .sp {
    flex: 1;
  }
  .dur {
    font-size: 10px;
    color: var(--fg2);
    text-transform: none;
    letter-spacing: 0;
  }
</style>

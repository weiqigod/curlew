<script lang="ts">
  // Run header card (§10.6.4.4): time, branch chip, outcome dot+word, code,
  // duration; the B side shows the signed duration delta + bad-outcome message.
  import { clockTime, fmtMs, relTime } from '../../format';
  import type { CompareSide } from '../../types/compare';
  import type { RunInfo } from '../../types/run';
  import Chip from '../atoms/Chip.svelte';
  import Code from '../atoms/Code.svelte';
  import Dot from '../atoms/Dot.svelte';
  import Icon from '../atoms/Icon.svelte';

  export let run: RunInfo;
  export let side: CompareSide | null;
  export let deltaMs: number | null = null;
  /** timing_us entries exceeding 10% change, pre-computed by the parent. */
  export let timingChips: string[] = [];

  $: created = run.meta?.created_at ?? '';
  $: branch = run.meta?.git?.branch ?? null;
  $: outcome = side?.outcome ?? null;
  $: dotState = (
    outcome === 'passed' || outcome === 'failed' || outcome === 'skipped' || outcome === 'error'
      ? outcome
      : 'pending'
  ) as 'passed' | 'failed' | 'skipped' | 'error' | 'pending';
  $: badMsg =
    side !== null
      ? (side.fail_message ?? (side.error !== null ? `${side.error.category}: ${side.error.message}` : null))
      : null;
</script>

<div class="card">
  <div class="top">
    {#if created !== ''}
      <span class="time at-mono">{clockTime(created)}</span>
      <span class="day">{relTime(created)}</span>
    {/if}
    {#if branch !== null}
      <Chip title="git branch"><Icon name="branch" size={12} />{branch}</Chip>
    {/if}
    <span class="sp"></span>
    {#if outcome !== null}
      <span
        class="outcome"
        class:ok={outcome === 'passed'}
        class:err={outcome === 'failed'}
        class:err2={outcome === 'error'}
      >
        <Dot state={dotState} />
        {outcome}
      </span>
    {:else}
      <span class="outcome miss">not in this run</span>
    {/if}
  </div>
  {#if side !== null}
    <div class="meta at-mono">
      {#if side.status_code !== undefined}<Code code={side.status_code} />{/if}
      <span class="dotsep">·</span>
      <span>{fmtMs(side.duration_ms)}</span>
      {#if deltaMs !== null}
        <span class="delta at-mono">{deltaMs > 0 ? '+' : ''}{deltaMs} ms</span>
      {/if}
    </div>
    {#if timingChips.length > 0}
      <div class="tchips">
        {#each timingChips as c}
          <span class="at-chip">{c}</span>
        {/each}
      </div>
    {/if}
    {#if badMsg !== null}
      <div class="badmsg at-mono">{badMsg}</div>
    {/if}
  {/if}
</div>

<style>
  .card {
    flex: 1;
    min-width: 0;
    background: var(--bg2);
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    padding: 9px 12px;
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .time {
    font-size: var(--fs-sm);
    font-weight: 600;
  }
  .day {
    font-size: var(--fs-xs);
    color: var(--fg3);
    white-space: nowrap;
  }
  .sp {
    flex: 1;
  }
  .outcome {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .outcome.ok {
    color: var(--ok);
  }
  .outcome.err {
    color: var(--err);
  }
  .outcome.err2 {
    color: var(--err2);
  }
  .outcome.miss {
    color: var(--fg3);
  }
  .meta {
    font-size: var(--fs-xs);
    color: var(--fg1);
    display: flex;
    gap: 8px;
    align-items: baseline;
    white-space: nowrap;
  }
  .dotsep {
    color: var(--fg3);
  }
  .delta {
    color: var(--fg2);
    background: var(--bg3);
    border: 1px solid var(--bd1);
    border-radius: 3px;
    padding: 0 5px;
    white-space: nowrap;
  }
  .tchips {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
  }
  .badmsg {
    font-size: var(--fs-xs);
    color: var(--err);
    overflow-wrap: anywhere;
  }
</style>

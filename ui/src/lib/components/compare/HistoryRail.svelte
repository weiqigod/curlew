<script lang="ts">
  // History rail (§10.6.4.2): GET /runs with infinite scroll, A/B selection
  // (newest = B default, second click on A swaps), counts n✓ n✗ n! n–.
  import { createEventDispatcher } from 'svelte';
  import { clockTime, fmtMs, relTime } from '../../format';
  import { historyTotal, loadHistory, runs } from '../../stores/history';
  import { meta } from '../../stores/meta';
  import Icon from '../atoms/Icon.svelte';

  export let base: string | null;
  export let target: string | null;

  const dispatch = createEventDispatcher<{ pick: string }>();

  let loadingMore = false;

  async function onScroll(e: Event): Promise<void> {
    const el = e.currentTarget as HTMLElement;
    if (loadingMore) return;
    if ($runs.length >= $historyTotal) return;
    if (el.scrollTop + el.clientHeight < el.scrollHeight - 80) return;
    loadingMore = true;
    try {
      await loadHistory(50, $runs.length);
    } finally {
      loadingMore = false;
    }
  }

  function running(exitStatus: string): boolean {
    return exitStatus === '' || exitStatus === 'running';
  }
</script>

<div class="rail">
  <div class="railhead">Recent runs</div>
  <div class="railsub">pick A and B to diff two runs</div>
  <div class="raillist" on:scroll={onScroll}>
    {#each $runs as r (r.run_id)}
      {@const role = r.run_id === target ? 'B' : r.run_id === base ? 'A' : null}
      {@const inProgress = running(r.exit_status)}
      <button
        class="runrow"
        class:picked={role !== null}
        class:dim={inProgress}
        disabled={inProgress}
        title={inProgress ? 'in progress' : undefined}
        on:click={() => dispatch('pick', r.run_id)}
      >
        <div class="r1">
          <span class="time at-mono">{clockTime(r.created_at)}</span>
          <span class="day">{relTime(r.created_at)}</span>
          <span class="sp"></span>
          {#if role !== null}
            <span class="role">{role}</span>
            {#if role === 'A'}<span class="swaphint" title="click again to swap A↔B"><Icon name="swap" size={11} /></span>{/if}
          {/if}
        </div>
        {#if r.git?.branch != null}
          <div class="branch at-mono">
            <Icon name="branch" size={12} />
            <span class="bname">{r.git.branch}</span>
          </div>
        {/if}
        <div class="counts at-mono">
          <span class="ok">{r.summary.passed}✓</span>
          {#if r.summary.failed > 0}<span class="err">{r.summary.failed}✗</span>{/if}
          {#if r.summary.error > 0}<span class="err2">{r.summary.error}!</span>{/if}
          {#if r.summary.skipped > 0}<span class="dim2">{r.summary.skipped}–</span>{/if}
          <span class="dim2">· {fmtMs(r.summary.duration_ms)}</span>
          {#if inProgress}<span class="dim2">· in progress</span>{/if}
        </div>
      </button>
    {:else}
      <div class="emptystate">
        no recorded runs yet — runs are saved automatically (last {$meta?.history.max_runs ?? 50})
      </div>
    {/each}
  </div>
</div>

<style>
  .rail {
    width: 224px;
    flex: none;
    border-right: 1px solid var(--bd0);
    background: var(--bg1);
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
  .railhead {
    padding: 9px 12px 5px;
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg2);
  }
  .railsub {
    padding: 0 12px 7px;
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
  .raillist {
    flex: 1;
    overflow-y: auto;
    padding: 6px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .runrow {
    all: unset;
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 7px 9px;
    border-radius: var(--rad);
    cursor: pointer;
    border: 1px solid transparent;
    box-sizing: border-box;
  }
  .runrow:hover:not(.picked):not(:disabled) {
    background: var(--bg2);
  }
  .runrow.picked {
    background: var(--acc-dim);
    border-color: var(--bd2);
  }
  .runrow.dim {
    opacity: 0.55;
    cursor: default;
  }
  .r1 {
    display: flex;
    align-items: center;
    gap: 7px;
    width: 100%;
  }
  .time {
    font-size: var(--fs-sm);
    color: var(--fg0);
  }
  .day {
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
  .sp {
    flex: 1;
  }
  .role {
    font-size: 9px;
    font-weight: 700;
    letter-spacing: 0.08em;
    color: var(--acc-fg);
    background: var(--acc);
    border-radius: 3px;
    padding: 1px 5px;
  }
  .swaphint {
    color: var(--fg3);
    display: flex;
  }
  .branch {
    font-size: var(--fs-xs);
    color: var(--fg2);
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
  }
  .bname {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .counts {
    font-size: var(--fs-xs);
    display: flex;
    gap: 8px;
    white-space: nowrap;
  }
  .ok {
    color: var(--ok);
  }
  .err {
    color: var(--err);
  }
  .err2 {
    color: var(--err2);
  }
  .dim2 {
    color: var(--fg3);
  }
  .emptystate {
    padding: 12px 9px;
    font-size: var(--fs-xs);
    color: var(--fg3);
    line-height: 1.5;
  }
</style>

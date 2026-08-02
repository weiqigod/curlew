<script lang="ts">
  // Summary strip (§10.6.2.2). Pure-props component (tested per §13.2); the
  // RunView wires it to the stores.
  import { fmtMs } from '../../format';
  import type { SummaryView } from '../../stores/run';
  import Icon from '../atoms/Icon.svelte';

  export let summary: SummaryView;
  /** True while the focused run is streaming. */
  export let running = false;
  /** Live elapsed ms (running) — terminal states use summary.duration_ms. */
  export let elapsedMs: number | null = null;
  /** Env the run actually used (run meta), echoed right-aligned. */
  export let envName: string | null = null;
  /** Past-run chip (#/runs/<id>): relative time; null when current. */
  export let pastRunRelative: string | null = null;
  /** Batch runs omit the wave affordances (§10.6.2.2). */
  export let batch = false;
  export let onClosePastRun: (() => void) | null = null;

  $: total = Math.max(summary.total, 1);
  $: seg = (n: number) => `${(n / total) * 100}%`;
  $: done = summary.completed;

  $: durationText = running
    ? `${((elapsedMs ?? 0) / 1000).toFixed(1)}s elapsed`
    : summary.duration_ms !== undefined
      ? `${fmtMs(summary.duration_ms)} total`
      : '';

  $: waveInfo =
    !running && !batch && summary.parallel === true && summary.wave_count !== undefined
      ? `· ${summary.wave_count} waves` +
        (summary.max_parallelism !== undefined ? ` · max ${summary.max_parallelism}∥` : '')
      : '';

  // aria-live: polite, throttled to one announcement per 2 s while running;
  // the terminal announcement is assertive (§10.8.2).
  let politeText = '';
  let assertiveText = '';
  let lastAnnounce = 0;
  $: if (running) {
    const now = Date.now();
    if (now - lastAnnounce >= 2000) {
      lastAnnounce = now;
      politeText = `${done} of ${summary.total} done${summary.failed > 0 ? `, ${summary.failed} failed` : ''}`;
    }
  }
  let wasRunning = false;
  $: {
    if (running) {
      wasRunning = true;
    } else if (wasRunning) {
      wasRunning = false;
      const parts = [`${summary.passed} passed`, `${summary.failed} failed`];
      if (summary.error > 0) parts.push(`${summary.error} error`);
      if (summary.skipped > 0) parts.push(`${summary.skipped} skipped`);
      assertiveText = `Run finished: ${parts.join(', ')}`;
    }
  }
</script>

<div class="strip">
  <div class="row">
    {#if pastRunRelative !== null}
      <span class="at-chip pastchip">
        viewing past run · {pastRunRelative}
        {#if onClosePastRun !== null}
          <button class="chipx" aria-label="back to current run" on:click={onClosePastRun}>
            <Icon name="x" size={11} />
          </button>
        {/if}
      </span>
    {/if}
    <span class="item" aria-label="{summary.passed} passed">
      <span class="n at-mono" class:ok={summary.passed > 0}>{summary.passed}</span>
      <span class="lbl">passed</span>
    </span>
    <span class="item" aria-label="{summary.failed} failed">
      <span class="n at-mono" class:err={summary.failed > 0}>{summary.failed}</span>
      <span class="lbl">failed</span>
    </span>
    {#if summary.error > 0}
      <span class="item" aria-label="{summary.error} error">
        <span class="n at-mono err2">{summary.error}</span>
        <span class="lbl">error</span>
      </span>
    {/if}
    <span class="item" aria-label="{summary.skipped} skipped">
      <span class="n at-mono" class:neutral={summary.skipped > 0}>{summary.skipped}</span>
      <span class="lbl">skipped</span>
    </span>
    {#if running}
      <span class="item" aria-label="{summary.running} running">
        <span class="n at-mono warn">{summary.running}</span>
        <span class="lbl">running</span>
      </span>
    {/if}
    {#if durationText !== ''}
      <span class="dotsep">·</span>
      <span class="dur at-mono">{durationText}</span>
    {/if}
    <span class="sp"></span>
    <span class="right at-mono">
      {done}/{summary.total}
      {running ? '· streaming' : '· run finished'}
      {#if waveInfo !== ''}{waveInfo}{/if}
      {#if envName !== null}· env: {envName}{/if}
    </span>
  </div>
  <div class="bar" role="progressbar" aria-valuemin={0} aria-valuemax={summary.total} aria-valuenow={done}>
    <div class="segp" style="width:{seg(summary.passed)}"></div>
    <div class="segf" style="width:{seg(summary.failed)}"></div>
    <div class="sege" style="width:{seg(summary.error)}"></div>
    <div class="segs" style="width:{seg(summary.skipped)}"></div>
    <div class="segr" style="width:{seg(summary.running)}"></div>
  </div>
  <span class="vh" aria-live="polite">{politeText}</span>
  <span class="vh" aria-live="assertive" role="status">{assertiveText}</span>
</div>

<style>
  .strip {
    flex: none;
    padding: var(--pad) var(--pad) 0;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 14px;
    padding-bottom: var(--pad-sm);
    flex-wrap: wrap;
  }
  .pastchip {
    gap: 6px;
  }
  .chipx {
    all: unset;
    cursor: pointer;
    display: flex;
    color: var(--fg2);
  }
  .item {
    display: inline-flex;
    align-items: baseline;
    gap: 5px;
  }
  .n {
    font-weight: 600;
    color: var(--fg3);
  }
  .n.ok {
    color: var(--ok);
  }
  .n.err {
    color: var(--err);
  }
  .n.err2 {
    color: var(--err2);
  }
  .n.warn {
    color: var(--warn);
  }
  .n.neutral {
    color: var(--fg0);
  }
  .lbl {
    color: var(--fg2);
    font-size: var(--fs-sm);
  }
  .dotsep {
    color: var(--fg3);
  }
  .dur {
    font-size: var(--fs-sm);
    color: var(--fg1);
    white-space: nowrap;
  }
  .sp {
    flex: 1;
  }
  .right {
    font-size: var(--fs-xs);
    color: var(--fg3);
    white-space: nowrap;
  }
  .bar {
    height: 3px;
    border-radius: 2px;
    background: var(--bg3);
    display: flex;
    overflow: hidden;
  }
  .bar > div {
    transition: width 0.3s;
  }
  .segp {
    background: var(--ok);
  }
  .segf {
    background: var(--err);
  }
  .sege {
    background: var(--err2);
  }
  .segs {
    background: var(--fg3);
  }
  .segr {
    background: var(--warn);
    opacity: 0.7;
  }
  .vh {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
</style>

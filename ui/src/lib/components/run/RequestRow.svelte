<script lang="ts">
  // Compact-list row (§10.6.2.3) — exact anatomy: Dot · PhaseBadge · Method ·
  // name · collection basename · RetryBadge · status-dependent middle ·
  // duration + Code. Clickable only when terminal (skipped included).
  import { createEventDispatcher } from 'svelte';
  import type { LiveRequest } from '../../event-reducer';
  import { fmtMs } from '../../format';
  import Code from '../atoms/Code.svelte';
  import Dot from '../atoms/Dot.svelte';
  import Method from '../atoms/Method.svelte';
  import PhaseBadge from '../atoms/PhaseBadge.svelte';
  import RetryBadge from '../atoms/RetryBadge.svelte';

  export let row: LiveRequest;
  /** Show the phase badge (mixed contexts like filtered views, §10.6.2.3). */
  export let showPhase = false;
  export let showCollection = true;
  /** Live elapsed for running rows (derived from at_ms by the parent). */
  export let liveElapsedMs: number | null = null;
  /** Row-cursor highlight (j/k). */
  export let cursor = false;

  const dispatch = createEventDispatcher<{ open: string }>();

  $: terminal =
    row.status === 'passed' ||
    row.status === 'failed' ||
    row.status === 'skipped' ||
    row.status === 'error';
  $: pending = row.status === 'pending';
  $: name =
    row.iteration !== undefined
      ? `${row.iteration.base_name} ${row.iteration.index + 1}/${row.iteration.total}`
      : row.name;
  $: basename = row.source_file.split('/').pop() ?? row.source_file;

  function open(): void {
    if (terminal) dispatch('open', row.request_id);
  }
</script>

<div
  class="rr"
  class:pending
  class:clickable={terminal}
  class:cursor
  class:inset={row.iteration !== undefined}
  id={`row-${row.request_id}`}
  role="option"
  aria-selected={cursor}
  tabindex="-1"
  on:click={open}
  on:keydown={(e) => e.key === 'Enter' && open()}
>
  <Dot state={row.status} />
  {#if showPhase}<PhaseBadge phase={row.phase} />{/if}
  <Method m={row.method} />
  <span class="name">{name}</span>
  {#if showCollection}
    <span class="coll at-mono">{basename}</span>
  {/if}
  <RetryBadge count={row.retry_count} />

  <span class="mid at-mono">
    {#if row.status === 'failed' && row.fail_message !== undefined}
      <span class="err">{row.fail_message}</span>
    {:else if row.status === 'error' && row.error !== undefined}
      <span class="err2">{row.error.category}: {row.error.message.split('\n')[0]}</span>
    {:else if row.status === 'skipped'}
      <span class="skip">{row.skip_reason ?? 'skipped'}</span>
    {:else if row.status === 'running' && liveElapsedMs !== null}
      <span class="warn">{fmtMs(liveElapsedMs)}</span>
    {/if}
  </span>

  <span class="meta at-mono">
    {#if (row.status === 'passed' || row.status === 'failed') && row.duration_ms !== undefined}
      <span class="dur">{fmtMs(row.duration_ms)}</span>
      {#if row.status_code !== undefined}<Code code={row.status_code} />{/if}
    {:else if row.status === 'error'}
      {#if row.duration_ms !== undefined}<span class="dur">{fmtMs(row.duration_ms)}</span>{/if}
      {#if row.status_code !== undefined}<Code code={row.status_code} />{/if}
    {/if}
  </span>
</div>

<style>
  .rr {
    display: flex;
    align-items: center;
    gap: 10px;
    height: var(--row-h);
    padding: 0 10px;
    border-bottom: 1px solid var(--bd0);
    cursor: default;
    position: relative;
  }
  .rr.clickable {
    cursor: pointer;
  }
  .rr.clickable:hover {
    background: var(--bg2);
  }
  .rr.pending {
    opacity: 0.55;
  }
  .rr.cursor {
    background: var(--bg2);
    box-shadow: inset 1px 0 0 var(--acc);
  }
  .rr.inset {
    box-shadow: inset 2px 0 0 var(--bd2);
    padding-left: 18px;
  }
  .rr.inset.cursor {
    box-shadow:
      inset 2px 0 0 var(--bd2),
      inset 3px 0 0 var(--acc);
  }
  .name {
    font-size: var(--fs-sm);
    white-space: nowrap;
    color: var(--fg0);
  }
  .coll {
    font-size: var(--fs-xs);
    color: var(--fg3);
    white-space: nowrap;
  }
  .mid {
    flex: 1;
    min-width: 0;
    font-size: var(--fs-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mid > span {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .err {
    color: var(--err);
  }
  .err2 {
    color: var(--err2);
  }
  .skip {
    color: var(--fg3);
  }
  .warn {
    color: var(--warn);
  }
  .meta {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: var(--fs-xs);
    white-space: nowrap;
    animation: at-fadein 0.25s;
  }
  .dur {
    color: var(--fg2);
  }
</style>

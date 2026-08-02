<script lang="ts">
  // Card for columns/lanes layouts (§10.6.2.4): 4-outcome treatment, verbatim
  // skip reasons, iteration/retry chips under the name line.
  import { createEventDispatcher } from 'svelte';
  import type { LiveRequest } from '../../event-reducer';
  import { fmtMs } from '../../format';
  import Code from '../atoms/Code.svelte';
  import Dot from '../atoms/Dot.svelte';
  import Method from '../atoms/Method.svelte';
  import RetryBadge from '../atoms/RetryBadge.svelte';

  export let row: LiveRequest;
  export let liveElapsedMs: number | null = null;

  const dispatch = createEventDispatcher<{ open: string }>();

  $: terminal =
    row.status === 'passed' ||
    row.status === 'failed' ||
    row.status === 'skipped' ||
    row.status === 'error';
  $: dim = row.status === 'pending';
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
  class="card"
  class:dim
  class:clickable={terminal}
  class:running={row.status === 'running'}
  role="button"
  tabindex="-1"
  on:click={open}
  on:keydown={(e) => e.key === 'Enter' && open()}
>
  <div class="top">
    <Dot state={row.status} />
    <span class="name">{name}</span>
    <Method m={row.method} />
  </div>
  <div class="file at-mono">{basename}</div>
  {#if row.iteration !== undefined || row.retry_count > 0}
    <div class="chips">
      {#if row.iteration !== undefined}
        <span class="at-chip">iter {row.iteration.index + 1}/{row.iteration.total}</span>
      {/if}
      <RetryBadge count={row.retry_count} />
    </div>
  {/if}
  {#if row.status === 'passed' || row.status === 'failed' || row.status === 'error'}
    <div class="meta at-mono">
      {#if row.duration_ms !== undefined}<span>{fmtMs(row.duration_ms)}</span>{/if}
      {#if row.status_code !== undefined}
        <span class="dotsep">·</span>
        <Code code={row.status_code} />
      {/if}
    </div>
  {/if}
  {#if row.status === 'running'}
    <div class="runtxt at-mono">
      {liveElapsedMs !== null ? fmtMs(liveElapsedMs) : 'running…'}
    </div>
  {/if}
  {#if row.status === 'skipped'}
    <div class="skiptxt at-mono">{row.skip_reason ?? 'skipped'}</div>
  {/if}
  {#if row.status === 'failed' && row.fail_message !== undefined}
    <div class="msgbox fail at-mono">{row.fail_message}</div>
  {/if}
  {#if row.status === 'error' && row.error !== undefined}
    <div class="msgbox err2 at-mono">{row.error.category}: {row.error.message}</div>
  {/if}
</div>

<style>
  .card {
    background: var(--bg2);
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    padding: var(--pad-sm) 10px;
    cursor: default;
    display: flex;
    flex-direction: column;
    gap: 4px;
    transition:
      opacity 0.25s,
      border-color 0.25s;
  }
  .card.clickable {
    cursor: pointer;
  }
  .card.clickable:hover,
  .card.running {
    border-color: var(--bd2);
  }
  .card.dim {
    opacity: 0.55;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .name {
    font-size: var(--fs-sm);
    font-weight: 500;
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .file {
    font-size: var(--fs-xs);
    color: var(--fg3);
    padding-left: 16px;
  }
  .chips {
    display: flex;
    gap: 4px;
    padding-left: 16px;
  }
  .meta {
    display: flex;
    gap: 8px;
    padding-left: 16px;
    font-size: var(--fs-xs);
    color: var(--fg2);
    white-space: nowrap;
    animation: at-fadein 0.25s;
  }
  .dotsep {
    color: var(--fg3);
  }
  .runtxt {
    padding-left: 16px;
    font-size: var(--fs-xs);
    color: var(--warn);
  }
  .skiptxt {
    padding-left: 16px;
    font-size: var(--fs-xs);
    color: var(--fg3);
    overflow-wrap: anywhere;
  }
  .msgbox {
    margin: 2px 0 1px 16px;
    padding: 4px 7px;
    font-size: var(--fs-xs);
    line-height: 1.45;
    border-radius: 3px;
    animation: at-fadein 0.25s;
    overflow-wrap: anywhere;
  }
  .msgbox.fail {
    color: var(--err);
    background: color-mix(in srgb, var(--err) 9%, transparent);
    border: 1px solid color-mix(in srgb, var(--err) 25%, transparent);
  }
  .msgbox.err2 {
    color: var(--err2);
    background: color-mix(in srgb, var(--err2) 9%, transparent);
    border: 1px solid color-mix(in srgb, var(--err2) 25%, transparent);
  }
</style>

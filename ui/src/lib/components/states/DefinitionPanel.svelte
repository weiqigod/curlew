<script lang="ts">
  // Request workspace: the definition and run control stay above inline results.
  import { cancelActiveRun, startRun } from '../../controller';
  import { selectedEnv } from '../../stores/environments';
  import { focusedRequests } from '../../stores/focused-run';
  import { meta } from '../../stores/meta';
  import { runMeta, runState } from '../../stores/run';
  import { tree } from '../../stores/tree';
  import Method from '../atoms/Method.svelte';
  import Icon from '../atoms/Icon.svelte';
  import OpenInEditor from '../atoms/OpenInEditor.svelte';
  import TemplateUrl from '../atoms/TemplateUrl.svelte';
  import Inspector from '../inspector/Inspector.svelte';
  import RunErrorBanner from '../run/RunErrorBanner.svelte';

  export let path: string;
  export let slug: string;

  $: collection = ($tree?.collections ?? []).find((c) => c.path === path);
  $: request = collection?.requests.find((r) => r.slug === slug);

  $: busy = $runState === 'starting' || $runState === 'running' || $runState === 'cancelling';

  $: ownsRun = $runMeta.params?.collection === path &&
    (request?.phase === 'setup' || $runMeta.params.selection === null || $runMeta.params.selection.includes(request?.name ?? ''));
  $: resultRows = $focusedRequests.filter((row) =>
    (ownsRun || row.source_file === path) && row.phase === request?.phase &&
    (row.iteration?.base_slug ?? row.slug) === slug &&
    row.status !== 'pending' && row.status !== 'running',
  );
  let chosenResult = '';
  $: resultRunId = $runMeta.run_id;
  $: stepRows = ownsRun ? $focusedRequests : resultRows;
  $: chosenRow = stepRows.find((row) => `${resultRunId}/${slug}/${row.request_id}` === chosenResult);
  $: resultRow = resultRows[0];
  $: setupFailure = ownsRun ? $focusedRequests.find((row) =>
    row.phase === 'setup' && (row.status === 'failed' || row.status === 'error'),
  ) : undefined;
  $: displayedRow = chosenRow ?? (resultRow?.status !== 'skipped' && resultRow !== undefined ? resultRow : setupFailure ?? resultRow);
  $: runningName = ownsRun ? $focusedRequests.find((row) => row.status === 'running')?.name : undefined;

  function runThis(): void {
    if (busy || request === undefined || request.phase === 'teardown') return;
    void startRun({
      collection: path,
      env: $selectedEnv ?? $meta?.project.default_env ?? '',
      parallel: false,
      mode: request.phase === 'setup' ? 'setup' : 'selection',
      selection: [request.name],
      rerun_of: null,
    }, { navigate: false });
  }
</script>

{#if request !== undefined && collection !== undefined}
  <div class="def">
    <div class="request-head">
      <div class="head">
        <Method m={request.method} />
        <span class="name">{request.name}</span>
        {#if request.data_driven}<span class="badge at-mono">data-driven</span>{/if}
        {#if request.required}<span class="badge at-mono">required</span>{/if}
        <span class="phase at-mono">{request.phase}</span>
      </div>

      <div class="row">
        <span class="label at-mono">url</span>
        <TemplateUrl url={request.url} />
      </div>

      <div class="row">
        <span class="label at-mono">source</span>
        <span class="src at-mono">{collection.path}:{request.source_line}</span>
        <OpenInEditor file={collection.path} line={request.source_line} compact />
      </div>

      <div class="actions">
        {#if request.phase === 'main' || request.phase === 'setup'}
          <button class="at-btn" disabled={busy} on:click={runThis}>
            <Icon name="play" size={13} /> Run this request
          </button>
        {/if}
        {#if ownsRun && busy}
          <button class="at-btn ghost" disabled={$runState === 'cancelling'} on:click={() => void cancelActiveRun()}>
            Cancel
          </button>
        {/if}
        {#if ownsRun}
          <span class="run-status at-mono" role="status">
            {busy ? `${$runState}: ${runningName ?? request.name}` : $runState}
            · {$runMeta.params?.env}
          </span>
        {/if}
      </div>
    </div>
    {#if ownsRun && $runMeta.error !== null}
      <RunErrorBanner error={$runMeta.error} />
    {/if}
    {#if stepRows.length > 0}
      <div class="steps" role="group" aria-label="Run steps">
        {#each stepRows as row (row.request_id)}
          <button
            class="step"
            aria-pressed={displayedRow?.request_id === row.request_id}
            disabled={row.status === 'pending' || row.status === 'running'}
            on:click={() => chosenResult = `${resultRunId}/${slug}/${row.request_id}`}
          >
            <span class="step-phase at-mono">{row.phase}</span>
            <span class="step-name">{row.name}</span>
            <span class="at-mono">{row.status}</span>
            <span class="at-mono">{row.status_code ?? ''}</span>
          </button>
        {/each}
      </div>
    {/if}
    <div class="response">
      {#if displayedRow !== undefined && resultRunId !== null}
        {#key `${resultRunId}/${displayedRow.request_id}`}
          <Inspector runId={resultRunId} requestId={displayedRow.request_id} embedded />
        {/key}
      {:else}
        <div class="empty-result">{ownsRun && busy ? 'Waiting for response' : 'No response yet'}</div>
      {/if}
    </div>
  </div>
{:else}
  <div class="def">
    <div class="request-head">
      <div class="head">
        <span class="name">request not found</span>
      </div>
      <div class="hintline">
        {path} has no request “{slug}” — the file may have changed on disk
      </div>
      <div class="actions">
        <a href="#/" class="at-btn ghost">← back to the run view</a>
      </div>
    </div>
  </div>
{/if}

<style>
  .def {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    min-width: 0;
    overflow: hidden;
  }
  .request-head {
    flex: none;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: var(--pad);
    border-bottom: 1px solid var(--bd0);
    background: var(--bg1);
  }
  .head {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .name {
    font: 600 var(--fs-lg) var(--font-sans);
    color: var(--fg0);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .badge {
    font-size: var(--fs-xs);
    color: var(--fg2);
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    padding: 1px 6px;
  }
  .phase {
    margin-left: auto;
    font-size: var(--fs-xs);
    color: var(--fg2);
  }
  .row {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: 10px;
    min-width: 0;
  }
  .label {
    flex: none;
    width: 52px;
    font-size: var(--fs-xs);
    color: var(--fg2);
  }
  .src {
    font-size: var(--fs-sm);
    color: var(--fg1);
    overflow-wrap: anywhere;
  }
  .hintline {
    font-size: var(--fs-xs);
    color: var(--fg2);
  }
  .actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    padding-top: 4px;
  }
  .run-status {
    color: var(--fg2);
    font-size: var(--fs-xs);
    overflow-wrap: anywhere;
  }
  .response {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
  .empty-result {
    margin: auto;
    color: var(--fg3);
  }
  .steps {
    flex: none;
    max-height: 160px;
    overflow: auto;
    border-bottom: 1px solid var(--bd0);
  }
  .step {
    display: grid;
    grid-template-columns: 64px minmax(0, 1fr) 82px 32px;
    gap: 10px;
    width: 100%;
    padding: 6px var(--pad);
    border: 0;
    background: transparent;
    color: var(--fg1);
    font-size: var(--fs-sm);
    text-align: left;
    cursor: pointer;
  }
  .step:hover, .step[aria-pressed='true'] {
    background: var(--bg2);
  }
  .step:disabled {
    color: var(--fg2);
    cursor: default;
  }
  .step-phase {
    color: var(--fg2);
  }
  .step-name {
    overflow-wrap: anywhere;
  }
</style>

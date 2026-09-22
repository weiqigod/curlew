<script lang="ts">
  // Request definition panel (#/def/<path>/<slug>): the click-anywhere landing
  // page for a sidebar request. Renders the parsed definition from /tree —
  // never resolved values — plus run-this and open-in-editor affordances, and
  // a link to the latest result when the focused run has one.
  import { startRun } from '../../controller';
  import { navigate } from '../../router';
  import { selectedEnv } from '../../stores/environments';
  import { focusedRequests, routeRunId } from '../../stores/focused-run';
  import { meta } from '../../stores/meta';
  import { runMeta, runState } from '../../stores/run';
  import { tree } from '../../stores/tree';
  import Method from '../atoms/Method.svelte';
  import OpenInEditor from '../atoms/OpenInEditor.svelte';
  import TemplateUrl from '../atoms/TemplateUrl.svelte';

  export let path: string;
  export let slug: string;

  $: collection = ($tree?.collections ?? []).find((c) => c.path === path);
  $: request = collection?.requests.find((r) => r.slug === slug);

  $: busy = $runState === 'starting' || $runState === 'running' || $runState === 'cancelling';

  // Latest result for this request in the focused run (if any).
  $: resultRow = $focusedRequests.find(
    (x) =>
      x.source_file === path &&
      (x.iteration?.base_slug ?? x.slug) === slug &&
      x.status !== 'pending' &&
      x.status !== 'running',
  );
  $: resultRunId = $routeRunId ?? $runMeta.run_id;

  function runThis(): void {
    if (busy || request?.phase !== 'main') return;
    void startRun({
      collection: path,
      env: $selectedEnv ?? $meta?.project.default_env ?? '',
      parallel: false,
      mode: 'selection',
      selection: [request.name],
      rerun_of: null,
    });
  }

  function viewResult(): void {
    if (resultRow !== undefined && resultRunId !== null) {
      navigate({ name: 'inspector', runId: resultRunId, requestId: resultRow.request_id });
    }
  }
</script>

{#if request !== undefined && collection !== undefined}
  <div class="def">
    <div class="card">
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

      <div class="hintline">
        definitions are read-only — edit the YAML file; this view shows raw templates, never
        resolved values
      </div>

      <div class="actions">
        {#if request.phase === 'main'}
          <button class="at-btn" disabled={busy} on:click={runThis}>
            ▶ Run this request
          </button>
        {/if}
        {#if resultRow !== undefined && resultRunId !== null}
          <button class="at-btn ghost" on:click={viewResult}>view latest result →</button>
        {/if}
        <a href="#/" class="at-btn ghost">back to run view</a>
      </div>
    </div>
  </div>
{:else}
  <div class="def">
    <div class="card">
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
    align-items: flex-start;
    justify-content: center;
    min-height: 0;
    padding: calc(var(--pad) * 3) var(--pad);
    overflow-y: auto;
  }
  .card {
    width: 100%;
    max-width: 720px;
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: var(--pad);
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
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
  }
  .hintline {
    font-size: var(--fs-xs);
    color: var(--fg2);
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-top: 4px;
  }
</style>

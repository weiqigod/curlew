<script lang="ts" context="module">
  // Scroll position preserved across inspector visits (§10.6.2.3).
  let savedScroll = 0;
</script>

<script lang="ts">
  // Run view (§10.6.2): SummaryStrip, filter chip, RunErrorBanner, layout
  // toggle, and the grouped result area (compact list / columns / lanes).
  import { onDestroy, onMount, tick } from 'svelte';
  import { registerKey } from '../../keyboard';
  import type { LiveRequest } from '../../event-reducer';
  import { fileBasename, relTime } from '../../format';
  import { navigate, route } from '../../router';
  import {
    focusedRequests,
    focusedRunInfo,
    focusedSummary,
    loadPinnedRun,
    pinnedRunError,
    routeRunId,
    viewingPastRun,
  } from '../../stores/focused-run';
  import { meta } from '../../stores/meta';
  import { selectedEnv } from '../../stores/environments';
  import { nowTick, runClock, runMeta, runState } from '../../stores/run';
  import { tree } from '../../stores/tree';
  import { listCursor, prefs, returnRoute } from '../../stores/ui';
  import { openInEditor } from '../../api/open';
  import Icon from '../atoms/Icon.svelte';
  import NotFoundPanel from '../states/NotFoundPanel.svelte';
  import CollectionHeader from './CollectionHeader.svelte';
  import LayoutToggle from './LayoutToggle.svelte';
  import PhaseHeader from './PhaseHeader.svelte';
  import RequestCard from './RequestCard.svelte';
  import RequestRow from './RequestRow.svelte';
  import RunErrorBanner from './RunErrorBanner.svelte';
  import SummaryStrip from './SummaryStrip.svelte';
  import WaveHeader from './WaveHeader.svelte';

  let scroller: HTMLElement;

  // ---- pinned run loading --------------------------------------------------
  $: if ($routeRunId !== null) {
    void loadPinnedRun($routeRunId, $runMeta.run_id);
  }

  // ---- filtering -------------------------------------------------------------
  $: fileFilter = $route.name === 'tree' ? $route.path : null;
  $: rows =
    fileFilter !== null
      ? $focusedRequests.filter((r) => r.source_file === fileFilter)
      : $focusedRequests;

  $: running = $runState === 'running' || $runState === 'cancelling' || $runState === 'starting';
  $: isParallel = $focusedSummary.parallel === true || ($runMeta.params?.parallel === true && $focusedSummary.live);
  $: distinctFiles = new Set(rows.map((r) => r.source_file)).size;
  $: isBatch = distinctFiles > 1;
  $: hasPhases = rows.some((r) => r.phase !== 'main');
  $: layout = isParallel ? $prefs.runLayout : 'compact';

  $: elapsedMs = running && $runClock !== null ? Math.max(0, $nowTick - $runClock) : null;

  // now/t0 are passed from the template so the expressions re-evaluate on
  // every tick (Svelte 4 template-dependency rule).
  function rowElapsed(row: LiveRequest, now: number, t0: number | null): number | null {
    if (row.status !== 'running' || t0 === null || row.at_ms === undefined) return null;
    return Math.max(0, now - t0 - row.at_ms);
  }

  $: envEcho =
    $focusedRunInfo?.meta?.env_name ?? $runMeta.params?.env ?? null;

  $: pastRelative =
    $viewingPastRun && $focusedRunInfo?.meta !== undefined
      ? relTime($focusedRunInfo.meta.created_at)
      : $viewingPastRun
        ? ''
        : null;

  // ---- compact-list grouping (§10.6.2.3): collection → phase → wave --------
  interface Item {
    kind: 'collection' | 'phase' | 'wave' | 'row';
    row?: LiveRequest;
    file?: string;
    phase?: 'setup' | 'teardown';
    wave?: number;
    waveCount?: number;
  }

  $: items = buildItems(rows, isBatch, hasPhases, isParallel);

  function buildItems(
    list: LiveRequest[],
    batch: boolean,
    phases: boolean,
    parallel: boolean,
  ): Item[] {
    const out: Item[] = [];
    let curFile: string | null = null;
    let curPhase: string | null = null;
    let curWave: number | null = null;
    for (const r of list) {
      if (batch && r.source_file !== curFile) {
        curFile = r.source_file;
        curPhase = null;
        curWave = null;
        out.push({ kind: 'collection', file: r.source_file });
      }
      if (phases && r.phase !== curPhase) {
        curPhase = r.phase;
        curWave = null;
        if (r.phase !== 'main') {
          out.push({ kind: 'phase', phase: r.phase });
        }
      }
      if (parallel && r.wave_index >= 0 && r.wave_index !== curWave) {
        curWave = r.wave_index;
        const count = list.filter(
          (x) => x.wave_index === r.wave_index && (!batch || x.source_file === r.source_file),
        ).length;
        out.push({ kind: 'wave', wave: r.wave_index, waveCount: count });
      }
      out.push({ kind: 'row', row: r });
    }
    return out;
  }

  function waveDuration(i: number, durations: number[] | undefined): number | null {
    if (isBatch || running) return null;
    return durations?.[i] ?? null;
  }

  // ---- columns / lanes wave groups ------------------------------------------
  interface WaveGroup {
    file: string | null;
    wave: number;
    rows: LiveRequest[];
  }

  $: waveGroups = buildWaveGroups(rows, isBatch);

  function buildWaveGroups(list: LiveRequest[], batch: boolean): WaveGroup[] {
    const groups: WaveGroup[] = [];
    for (const r of list) {
      const file = batch ? r.source_file : null;
      const wave = Math.max(0, r.wave_index);
      const last = groups[groups.length - 1];
      if (last !== undefined && last.file === file && last.wave === wave) {
        last.rows.push(r);
      } else {
        groups.push({ file, wave, rows: [r] });
      }
    }
    return groups;
  }

  // ---- cursor (j/k) ----------------------------------------------------------
  $: cursorId = $listCursor;
  $: rowIds = rows.map((r) => r.request_id);

  function moveCursor(d: number): void {
    if (rowIds.length === 0) return;
    const i = cursorId !== null ? rowIds.indexOf(cursorId) : -1;
    const next =
      i === -1 ? (d > 0 ? 0 : rowIds.length - 1) : Math.min(rowIds.length - 1, Math.max(0, i + d));
    listCursor.set(rowIds[next]);
    void tick().then(() => {
      document.getElementById(`row-${rowIds[next]}`)?.scrollIntoView({ block: 'nearest' });
    });
  }

  function openCursor(): void {
    if (cursorId === null) return;
    openRow(cursorId);
  }

  function openRow(id: string): void {
    const row = rows.find((r) => r.request_id === id);
    if (row === undefined) return;
    if (
      row.status !== 'passed' &&
      row.status !== 'failed' &&
      row.status !== 'skipped' &&
      row.status !== 'error'
    ) {
      return;
    }
    const runId = $routeRunId ?? $runMeta.run_id;
    if (runId === null) return;
    savedScroll = scroller?.scrollTop ?? 0;
    listCursor.set(id);
    returnRoute.set($route);
    navigate({ name: 'inspector', runId, requestId: id });
  }

  function editorCursor(): void {
    if (cursorId === null) return;
    const row = rows.find((r) => r.request_id === cursorId);
    if (row !== undefined && row.source_file !== '') {
      void openInEditor(row.source_file, row.source_line).catch(() => undefined);
    }
  }

  // ---- no-run-yet ------------------------------------------------------------
  $: noRunYet = $runMeta.run_id === null && $routeRunId === null;
  $: totalPlanned = ($tree?.collections ?? [])
    .filter((c) => c.valid)
    .reduce((n, c) => n + (c.counts?.main ?? 0), 0);
  $: historyAvailable = $meta?.history.enabled === true;
  $: runNotFound = $routeRunId !== null && $pinnedRunError === $routeRunId;

  let unregister: Array<() => void> = [];
  onMount(() => {
    unregister = [
      registerKey('listMove', (d) => moveCursor(d ?? 1)),
      registerKey('listOpen', openCursor),
      registerKey('listOpenEditor', editorCursor),
    ];
    if (scroller !== undefined) scroller.scrollTop = savedScroll;
    // Focus restoration (§10.7): returning from the inspector focuses the
    // originating row.
    if ($listCursor !== null) {
      void tick().then(() => document.getElementById(`row-${$listCursor}`)?.focus());
    }
  });
  onDestroy(() => {
    unregister.forEach((u) => u());
  });
</script>

<div class="rv">
  {#if runNotFound}
    <NotFoundPanel message="run no longer available" />
  {:else if noRunYet}
    <div class="norun">
      <div class="hl" tabindex="-1">No run yet</div>
      <div class="sub">
        Press <kbd class="at-kbd">r</kbd> or click <strong>Run all</strong> to execute
        <span class="at-mono">{totalPlanned}</span> requests against
        <span class="at-mono">{$selectedEnv ?? $meta?.project.default_env ?? ''}</span>
      </div>
      {#if historyAvailable}
        <a href="#/compare" class="histlink">view previous runs →</a>
      {/if}
    </div>
  {:else}
    <SummaryStrip
      summary={$focusedSummary}
      running={running && !$viewingPastRun}
      {elapsedMs}
      envName={envEcho}
      pastRunRelative={pastRelative}
      batch={isBatch}
      onClosePastRun={() => navigate({ name: 'run' })}
    />

    {#if fileFilter !== null}
      <div class="chiprow">
        <span class="at-chip filterchip">
          filtered: {fileBasename(fileFilter)}
          <button class="chipx" aria-label="clear filter" on:click={() => navigate({ name: 'run' })}>
            <Icon name="x" size={11} />
          </button>
        </span>
      </div>
    {/if}

    {#if $runMeta.error !== null && !$viewingPastRun}
      <RunErrorBanner error={$runMeta.error} />
    {/if}

    <div class="togglerow">
      <span class="sp"></span>
      <LayoutToggle parallel={isParallel} />
    </div>

    {#if layout === 'compact'}
      <div
        class="list-scroll"
        bind:this={scroller}
        role="listbox"
        aria-label="run results"
        aria-activedescendant={cursorId !== null ? `row-${cursorId}` : undefined}
        tabindex="0"
      >
        <div class="listbox">
          {#each items as item, i (item.kind + (item.row?.request_id ?? item.file ?? item.phase ?? String(item.wave)) + i)}
            {#if item.kind === 'collection' && item.file !== undefined}
              <CollectionHeader file={item.file} />
            {:else if item.kind === 'phase' && item.phase !== undefined}
              <PhaseHeader phase={item.phase} />
            {:else if item.kind === 'wave' && item.wave !== undefined}
              <WaveHeader
                index={item.wave}
                count={item.waveCount ?? 0}
                durationMs={waveDuration(item.wave, $focusedSummary.wave_durations_ms)}
              />
            {:else if item.row !== undefined}
              <RequestRow
                row={item.row}
                showPhase={!hasPhases && item.row.phase !== 'main'}
                showCollection={!isBatch}
                liveElapsedMs={rowElapsed(item.row, $nowTick, $runClock)}
                cursor={cursorId === item.row.request_id}
                on:open={(e) => openRow(e.detail)}
              />
            {/if}
          {/each}
        </div>
      </div>
    {:else if layout === 'columns'}
      <div class="cols-scroll" bind:this={scroller}>
        {#each waveGroups as g, gi}
          {#if gi > 0}
            <div class="colchev"><Icon name="chevron-right" size={12} /></div>
          {/if}
          <div class="col">
            {#if g.file !== null && (gi === 0 || waveGroups[gi - 1].file !== g.file)}
              <div class="colfile at-mono">{fileBasename(g.file)}</div>
            {/if}
            <div class="collabel">
              <span class="wl">Wave {g.wave + 1}</span>
              <span class="wn at-mono">{g.rows.length} req</span>
            </div>
            {#each g.rows as r (r.request_id)}
              <RequestCard row={r} liveElapsedMs={rowElapsed(r, $nowTick, $runClock)} on:open={(e) => openRow(e.detail)} />
            {/each}
          </div>
        {/each}
      </div>
    {:else}
      <div class="lanes-scroll" bind:this={scroller}>
        {#each waveGroups as g, gi}
          <div class="lane">
            <div class="lanelabel">
              {#if g.file !== null && (gi === 0 || waveGroups[gi - 1].file !== g.file)}
                <div class="colfile at-mono">{fileBasename(g.file)}</div>
              {/if}
              <span class="wl">Wave {g.wave + 1}</span>
              <span class="wn at-mono">{g.rows.length} req</span>
            </div>
            <div class="lanegrid">
              {#each g.rows as r (r.request_id)}
                <RequestCard row={r} liveElapsedMs={rowElapsed(r, $nowTick, $runClock)} on:open={(e) => openRow(e.detail)} />
              {/each}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .rv {
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
  }
  .norun {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    min-height: 0;
  }
  .hl {
    font: 600 var(--fs-lg) var(--font-sans);
    outline: none;
  }
  .sub {
    font-size: var(--fs-sm);
    color: var(--fg2);
  }
  .histlink {
    font-size: var(--fs-sm);
    color: var(--fg1);
  }
  .chiprow {
    padding: var(--pad-sm) var(--pad) 0;
    display: flex;
  }
  .filterchip {
    gap: 6px;
  }
  .chipx {
    all: unset;
    cursor: pointer;
    display: flex;
    color: var(--fg2);
  }
  .togglerow {
    display: flex;
    align-items: center;
    padding: var(--pad-sm) var(--pad) 0;
  }
  .sp {
    flex: 1;
  }
  .list-scroll {
    flex: 1;
    overflow: auto;
    padding: var(--pad);
    outline: none;
    min-height: 0;
  }
  .listbox {
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    overflow: hidden;
    background: var(--bg1);
  }
  .cols-scroll {
    flex: 1;
    overflow: auto;
    display: flex;
    gap: 0;
    padding: var(--pad);
    align-items: stretch;
    min-height: 0;
  }
  .colchev {
    flex: none;
    display: flex;
    align-items: flex-start;
    padding: 34px 6px 0;
    color: var(--fg3);
  }
  .col {
    width: 252px;
    flex: none;
    display: flex;
    flex-direction: column;
    gap: var(--pad-sm);
  }
  .collabel,
  .lanelabel {
    display: flex;
    align-items: center;
    gap: 8px;
    white-space: nowrap;
  }
  .wl {
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg2);
  }
  .wn {
    font-size: 10px;
    color: var(--fg3);
  }
  .colfile {
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
  .lanes-scroll {
    flex: 1;
    overflow: auto;
    padding: var(--pad);
    display: flex;
    flex-direction: column;
    gap: var(--pad);
    min-height: 0;
  }
  .lane {
    display: flex;
    gap: var(--pad);
    align-items: flex-start;
  }
  .lanelabel {
    width: 96px;
    flex: none;
    padding-top: 6px;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
  }
  .lanegrid {
    flex: 1;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(236px, 1fr));
    gap: var(--pad-sm);
    border-left: 1px solid var(--bd0);
    padding-left: var(--pad);
  }
</style>

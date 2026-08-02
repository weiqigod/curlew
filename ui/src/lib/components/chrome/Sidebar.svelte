<script lang="ts">
  // Sidebar (§10.6.1.6): filter, collection rows with 4-outcome aggregate
  // dots, phase separators, live request dots matched by slug (iterations via
  // base_slug), selection mode scoped to one collection, read-only footer.
  import { onDestroy, onMount } from 'svelte';
  import type { LiveStatus } from '../../event-reducer';
  import { pushEscFallback, registerKey } from '../../keyboard';
  import { navigate, route } from '../../router';
  import { focusedRequests, routeRunId } from '../../stores/focused-run';
  import { runMeta } from '../../stores/run';
  import { toast } from '../../stores/toast';
  import { tree } from '../../stores/tree';
  import { clearSelection, sidebarSelection } from '../../stores/ui';
  import type { TreeCollection, TreeRequest } from '../../types/tree';
  import Dot from '../atoms/Dot.svelte';
  import Icon from '../atoms/Icon.svelte';
  import Method from '../atoms/Method.svelte';
  import TemplateUrl from '../atoms/TemplateUrl.svelte';

  let filter = '';
  let filterInput: HTMLInputElement;
  let collapsed: Record<string, boolean> = {};

  $: q = filter.trim().toLowerCase();
  $: collections = $tree?.collections ?? [];

  // ---- live status lookup: "<source_file>|<base_slug>" → statuses ---------
  $: statusIndex = (() => {
    const idx = new Map<string, LiveStatus[]>();
    for (const r of $focusedRequests) {
      const key = `${r.source_file}|${r.iteration?.base_slug ?? r.slug}`;
      const list = idx.get(key);
      if (list === undefined) {
        idx.set(key, [r.status]);
      } else {
        list.push(r.status);
      }
    }
    return idx;
  })();

  type DotState = LiveStatus | 'mixed' | null;

  function aggregate(statuses: LiveStatus[]): DotState {
    if (statuses.length === 0) return null;
    if (statuses.some((s) => s === 'running')) return 'running';
    if (statuses.every((s) => s === 'pending')) return 'pending';
    const bad = statuses.filter((s) => s === 'failed' || s === 'error');
    const passed = statuses.some((s) => s === 'passed');
    if (bad.length > 0 && passed) return 'mixed';
    if (bad.length > 0 && !passed) {
      return bad.every((s) => s === 'error') ? 'error' : 'failed';
    }
    return statuses.some((s) => s === 'passed') ? 'passed' : 'skipped';
  }

  // NOTE: the index is passed explicitly from the template so {@const}
  // blocks re-evaluate when live statuses change (Svelte 4 dependency rule).
  function requestDot(idx: Map<string, LiveStatus[]>, c: TreeCollection, r: TreeRequest): DotState {
    const statuses = idx.get(`${c.path}|${r.slug}`);
    return statuses === undefined ? null : aggregate(statuses);
  }

  function collectionStatuses(idx: Map<string, LiveStatus[]>, c: TreeCollection): LiveStatus[] {
    const out: LiveStatus[] = [];
    for (const r of c.requests) {
      const statuses = idx.get(`${c.path}|${r.slug}`);
      if (statuses !== undefined) out.push(...statuses);
    }
    return out;
  }

  function enumerate(statuses: LiveStatus[]): string {
    const counts: Record<string, number> = {};
    for (const s of statuses) counts[s] = (counts[s] ?? 0) + 1;
    return ['passed', 'failed', 'error', 'skipped', 'running', 'pending']
      .filter((s) => (counts[s] ?? 0) > 0)
      .map((s) => `${counts[s]} ${s}`)
      .join(' · ');
  }

  // ---- filtering -----------------------------------------------------------
  function matchingRequests(needle: string, c: TreeCollection): TreeRequest[] {
    if (needle === '') return c.requests;
    if (c.path.toLowerCase().includes(needle)) return c.requests;
    return c.requests.filter(
      (r) => r.name.toLowerCase().includes(needle) || r.slug.toLowerCase().includes(needle),
    );
  }

  function visible(needle: string, c: TreeCollection): boolean {
    if (needle === '') return true;
    return c.path.toLowerCase().includes(needle) || matchingRequests(needle, c).length > 0;
  }

  // ---- navigation ----------------------------------------------------------
  function clickCollection(c: TreeCollection): void {
    if (!c.valid) {
      navigate({ name: 'file', path: c.path });
      return;
    }
    collapsed = { ...collapsed, [c.path]: !collapsed[c.path] };
  }

  function dblclickCollection(c: TreeCollection): void {
    if (c.valid) navigate({ name: 'tree', path: c.path });
  }

  function clickRequest(c: TreeCollection, r: TreeRequest): void {
    // → inspector when the request has a result in the focused run.
    const runId = $routeRunId ?? $runMeta.run_id;
    if (runId !== null) {
      const row = $focusedRequests.find(
        (x) =>
          x.source_file === c.path &&
          (x.iteration?.base_slug ?? x.slug) === r.slug &&
          x.status !== 'pending' &&
          x.status !== 'running',
      );
      if (row !== undefined) {
        navigate({ name: 'inspector', runId, requestId: row.request_id });
        return;
      }
    }
    // No result to inspect — land on the request definition instead of a
    // dead click (the old footer hint was invisible in practice).
    navigate({ name: 'definition', path: c.path, slug: r.slug });
  }

  // ---- selection mode ------------------------------------------------------
  $: selection = $sidebarSelection;
  $: selectionMode = selection.names.length > 0;

  function toggleSelect(c: TreeCollection, r: TreeRequest, checked: boolean): void {
    sidebarSelection.update((sel) => {
      if (sel.collection !== null && sel.collection !== c.path && checked) {
        toast(`selection moved to ${c.path.split('/').pop() ?? c.path}`);
        return { collection: c.path, names: [r.name] };
      }
      const names = checked
        ? [...sel.names.filter((n) => n !== r.name), r.name]
        : sel.names.filter((n) => n !== r.name);
      return { collection: names.length > 0 ? c.path : null, names };
    });
  }

  function isSelected(
    sel: { collection: string | null; names: string[] },
    c: TreeCollection,
    r: TreeRequest,
  ): boolean {
    return sel.collection === c.path && sel.names.includes(r.name);
  }

  function filterKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape') {
      e.stopPropagation();
      filter = '';
      filterInput.blur();
    }
  }

  // ---- phase grouping ------------------------------------------------------
  function phaseOf(reqs: TreeRequest[], phase: string): TreeRequest[] {
    return reqs.filter((r) => r.phase === phase);
  }

  $: totalCollections = collections.length;
  $: totalRequests = collections.reduce((n, c) => n + c.requests.length, 0);

  let unregister: Array<() => void> = [];
  onMount(() => {
    unregister = [
      registerKey('focusFilter', () => filterInput.focus()),
      pushEscFallback(() => {
        if (selection.names.length > 0) {
          clearSelection();
          return true;
        }
        return false;
      }),
    ];
  });
  onDestroy(() => {
    unregister.forEach((u) => u());
  });

  $: selectedPath =
    $route.name === 'tree' || $route.name === 'file' ? $route.path : null;
</script>

<nav class="sidebar" aria-label="collections">
  <div class="filterbox">
    <div class="filterwrap">
      <span class="searchico"><Icon name="search" size={13} /></span>
      <input
        class="at-input filterinput"
        placeholder="Filter requests…"
        aria-label="filter requests"
        bind:value={filter}
        bind:this={filterInput}
        on:keydown={filterKeydown}
      />
    </div>
  </div>

  <div class="treebody">
    {#each collections as c (c.path)}
      {#if visible(q, c)}
        {@const reqs = matchingRequests(q, c)}
        {@const open = !collapsed[c.path]}
        {@const agg = c.valid ? aggregate(collectionStatuses(statusIndex, c)) : null}
        <div class="cgroup">
          <div
            class="crow"
            class:selrow={selectedPath === c.path}
            role="button"
            tabindex="0"
            title={c.valid ? c.path : (c.issues[0]?.message ?? 'invalid collection')}
            on:click={() => clickCollection(c)}
            on:dblclick={() => dblclickCollection(c)}
            on:keydown={(e) => e.key === 'Enter' && clickCollection(c)}
          >
            <span class="chev">
              {#if c.valid}<Icon name={open ? 'chevron-down' : 'chevron-right'} size={12} />{/if}
            </span>
            <span class="cpath at-mono">{c.path.replace(/^collections\//, '')}</span>
            {#if !c.valid}
              <span class="invalid"><Icon name="warn" size={13} /> invalid</span>
            {:else}
              <span class="ccount at-mono">{c.requests.length}</span>
              {#if agg !== null && agg !== 'pending'}
                <Dot state={agg} title={enumerate(collectionStatuses(statusIndex, c))} />
              {/if}
            {/if}
          </div>

          {#if open && c.valid}
            {#each ['setup', 'main', 'teardown'] as phase}
              {@const phaseReqs = phaseOf(reqs, phase)}
              {#if phaseReqs.length > 0}
                {#if phase !== 'main' && (c.counts?.setup ?? 0) + (c.counts?.teardown ?? 0) > 0}
                  <div class="phasesep">{phase}</div>
                {/if}
                {#each phaseReqs as r (r.slug)}
                  {@const dot = requestDot(statusIndex, c, r)}
                  <div
                    class="rrow"
                    class:checked={isSelected(selection, c, r)}
                    class:active={$route.name === 'definition' &&
                      $route.path === c.path &&
                      $route.slug === r.slug}
                    role="button"
                    tabindex="0"
                    on:click={() => clickRequest(c, r)}
                    on:keydown={(e) => e.key === 'Enter' && clickRequest(c, r)}
                  >
                    {#if r.phase === 'main'}
                      <span
                        class="selbox"
                        class:visible={selectionMode || isSelected(selection, c, r)}
                        role="presentation"
                        on:click|stopPropagation
                      >
                        <input
                          type="checkbox"
                          aria-label={`select ${r.name}`}
                          checked={isSelected(selection, c, r)}
                          on:click|stopPropagation
                          on:change={(e) => toggleSelect(c, r, e.currentTarget.checked)}
                        />
                      </span>
                    {:else}
                      <span class="selbox spacer"></span>
                    {/if}
                    {#if dot !== null}
                      <Dot state={dot} />
                    {:else}
                      <span class="nodot"></span>
                    {/if}
                    <span class="rname">{r.name}</span>
                    {#if r.data_driven}
                      <span class="ddglyph" title="data-driven — expands at run time">⛁</span>
                    {/if}
                    <Method m={r.method} />
                    <div class="tip" role="tooltip">
                      <TemplateUrl url={r.url} />
                      <span class="tiploc at-mono">{c.path}:{r.source_line}</span>
                      {#if r.required}<span class="tiploc at-mono">required</span>{/if}
                    </div>
                  </div>
                {/each}
              {/if}
            {/each}
          {/if}
        </div>
      {/if}
    {/each}
  </div>

  <div class="foot at-mono">
    {#if selectionMode}
      <span>{selection.names.length} selected · esc to clear</span>
    {:else}
      <span>{totalCollections} collections</span><span>·</span><span>{totalRequests} requests</span>
    {/if}
    <span class="sp"></span>
    <span title="files are the source of truth — curlew never edits them">read-only</span>
  </div>
</nav>

<style>
  .sidebar {
    width: 264px;
    flex: none;
    display: flex;
    flex-direction: column;
    background: var(--bg1);
    border-right: 1px solid var(--bd0);
    min-height: 0;
  }
  .filterbox {
    padding: 8px;
    border-bottom: 1px solid var(--bd0);
  }
  .filterwrap {
    position: relative;
  }
  .searchico {
    position: absolute;
    left: 7px;
    top: 6px;
    color: var(--fg3);
  }
  .filterinput {
    padding-left: 26px;
  }
  .treebody {
    flex: 1;
    overflow-y: auto;
    padding: 6px;
  }
  .cgroup {
    margin-bottom: 2px;
  }
  .crow,
  .rrow {
    display: flex;
    align-items: center;
    gap: 7px;
    height: var(--row-h);
    padding: 0 8px;
    border-radius: var(--rad);
    cursor: pointer;
    user-select: none;
    min-width: 0;
  }
  .crow:hover,
  .rrow:hover {
    background: var(--bg2);
  }
  .crow.selrow {
    background: var(--acc-dim);
  }
  .chev {
    color: var(--fg3);
    display: flex;
    width: 12px;
    flex: none;
  }
  .cpath {
    font-size: var(--fs-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .invalid {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    color: var(--warn);
    font-size: 10px;
    font-weight: 600;
    flex: none;
  }
  .ccount {
    font-size: 10px;
    color: var(--fg3);
  }
  .phasesep {
    font: 600 10px var(--font-sans);
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg3);
    padding: 4px 8px 2px 27px;
  }
  .rrow {
    padding-left: 8px;
    gap: 8px;
    position: relative;
  }
  .rrow.checked {
    background: var(--acc-dim);
  }
  .rrow.active {
    background: var(--bg2);
  }
  .selbox {
    width: 13px;
    flex: none;
    display: inline-flex;
    visibility: hidden;
  }
  .selbox.visible,
  .rrow:hover .selbox:not(.spacer),
  .rrow:focus-within .selbox:not(.spacer) {
    visibility: visible;
  }
  .selbox input {
    width: 12px;
    height: 12px;
    margin: 0;
    accent-color: var(--acc);
    cursor: pointer;
  }
  .nodot {
    width: 8px;
    flex: none;
  }
  .rname {
    font-size: var(--fs-sm);
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--fg1);
  }
  .ddglyph {
    color: var(--fg2);
    font-size: 11px;
    flex: none;
  }
  .tip {
    display: none;
    position: absolute;
    left: 12px;
    top: calc(var(--row-h) - 2px);
    z-index: 45;
    background: var(--bg2);
    border: 1px solid var(--bd1);
    border-radius: var(--rad);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
    padding: 6px 8px;
    max-width: 360px;
    flex-direction: column;
    gap: 3px;
    pointer-events: none;
  }
  .rrow:hover .tip {
    display: flex;
  }
  .tiploc {
    font-size: 10px;
    color: var(--fg3);
  }
  .foot {
    padding: 7px 12px;
    border-top: 1px solid var(--bd0);
    display: flex;
    gap: 8px;
    align-items: center;
    color: var(--fg3);
    font-size: var(--fs-xs);
    white-space: nowrap;
  }
  .foot .sp {
    flex: 1;
  }
</style>

<script lang="ts">
  // Inspector (§10.6.3): header from list-store data immediately, full detail
  // from GET /runs/{id}/requests/{rid}; Error tab first+default on error
  // outcomes; skip panel replaces tabs when skipped (Request stays available).
  import { onDestroy, onMount, tick } from 'svelte';
  import { get } from 'svelte/store';
  import { openInEditor as openInEditorApi } from '../../api/open';
  import { getRequestDetail } from '../../api/runs';
  import { registerKey } from '../../keyboard';
  import { fmtMs } from '../../format';
  import { navigate, route, type InspectorTab } from '../../router';
  import { focusedRequests } from '../../stores/focused-run';
  import { tree } from '../../stores/tree';
  import { returnRoute } from '../../stores/ui';
  import type { RequestDetail } from '../../types/run';
  import Code from '../atoms/Code.svelte';
  import Dot from '../atoms/Dot.svelte';
  import Icon from '../atoms/Icon.svelte';
  import OpenInEditor from '../atoms/OpenInEditor.svelte';
  import RetryBadge from '../atoms/RetryBadge.svelte';
  import Tabs, { type TabDef } from '../atoms/Tabs.svelte';
  import TemplateUrl from '../atoms/TemplateUrl.svelte';
  import AssertionsTab from './AssertionsTab.svelte';
  import BodyTab from './BodyTab.svelte';
  import ErrorTab from './ErrorTab.svelte';
  import HeadersTab from './HeadersTab.svelte';
  import RequestTab from './RequestTab.svelte';
  import TimingTab from './TimingTab.svelte';

  export let runId: string;
  export let requestId: string;
  export let tab: InspectorTab | undefined = undefined;

  let detail: RequestDetail | null = null;
  let loadError: string | null = null;
  let backBtn: HTMLButtonElement;

  // Header renders immediately from list-store data while detail loads.
  $: liveRow = $focusedRequests.find((r) => r.request_id === requestId);

  let lastKey = '';
  $: key = `${runId}/${requestId}`;
  $: if (key !== lastKey) {
    lastKey = key;
    detail = null;
    loadError = null;
    void load(runId, requestId);
  }

  async function load(rid: string, reqId: string): Promise<void> {
    try {
      const d = await getRequestDetail(rid, reqId);
      if (`${rid}/${reqId}` === lastKey) detail = d;
    } catch {
      if (`${rid}/${reqId}` === lastKey) loadError = 'request detail not available';
    }
  }

  $: outcome = detail?.outcome ?? liveRow?.status ?? null;
  $: isError = outcome === 'error';
  $: isSkipped = outcome === 'skipped';

  $: name = (() => {
    const iter = detail?.iteration ?? liveRow?.iteration;
    const base = detail?.name ?? liveRow?.name ?? requestId;
    return iter != null ? `${iter.base_name} ${iter.index + 1}/${iter.total}` : base;
  })();
  $: method = detail?.method ?? liveRow?.method ?? '';
  $: statusCode = detail?.status_code ?? liveRow?.status_code;
  $: durationMs = detail?.duration_ms ?? liveRow?.duration_ms;
  $: retryCount = detail?.retry?.count ?? liveRow?.retry_count ?? 0;
  $: sourceFile = detail?.source?.file ?? liveRow?.source_file ?? '';
  $: sourceLine = detail?.source?.line ?? liveRow?.source_line ?? 1;

  // Raw template URL from the tree by slug (§10.6.3 header).
  $: templateUrl = (() => {
    const slug = detail?.iteration?.base_slug ?? liveRow?.iteration?.base_slug ?? detail?.slug ?? liveRow?.slug;
    if (slug === undefined) return null;
    for (const c of $tree?.collections ?? []) {
      if (sourceFile !== '' && c.path !== sourceFile) continue;
      const r = c.requests.find((x) => x.slug === slug);
      if (r !== undefined) return r.url;
    }
    return null;
  })();

  // ---- tabs ------------------------------------------------------------------
  $: headerCount = detail !== null ? Object.entries(detail.response?.headers ?? {}).reduce((n, [, v]) => n + v.length, 0) : 0;
  $: assertionItems = detail?.assertions?.items ?? [];
  $: failedAssertions = assertionItems.filter((a) => !a.passed).length;

  const notExecuted = 'request was not executed';
  const noResponse = 'no response — the request errored before/while receiving';

  $: tabDefs = buildTabs(isError, isSkipped, headerCount, assertionItems.length, failedAssertions);

  function buildTabs(
    err: boolean,
    skipped: boolean,
    nHeaders: number,
    nAsserts: number,
    nFailed: number,
  ): TabDef[] {
    const defs: TabDef[] = [];
    if (err) defs.push({ id: 'error', label: 'Error', accent: true });
    if (skipped) defs.push({ id: 'skipped', label: 'Skipped' });
    defs.push(
      {
        id: 'body',
        label: 'Body',
        ...(skipped ? { disabled: true, title: notExecuted } : {}),
      },
      {
        id: 'headers',
        label: 'Headers',
        count: String(nHeaders),
        ...(skipped ? { disabled: true, title: notExecuted } : {}),
      },
      {
        id: 'assertions',
        label: 'Assertions',
        count: nFailed > 0 ? `${nFailed}✗` : String(nAsserts),
        countErr: nFailed > 0,
        ...(skipped ? { disabled: true, title: notExecuted } : {}),
      },
      {
        id: 'timing',
        label: 'Timing',
        ...(skipped ? { disabled: true, title: notExecuted } : {}),
      },
      { id: 'request', label: 'Request' },
    );
    return defs;
  }

  $: activeTab = tab ?? (isError ? 'error' : isSkipped ? 'skipped' : 'body');

  function selectTab(id: string): void {
    const t = id === 'skipped' ? undefined : (id as InspectorTab);
    navigate({ name: 'inspector', runId, requestId, ...(t !== undefined ? { tab: t } : {}) });
  }

  function selectTabByIndex(n: number): void {
    const enabled = tabDefs.filter((t) => t.disabled !== true);
    const def = enabled[n - 1];
    if (def !== undefined) selectTab(def.id);
  }

  // ---- navigation --------------------------------------------------------------
  function back(): void {
    const origin = get(returnRoute);
    navigate(origin ?? { name: 'run' });
  }

  function nav(d: number): void {
    const withResult = $focusedRequests.filter(
      (r) =>
        r.status === 'passed' ||
        r.status === 'failed' ||
        r.status === 'skipped' ||
        r.status === 'error',
    );
    if (withResult.length === 0) return;
    const i = withResult.findIndex((r) => r.request_id === requestId);
    const next = i === -1 ? 0 : Math.min(withResult.length - 1, Math.max(0, i + d));
    if (withResult[next].request_id === requestId) return;
    const current = get(route);
    navigate({
      name: 'inspector',
      runId,
      requestId: withResult[next].request_id,
      ...(current.name === 'inspector' && current.tab !== undefined ? { tab: current.tab } : {}),
    });
  }

  function openEditor(): void {
    if (sourceFile !== '') {
      void openInEditorApi(sourceFile, sourceLine).catch(() => undefined);
    }
  }

  let unregister: Array<() => void> = [];
  onMount(() => {
    unregister = [
      registerKey('inspectorBack', back),
      registerKey('inspectorNav', (d) => nav(d ?? 1)),
      registerKey('selectTab', (n) => selectTabByIndex(n ?? 1)),
      registerKey('openEditor', openEditor),
    ];
    void tick().then(() => backBtn?.focus());
  });
  onDestroy(() => unregister.forEach((u) => u()));
</script>

<div class="insp">
  <div class="head">
    <button class="at-btn ghost sm backbtn" aria-label="back to run view" bind:this={backBtn} on:click={back}>
      <Icon name="back" size={13} />
    </button>
    {#if outcome !== null}
      <Dot state={outcome} />
    {/if}
    <span class="name">{name}</span>
    <span class="urlbit at-mono" title={detail?.url ?? ''}>
      {method}
      {#if templateUrl !== null}
        <TemplateUrl url={templateUrl} />
      {:else if detail !== null}
        <span class="resolved">{detail.url}</span>
      {/if}
    </span>
    <span class="sp"></span>
    {#if statusCode !== undefined}<Code code={statusCode} /><span class="dotsep">·</span>{/if}
    {#if durationMs !== undefined && !isSkipped}
      <span class="dur at-mono">{fmtMs(durationMs)}</span>
    {/if}
    <RetryBadge count={retryCount} />
    {#if sourceFile !== ''}
      <OpenInEditor file={sourceFile} line={sourceLine} compact />
    {/if}
  </div>

  <Tabs tabs={tabDefs} active={activeTab} on:change={(e) => selectTab(e.detail)} />

  {#if loadError !== null}
    <div class="loaderr at-mono">{loadError}</div>
  {:else if detail === null}
    <div class="loading"><span class="at-spin"></span></div>
  {:else if activeTab === 'skipped'}
    <div class="skippanel">
      <Dot state="skipped" />
      <div class="sktitle">Skipped</div>
      <div class="skreason at-mono">{detail.skip_reason ?? 'skipped'}</div>
    </div>
  {:else if activeTab === 'error'}
    <ErrorTab {runId} {detail} />
  {:else if activeTab === 'body'}
    <BodyTab
      {runId}
      requestId={detail.request_id}
      slug={detail.slug}
      body={detail.response?.body ?? null}
      which="response"
      emptyText={isError ? noResponse : 'no response body'}
    />
  {:else if activeTab === 'headers'}
    {#if detail.response !== null}
      <HeadersTab headers={detail.response.headers} />
    {:else}
      <div class="empty at-mono">{isError ? noResponse : 'no response headers'}</div>
    {/if}
  {:else if activeTab === 'assertions'}
    {#if isError && assertionItems.length === 0}
      <div class="empty at-mono">{noResponse}</div>
    {:else}
      <AssertionsTab assertions={detail.assertions} />
    {/if}
  {:else if activeTab === 'timing'}
    <TimingTab timing={detail.timing} retry={detail.retry} durationMs={detail.duration_ms} />
  {:else if activeTab === 'request'}
    <RequestTab {runId} {detail} />
  {/if}
</div>

<style>
  .insp {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: var(--pad-sm) var(--pad);
    flex: none;
    border-bottom: 1px solid var(--bd0);
    background: var(--bg1);
  }
  .backbtn {
    padding: 0 5px;
  }
  .name {
    font-weight: 600;
    font-size: var(--fs-md);
    white-space: nowrap;
  }
  .urlbit {
    font-size: var(--fs-xs);
    color: var(--fg3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: inline-flex;
    gap: 6px;
    align-items: baseline;
    min-width: 0;
  }
  .resolved {
    color: var(--fg2);
  }
  .sp {
    flex: 1;
  }
  .dotsep {
    color: var(--fg3);
  }
  .dur {
    font-size: var(--fs-xs);
    color: var(--fg1);
    white-space: nowrap;
  }
  .loading {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .loaderr,
  .empty {
    padding: var(--pad);
    font-size: var(--fs-sm);
    color: var(--fg3);
  }
  .skippanel {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    min-height: 0;
  }
  .sktitle {
    font: 600 var(--fs-lg) var(--font-sans);
    color: var(--fg1);
  }
  .skreason {
    font-size: var(--fs-sm);
    color: var(--fg2);
  }
</style>

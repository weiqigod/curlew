<script lang="ts">
  // Request tab (§10.6.3.5): resolved URL + template beneath, request headers,
  // request body (same decision logic, which=request), Source section.
  import { tree } from '../../stores/tree';
  import type { RequestDetail } from '../../types/run';
  import Method from '../atoms/Method.svelte';
  import OpenInEditor from '../atoms/OpenInEditor.svelte';
  import TemplateUrl from '../atoms/TemplateUrl.svelte';
  import BodyTab from './BodyTab.svelte';
  import HeadersTab from './HeadersTab.svelte';
  import SourceSnippet from './SourceSnippet.svelte';

  export let runId: string;
  export let detail: RequestDetail;

  // Raw template from the tree, by slug (iterations match via base slug).
  $: templateUrl = (() => {
    const slug = detail.iteration?.base_slug ?? detail.slug;
    for (const c of $tree?.collections ?? []) {
      if (detail.source !== null && c.path !== detail.source.file) continue;
      const r = c.requests.find((x) => x.slug === slug);
      if (r !== undefined) return r.url;
    }
    return null;
  })();
</script>

<div class="rt">
  <div class="urlline at-mono">
    <Method m={detail.method} />
    <span class="resolved">{detail.url}</span>
  </div>
  {#if templateUrl !== null}
    <div class="tplline at-mono">
      <span class="tlbl">template:</span>
      <TemplateUrl url={templateUrl} />
    </div>
  {/if}

  <div class="section">
    <h3>Request headers</h3>
    <div class="hdrs">
      <HeadersTab headers={detail.request?.headers ?? {}} />
    </div>
  </div>

  {#if detail.request?.body != null}
    <div class="section">
      <h3>Request body <span class="note">(variables resolved, secrets redacted)</span></h3>
      <div class="bodybox">
        <BodyTab
          {runId}
          requestId={detail.request_id}
          slug={detail.slug}
          body={detail.request.body}
          which="request"
          shortcuts={false}
        />
      </div>
    </div>
  {/if}

  {#if detail.source !== null}
    <div class="section">
      <div class="srchead">
        <h3>Source</h3>
        <span class="at-mono at-xs at-dim">{detail.source.file}:{detail.source.line}</span>
        <OpenInEditor file={detail.source.file} line={detail.source.line} compact />
      </div>
      {#if (detail.source.snippet?.length ?? 0) > 0}
        <SourceSnippet source={detail.source} />
      {:else}
        <div class="at-mono at-xs at-dim nosnip">
          definition in {detail.source.file}:{detail.source.line} — open in your editor to view
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .rt {
    overflow: auto;
    flex: 1;
    padding: var(--pad);
    display: flex;
    flex-direction: column;
    gap: calc(var(--pad) * 1.4);
  }
  .urlline {
    font-size: var(--fs-sm);
    display: flex;
    gap: 10px;
    align-items: baseline;
  }
  .resolved {
    color: var(--fg1);
    word-break: break-all;
  }
  .tplline {
    display: flex;
    gap: 8px;
    align-items: baseline;
    margin-top: -10px;
  }
  .tlbl {
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
  h3 {
    margin: 0 0 6px;
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg2);
  }
  .note {
    font-weight: 400;
    text-transform: none;
    letter-spacing: 0;
  }
  .hdrs {
    max-width: 760px;
    display: flex;
  }
  .bodybox {
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    background: var(--bg1);
    max-width: 760px;
    display: flex;
    flex-direction: column;
    max-height: 420px;
    overflow: hidden;
  }
  .srchead {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 6px;
  }
  .srchead h3 {
    margin: 0;
  }
  .nosnip {
    padding: 6px 0;
  }
</style>

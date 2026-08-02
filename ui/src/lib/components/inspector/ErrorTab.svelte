<script lang="ts">
  // Error tab (§10.6.3.6): category + code chip + verbatim message, hint
  // block, request-as-sent, partial-response note.
  import type { RequestDetail } from '../../types/run';
  import Method from '../atoms/Method.svelte';
  import HeadersTab from './HeadersTab.svelte';
  import BodyTab from './BodyTab.svelte';

  export let runId: string;
  export let detail: RequestDetail;

  $: err = detail.error;
</script>

<div class="et">
  {#if err !== null}
    <div class="banner">
      <div class="bhead">
        <span class="cat">{err.category}</span>
        {#if err.code !== undefined && err.code !== ''}
          <span class="at-chip">{err.code}</span>
        {/if}
      </div>
      <div class="msg at-mono">{err.message}</div>
    </div>
    {#if err.hint !== undefined && err.hint !== ''}
      <div class="hint">hint: {err.hint}</div>
    {/if}
  {/if}

  {#if detail.status_code !== undefined}
    <div class="partial at-mono">
      a partial response was received (status {detail.status_code}) — see the Body and Headers tabs
    </div>
  {/if}

  <div class="section">
    <h3>Request as sent</h3>
    <div class="urlline at-mono">
      <Method m={detail.method} />
      <span class="resolved">{detail.url}</span>
    </div>
    <div class="hdrs">
      <HeadersTab headers={detail.request?.headers ?? {}} />
    </div>
    {#if detail.request?.body != null}
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
    {/if}
  </div>
</div>

<style>
  .et {
    overflow: auto;
    flex: 1;
    padding: var(--pad);
    display: flex;
    flex-direction: column;
    gap: var(--pad);
  }
  .banner {
    background: color-mix(in srgb, var(--err2) 8%, transparent);
    border: 1px solid color-mix(in srgb, var(--err2) 30%, transparent);
    border-radius: var(--rad);
    padding: 10px 14px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    max-width: 760px;
  }
  .bhead {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .cat {
    font-weight: 700;
    color: var(--err2);
  }
  .msg {
    font-size: var(--fs-sm);
    color: var(--fg0);
    line-height: 1.5;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .hint {
    font-size: var(--fs-sm);
    color: var(--fg1);
    background: var(--bg1);
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    padding: 8px 12px;
    max-width: 760px;
  }
  .partial {
    font-size: var(--fs-xs);
    color: var(--fg2);
  }
  h3 {
    margin: 0 0 6px;
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg2);
  }
  .urlline {
    font-size: var(--fs-sm);
    display: flex;
    gap: 10px;
    align-items: baseline;
    margin-bottom: 8px;
  }
  .resolved {
    color: var(--fg1);
    word-break: break-all;
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
    margin-top: 8px;
  }
</style>

<script lang="ts">
  // Body tab decision tree (§10.6.3.1):
  //   truncated → load-on-demand (/body) · base64 → BinaryPanel ·
  //   JSON (content-type AND parse agree) → JsonTree · else raw <pre> + find.
  import { getRequestBody } from '../../api/runs';
  import { fmtBytes } from '../../format';
  import type { BodyPayload } from '../../types/run';
  import Icon from '../atoms/Icon.svelte';
  import BinaryPanel from './BinaryPanel.svelte';
  import JsonTree from './JsonTree.svelte';

  export let runId: string;
  export let requestId: string;
  export let slug: string;
  export let body: BodyPayload | null;
  export let which: 'request' | 'response' = 'response';
  /** Empty-state line when body is null. */
  export let emptyText = 'no body';
  export let shortcuts = true;

  const BIG_BODY_BYTES = 5 * 1024 * 1024;
  const RAW_CHUNK_LINES = 2000;

  let loading = false;
  let loadError: string | null = null;
  let loadedText: string | null = null;
  let loadedB64: string | null = null;
  let loadedType = '';
  let rawQuery = '';
  let rawShown = RAW_CHUNK_LINES;

  // Reset per request.
  let lastKey = '';
  $: key = `${runId}/${requestId}/${which}`;
  $: if (key !== lastKey) {
    lastKey = key;
    loadedText = null;
    loadedB64 = null;
    loadError = null;
    loading = false;
    rawQuery = '';
    rawShown = RAW_CHUNK_LINES;
  }

  function isJsonType(ct: string | undefined): boolean {
    if (ct === undefined) return false;
    const t = ct.toLowerCase();
    return t.includes('application/json') || t.includes('+json') || t.includes('text/json');
  }

  function tryParse(text: string): { ok: boolean; value?: unknown } {
    try {
      return { ok: true, value: JSON.parse(text) };
    } catch {
      return { ok: false };
    }
  }

  async function loadBody(): Promise<void> {
    loading = true;
    loadError = null;
    try {
      const res = await getRequestBody(runId, requestId, which);
      loadedType = res.headers.get('Content-Type') ?? body?.content_type ?? '';
      const buf = await res.arrayBuffer();
      const bytes = new Uint8Array(buf);
      if (looksBinary(bytes)) {
        let bin = '';
        for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
        loadedB64 = btoa(bin);
      } else {
        loadedText = new TextDecoder().decode(bytes);
      }
    } catch {
      loadError = 'could not load the body — the run may have been evicted';
    } finally {
      loading = false;
    }
  }

  function looksBinary(bytes: Uint8Array): boolean {
    const n = Math.min(bytes.length, 1024);
    for (let i = 0; i < n; i++) {
      if (bytes[i] === 0) return true;
    }
    return false;
  }

  // ---- decision tree -------------------------------------------------------
  $: contentType = loadedType !== '' ? loadedType : (body?.content_type ?? '');
  $: needsLoad = body !== null && body.truncated && body.content === undefined && loadedText === null && loadedB64 === null;
  $: b64 = loadedB64 ?? (body?.encoding === 'base64' ? (body.content ?? null) : null);
  $: text =
    loadedText ?? (body !== null && body.encoding !== 'base64' ? (body.content ?? null) : null);
  $: tooBigForTree = (text?.length ?? 0) > BIG_BODY_BYTES;
  $: parsed = text !== null && !tooBigForTree && isJsonType(contentType) ? tryParse(text) : { ok: false };

  // ---- raw text find -------------------------------------------------------
  $: rawLines = text !== null ? text.split('\n') : [];
  $: rq = rawQuery.trim().toLowerCase();
  $: rawCount =
    rq === '' || text === null
      ? 0
      : (text.toLowerCase().split(rq).length - 1);

  function highlight(line: string, needle: string): Array<{ t: string; hit: boolean }> {
    if (needle === '') return [{ t: line, hit: false }];
    const out: Array<{ t: string; hit: boolean }> = [];
    let rest = line;
    let lower = line.toLowerCase();
    let i = lower.indexOf(needle);
    while (i >= 0) {
      if (i > 0) out.push({ t: rest.slice(0, i), hit: false });
      out.push({ t: rest.slice(i, i + needle.length), hit: true });
      rest = rest.slice(i + needle.length);
      lower = lower.slice(i + needle.length);
      i = lower.indexOf(needle);
    }
    if (rest !== '') out.push({ t: rest, hit: false });
    return out;
  }

  function downloadText(): void {
    if (text === null) return;
    const blob = new Blob([text], { type: contentType || 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${slug}-${which}.txt`;
    a.click();
    URL.revokeObjectURL(url);
  }
</script>

{#if body === null}
  <div class="empty at-mono">{emptyText}</div>
{:else if needsLoad}
  <div class="loadpanel">
    <div class="meta at-mono">
      <span>{body.content_type ?? 'unknown content type'}</span>
      <span class="dotsep">·</span>
      <span>{fmtBytes(body.size)}</span>
    </div>
    {#if loadError !== null}
      <div class="err at-mono">{loadError}</div>
    {/if}
    <button class="at-btn" disabled={loading} on:click={() => void loadBody()}>
      <Icon name="download" size={13} />
      {loading ? 'Loading…' : `Load body (${fmtBytes(body.size)})`}
    </button>
  </div>
{:else if b64 !== null}
  <BinaryPanel
    base64={b64}
    contentType={contentType}
    size={body.size}
    filename={`${slug}-${which}`}
  />
{:else if text === null}
  <div class="empty at-mono">{emptyText}</div>
{:else if parsed.ok}
  <JsonTree body={parsed.value} {shortcuts} />
{:else}
  <div class="rawwrap">
    <div class="rawbar">
      <div class="searchwrap">
        <span class="searchico"><Icon name="search" size={13} /></span>
        <input
          class="at-input searchinput"
          placeholder="Find in body…"
          aria-label="find in body"
          bind:value={rawQuery}
          on:keydown={(e) => e.key === 'Escape' && (e.stopPropagation(), (rawQuery = ''))}
        />
      </div>
      {#if rq !== ''}
        <span class="at-mono at-xs at-dim">{rawCount} match{rawCount === 1 ? '' : 'es'}</span>
      {/if}
      <span class="sp"></span>
      {#if tooBigForTree}
        <span class="at-mono at-xs at-dim">large body — raw view only</span>
      {/if}
      <button class="at-btn sm ghost" on:click={downloadText}>
        <Icon name="download" size={12} /> Download
      </button>
    </div>
    <div class="rawbody">
      <pre class="rawpre at-mono">{#each rawLines.slice(0, rawShown) as line, i}{#if i > 0}{'\n'}{/if}{#each highlight(line, rq) as seg}{#if seg.hit}<mark>{seg.t}</mark>{:else}{seg.t}{/if}{/each}{/each}</pre>
      {#if rawLines.length > rawShown}
        <button class="jt-more" on:click={() => (rawShown += RAW_CHUNK_LINES)}>
          … show {Math.min(RAW_CHUNK_LINES, rawLines.length - rawShown)} more lines
        </button>
      {/if}
    </div>
  </div>
{/if}

<style>
  .empty {
    padding: var(--pad);
    font-size: var(--fs-sm);
    color: var(--fg3);
  }
  .loadpanel {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: var(--pad);
    align-items: flex-start;
  }
  .meta {
    font-size: var(--fs-sm);
    color: var(--fg1);
    display: flex;
    gap: 8px;
  }
  .dotsep {
    color: var(--fg3);
  }
  .err {
    color: var(--err);
    font-size: var(--fs-xs);
  }
  .rawwrap {
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
  }
  .rawbar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: var(--pad-sm) var(--pad);
    flex: none;
  }
  .searchwrap {
    position: relative;
    width: 240px;
  }
  .searchico {
    position: absolute;
    left: 7px;
    top: 6px;
    color: var(--fg3);
  }
  .searchinput {
    padding-left: 26px;
  }
  .sp {
    flex: 1;
  }
  .rawbody {
    flex: 1;
    overflow: auto;
    padding: 0 var(--pad) var(--pad);
  }
  .rawpre {
    margin: 0;
    font-size: var(--fs-sm);
    line-height: 1.5;
    color: var(--fg1);
    white-space: pre-wrap;
    word-break: break-word;
  }
  mark {
    background: color-mix(in srgb, var(--warn) 30%, transparent);
    color: inherit;
    border-radius: 2px;
  }
</style>

<script lang="ts">
  // Body diff (§10.6.4.5): fetch both (redacted) bodies, canonicalize JSON,
  // Myers line diff, unified rendering with intra-line marks; changes-only
  // toggle persisted; identical-state panel itemizes non-body deltas;
  // >1.5 MB guard with per-side downloads.
  import { getRequestBody, getRequestDetail } from '../../api/runs';
  import { computeDiff, type DiffResult } from '../../diff';
  import { fmtMs } from '../../format';
  import { prefs } from '../../stores/ui';
  import type { ComparePair } from '../../types/compare';
  import Icon from '../atoms/Icon.svelte';

  export let baseRunId: string;
  export let targetRunId: string;
  export let pair: ComparePair;

  type State =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'binary' }
    | { kind: 'ready'; result: DiffResult; changedKeys: number | null };

  let state: State = { kind: 'loading' };
  let lastKey = '';

  $: changesOnly = $prefs.changesOnly;
  $: key = `${baseRunId}/${pair.base?.request_id ?? ''}|${targetRunId}/${pair.target?.request_id ?? ''}|${changesOnly}`;
  $: if (key !== lastKey) {
    lastKey = key;
    void load();
  }

  async function fetchBody(runId: string, requestId: string): Promise<string | null> {
    const detail = await getRequestDetail(runId, requestId);
    const body = detail.response?.body ?? null;
    if (body === null) return '';
    if (body.encoding === 'base64') return null; // binary
    if (body.content !== undefined && !body.truncated) return body.content;
    const res = await getRequestBody(runId, requestId, 'response');
    return res.text();
  }

  async function load(): Promise<void> {
    if (pair.base === null || pair.target === null) return;
    state = { kind: 'loading' };
    const myKey = lastKey;
    try {
      const [a, b] = await Promise.all([
        fetchBody(baseRunId, pair.base.request_id),
        fetchBody(targetRunId, pair.target.request_id),
      ]);
      if (lastKey !== myKey) return;
      if (a === null || b === null) {
        state = { kind: 'binary' };
        return;
      }
      const result = computeDiff(a, b, { changesOnly, context: 2 });
      state = {
        kind: 'ready',
        result,
        changedKeys: result.kind === 'diff' && result.isJson ? countChangedTopKeys(a, b) : null,
      };
    } catch {
      if (lastKey === myKey) state = { kind: 'error', message: 'could not load both bodies' };
    }
  }

  /** Cheap structural walk: changed top-level paths between two JSON bodies. */
  function countChangedTopKeys(aText: string, bText: string): number | null {
    try {
      const a = JSON.parse(aText) as unknown;
      const b = JSON.parse(bText) as unknown;
      if (a === null || b === null || typeof a !== 'object' || typeof b !== 'object') {
        return null;
      }
      const ao = a as Record<string, unknown>;
      const bo = b as Record<string, unknown>;
      const keys = new Set([...Object.keys(ao), ...Object.keys(bo)]);
      let changed = 0;
      for (const k of keys) {
        if (JSON.stringify(ao[k]) !== JSON.stringify(bo[k])) changed++;
      }
      return changed;
    } catch {
      return null;
    }
  }

  function toggleChangesOnly(): void {
    prefs.update((p) => ({ ...p, changesOnly: !p.changesOnly }));
  }

  async function download(which: 'base' | 'target'): Promise<void> {
    const side = which === 'base' ? pair.base : pair.target;
    const runId = which === 'base' ? baseRunId : targetRunId;
    if (side === null) return;
    try {
      const res = await getRequestBody(runId, side.request_id, 'response');
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${pair.slug}-${which}-response`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      // download failed — nothing useful to do
    }
  }

  // Non-body delta itemization for the identical state.
  $: nonBodyDeltas = (() => {
    const out: string[] = [];
    if (pair.delta !== null) {
      out.push(pair.delta.status_changed ? 'status changed' : 'status unchanged');
      out.push(
        pair.delta.duration_ms === 0
          ? 'duration unchanged'
          : `duration ${pair.delta.duration_ms > 0 ? '+' : '−'}${fmtMs(Math.abs(pair.delta.duration_ms))}`,
      );
    }
    return out;
  })();
</script>

<div class="dv">
  <div class="dvhead">
    <span class="dvtitle">Response body diff</span>
    {#if state.kind === 'ready' && state.changedKeys !== null}
      <span class="at-mono keys">{state.changedKeys} keys changed</span>
    {/if}
    <span class="sp"></span>
    {#if state.kind === 'ready' && state.result.kind === 'diff' && !state.result.identical}
      <span class="at-mono totals">
        <span class="del">−{state.result.removed}</span>
        <span class="add">+{state.result.added}</span>
      </span>
    {/if}
    <label class="conly">
      <input type="checkbox" checked={changesOnly} on:change={toggleChangesOnly} />
      changes only
    </label>
  </div>

  {#if state.kind === 'loading'}
    <div class="center"><span class="at-spin"></span></div>
  {:else if state.kind === 'error'}
    <div class="note at-mono">{state.message}</div>
  {:else if state.kind === 'binary'}
    <div class="guard">
      <span>binary bodies can't be diffed</span>
      <div class="dlrow">
        <button class="at-btn sm" on:click={() => void download('base')}>download A</button>
        <button class="at-btn sm" on:click={() => void download('target')}>download B</button>
      </div>
    </div>
  {:else if state.result.kind === 'too_large'}
    <div class="guard">
      <span>bodies too large to diff</span>
      <div class="dlrow">
        <button class="at-btn sm" on:click={() => void download('base')}>download A</button>
        <button class="at-btn sm" on:click={() => void download('target')}>download B</button>
      </div>
    </div>
  {:else if state.result.identical}
    <div class="identical">
      <span class="okico"><Icon name="check" size={13} /></span>
      <span class="idtitle">Response bodies are identical between these runs</span>
      {#if nonBodyDeltas.length > 0}
        <span class="iddetail at-mono">{nonBodyDeltas.join(' · ')}</span>
      {/if}
    </div>
  {:else}
    <div class="diffbody at-mono">
      {#each state.result.hunks as hunk, hi}
        {#if hi > 0}
          <div class="hunksep">⋯</div>
        {/if}
        {#each hunk.lines as line}
          <div class="dline" class:del={line.type === 'del'} class:add={line.type === 'add'}>
            <span class="lna">{line.aLine ?? ''}</span>
            <span class="lnb">{line.bLine ?? ''}</span>
            <span class="sign">{line.type === 'del' ? '−' : line.type === 'add' ? '+' : ''}</span>
            <span class="txt">
              {#if line.markStart !== undefined && line.markEnd !== undefined && line.markEnd > line.markStart}
                {line.text.slice(0, line.markStart)}<mark
                  class:mdel={line.type === 'del'}
                  class:madd={line.type === 'add'}>{line.text.slice(line.markStart, line.markEnd)}</mark
                >{line.text.slice(line.markEnd)}
              {:else}
                {line.text}
              {/if}
            </span>
          </div>
        {/each}
      {/each}
    </div>
  {/if}
</div>

<style>
  .dv {
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    background: var(--bg1);
    overflow: hidden;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
  .dvhead {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    border-bottom: 1px solid var(--bd0);
    background: var(--bg2);
    flex: none;
  }
  .dvtitle {
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg2);
  }
  .keys {
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
  .sp {
    flex: 1;
  }
  .totals {
    font-size: var(--fs-xs);
  }
  .del {
    color: var(--err);
  }
  .add {
    color: var(--ok);
  }
  .conly {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: var(--fs-xs);
    color: var(--fg2);
    cursor: pointer;
    user-select: none;
  }
  .conly input {
    accent-color: var(--acc);
    margin: 0;
  }
  .center {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 30px;
  }
  .note {
    padding: var(--pad);
    font-size: var(--fs-sm);
    color: var(--fg3);
  }
  .guard {
    padding: 28px 20px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    font-size: var(--fs-sm);
    color: var(--fg1);
  }
  .dlrow {
    display: flex;
    gap: 8px;
  }
  .identical {
    border: 1px dashed var(--bd1);
    border-radius: var(--rad);
    margin: var(--pad);
    padding: 28px 20px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
  }
  .okico {
    color: var(--ok);
    display: flex;
  }
  .idtitle {
    font-size: var(--fs-sm);
    color: var(--fg1);
  }
  .iddetail {
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
  .diffbody {
    font-size: var(--fs-sm);
    line-height: 1.5;
    padding: 6px 0;
    overflow: auto;
    min-height: 0;
  }
  .dline {
    display: flex;
    min-height: 19px;
  }
  .dline.del {
    background: color-mix(in srgb, var(--err) 10%, transparent);
  }
  .dline.add {
    background: color-mix(in srgb, var(--ok) 10%, transparent);
  }
  .lna,
  .lnb {
    width: 38px;
    flex: none;
    text-align: right;
    padding-right: 10px;
    color: var(--fg3);
    user-select: none;
  }
  .sign {
    width: 16px;
    flex: none;
    user-select: none;
    color: var(--fg3);
  }
  .dline.del .sign {
    color: var(--err);
  }
  .dline.add .sign {
    color: var(--ok);
  }
  .txt {
    white-space: pre;
    color: var(--fg1);
  }
  mark {
    color: inherit;
    border-radius: 2px;
    padding: 0 1px;
    background: transparent;
  }
  mark.mdel {
    background: color-mix(in srgb, var(--err) 28%, transparent);
  }
  mark.madd {
    background: color-mix(in srgb, var(--ok) 28%, transparent);
  }
  .hunksep {
    text-align: center;
    color: var(--fg3);
    user-select: none;
    padding: 2px 0;
  }
</style>

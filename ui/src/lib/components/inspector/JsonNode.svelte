<script lang="ts" context="module">
  export interface JtCtx {
    matches: Set<string>;
    force: Set<string>;
    overrides: Record<string, boolean>;
    showAll: Record<string, boolean>;
    mode: 'default' | 'all' | 'none';
    currentMatch: string | null;
    onToggle: (path: string, expanded: boolean) => void;
    onShowAll: (path: string) => void;
  }

  export function isExpanded(ctx: JtCtx, path: string, depth: number, v: unknown): boolean {
    if (ctx.force.has(path)) return true;
    if (path in ctx.overrides) return ctx.overrides[path];
    if (ctx.mode === 'all') return true;
    if (ctx.mode === 'none') return depth === 0;
    return depth < 2 && !(Array.isArray(v) && v.length > 10);
  }
</script>

<script lang="ts">
  // Recursive JSON-tree node (§10.6.3.1). Copy-path emits body.$ + JSONPath —
  // matching apitest's assertion syntax. Redacted leaves render the chip.
  import CopyBtn from '../atoms/CopyBtn.svelte';
  import Icon from '../atoms/Icon.svelte';
  import Redacted from '../atoms/Redacted.svelte';

  export let k: string | number | null = null;
  export let v: unknown;
  export let path: string;
  export let depth: number;
  export let ctx: JtCtx;

  const SHOW_LIMIT = 20;

  $: isObj = v !== null && typeof v === 'object';
  $: isArr = Array.isArray(v);
  $: expanded = isObj && isExpanded(ctx, path, depth, v);
  $: hit = ctx.matches.has(path);
  $: current = ctx.currentMatch === path;
  $: indent = `padding-left:${depth * 14 + 8}px`;

  $: entries = isArr
    ? (v as unknown[]).map((x, i) => [i, x] as [number, unknown])
    : isObj
      ? Object.entries(v as Record<string, unknown>)
      : [];
  $: count = entries.length;
  $: truncated = isArr && expanded && count > SHOW_LIMIT && ctx.showAll[path] !== true;
  $: visibleEntries = truncated ? entries.slice(0, SHOW_LIMIT) : entries;

  function childPath(ck: string | number): string {
    return isArr ? `${path}[${ck}]` : `${path}.${ck}`;
  }

  function leafText(value: unknown): { cls: string; text: string } {
    if (value === null) return { cls: 'jt-val-null', text: 'null' };
    if (typeof value === 'string') {
      const s = value.length > 64 ? value.slice(0, 64) + '…' : value;
      return { cls: 'jt-val-str', text: `"${s}"` };
    }
    if (typeof value === 'boolean' || typeof value === 'number') {
      return { cls: 'jt-val-num', text: String(value) };
    }
    return { cls: 'jt-val-str', text: String(value) };
  }
</script>

{#if !isObj}
  <div
    class="jt-row"
    class:jt-hit={hit}
    class:jt-hit-current={current}
    style={indent}
    data-jtpath={path}
    role="treeitem"
    aria-selected={current}
  >
    <span class="pad14"></span>
    {#if k !== null}
      <span class="jt-key"
        >{#if typeof k === 'number'}<span class="jt-pn">{k}</span>{:else}"{k}"{/if}<span
          class="jt-pn">: </span></span
      >
    {/if}
    {#if typeof v === 'string' && v === '[REDACTED]'}
      <Redacted />
    {:else}
      {@const leaf = leafText(v)}
      <span class={leaf.cls}>{leaf.text}</span>
    {/if}
    <span class="jt-copy"><CopyBtn text={'body.' + path} label="path" /></span>
  </div>
{:else}
  <div
    class="jt-row"
    class:jt-hit={hit}
    class:jt-hit-current={current}
    style={indent}
    data-jtpath={path}
    role="treeitem"
    aria-expanded={expanded}
    aria-selected={current}
  >
    <button
      class="jt-chev"
      aria-label={expanded ? 'collapse' : 'expand'}
      on:click={() => ctx.onToggle(path, expanded)}
    >
      <Icon name={expanded ? 'chevron-down' : 'chevron-right'} size={12} />
    </button>
    {#if k !== null}
      <span class="jt-key"
        >{#if typeof k === 'number'}<span class="jt-pn">{k}</span>{:else}"{k}"{/if}<span
          class="jt-pn">: </span></span
      >
    {/if}
    {#if expanded}
      <span class="jt-pn">{isArr ? '[' : '{'}</span>
    {:else}
      <button class="jt-preview" on:click={() => ctx.onToggle(path, false)}>
        {isArr ? `[…] ${count} items` : `{…} ${count} keys`}
      </button>
    {/if}
    <span class="jt-copy"><CopyBtn text={'body.' + path} label="path" /></span>
  </div>
  {#if expanded}
    <div role="group">
      {#each visibleEntries as [ck, cv] (String(ck))}
        <svelte:self k={ck} v={cv} path={childPath(ck)} depth={depth + 1} {ctx} />
      {/each}
      {#if truncated}
        <div style={`padding-left:${(depth + 1) * 14 + 8}px`}>
          <button class="jt-more" on:click={() => ctx.onShowAll(path)}>
            … show {count - SHOW_LIMIT} more items
          </button>
        </div>
      {/if}
      <div class="jt-row" style={indent}>
        <span class="pad14"></span>
        <span class="jt-pn">{isArr ? ']' : '}'}</span>
      </div>
    </div>
  {/if}
{/if}

<style>
  .pad14 {
    width: 14px;
    flex: none;
  }
</style>

<script lang="ts">
  // JSON tree (§10.6.3.1, normative port of the prototype): expansion
  // defaults, search with force-expand + Enter cycling, copy-path (body.$…),
  // Pretty/Raw toggle, arrays > 20 show-more, Redacted leaves.
  import { onDestroy, onMount, tick } from 'svelte';
  import { registerKey } from '../../keyboard';
  import { toast } from '../../stores/toast';
  import CopyBtn from '../atoms/CopyBtn.svelte';
  import Icon from '../atoms/Icon.svelte';
  import JsonNode, { type JtCtx } from './JsonNode.svelte';

  export let body: unknown;
  /** Register global shortcuts (Cmd+F focus, y copy-path). Off in tests. */
  export let shortcuts = true;

  let q = '';
  let raw = false;
  let mode: 'default' | 'all' | 'none' = 'default';
  let overrides: Record<string, boolean> = {};
  let showAll: Record<string, boolean> = {};
  let searchInput: HTMLInputElement;
  let currentMatch: string | null = null;

  $: query = q.trim().toLowerCase();

  interface MatchResult {
    matches: Set<string>;
    force: Set<string>;
    ordered: string[];
  }

  function findMatches(data: unknown, needle: string): MatchResult {
    const matches = new Set<string>();
    const force = new Set<string>();
    const ordered: string[] = [];
    if (needle === '' || data === null || typeof data !== 'object') {
      return { matches, force, ordered };
    }
    const walk = (node: unknown, path: string): boolean => {
      let any = false;
      const entries = Array.isArray(node)
        ? node.map((value, i) => [i, value] as [number, unknown])
        : Object.entries(node as Record<string, unknown>);
      for (const [k, v] of entries) {
        const p = Array.isArray(node) ? `${path}[${k}]` : `${path}.${k}`;
        const isObj = v !== null && typeof v === 'object';
        const hit =
          String(k).toLowerCase().includes(needle) ||
          (!isObj && JSON.stringify(v).toLowerCase().includes(needle));
        if (hit) {
          matches.add(p);
          ordered.push(p);
          any = true;
        }
        if (isObj && walk(v, p)) {
          force.add(p);
          any = true;
        }
      }
      return any;
    };
    walk(data, '$');
    return { matches, force, ordered };
  }

  $: result = findMatches(body, query);
  $: if (query === '') currentMatch = null;

  function cycle(d: number): void {
    if (result.ordered.length === 0) return;
    const i = currentMatch !== null ? result.ordered.indexOf(currentMatch) : -1;
    const next = (i + d + result.ordered.length) % result.ordered.length;
    currentMatch = result.ordered[next];
    void tick().then(() => {
      const el = document.querySelector(`[data-jtpath="${CSS.escape(currentMatch ?? '')}"]`);
      el?.scrollIntoView({ block: 'nearest' });
    });
  }

  function searchKeydown(e: KeyboardEvent): void {
    if (e.key === 'Enter') {
      e.preventDefault();
      cycle(e.shiftKey ? -1 : 1);
    } else if (e.key === 'Escape') {
      e.stopPropagation();
      q = '';
      searchInput.blur();
    }
  }

  $: ctx = {
    matches: result.matches,
    force: result.force,
    overrides,
    showAll,
    mode,
    currentMatch,
    onToggle: (path: string, expanded: boolean) => {
      overrides = { ...overrides, [path]: !expanded };
    },
    onShowAll: (path: string) => {
      showAll = { ...showAll, [path]: true };
    },
  } satisfies JtCtx;

  $: pretty = JSON.stringify(body, null, 2);

  let unregister: Array<() => void> = [];
  onMount(() => {
    if (!shortcuts) return;
    unregister = [
      registerKey('bodySearch', () => {
        searchInput?.focus();
        return true;
      }),
      registerKey('copyJsonPath', () => {
        const path = currentMatch ?? '$';
        try {
          void navigator.clipboard?.writeText('body.' + path);
        } catch {
          // clipboard unavailable
        }
        toast(`copied body.${path}`, { durationMs: 1600 });
      }),
    ];
  });
  onDestroy(() => unregister.forEach((u) => u()));
</script>

<div class="jt">
  <div class="toolbar">
    <div class="searchwrap">
      <span class="searchico"><Icon name="search" size={13} /></span>
      <input
        class="at-input searchinput"
        placeholder="Search keys & values…"
        aria-label="search body"
        bind:value={q}
        bind:this={searchInput}
        on:keydown={searchKeydown}
      />
    </div>
    {#if query !== ''}
      <span class="at-mono at-xs at-dim">
        {result.matches.size} match{result.matches.size === 1 ? '' : 'es'}
      </span>
    {/if}
    <span class="sp"></span>
    <button
      class="at-btn sm ghost"
      on:click={() => {
        mode = 'all';
        overrides = {};
      }}>Expand all</button
    >
    <button
      class="at-btn sm ghost"
      on:click={() => {
        mode = 'none';
        overrides = {};
      }}>Collapse all</button
    >
    <div class="at-seg">
      <button class:on={!raw} on:click={() => (raw = false)}>Pretty</button>
      <button class:on={raw} on:click={() => (raw = true)}>Raw</button>
    </div>
    <CopyBtn text={pretty} label="body" />
  </div>
  <div class="treebody">
    {#if raw}
      <pre class="rawpre at-mono">{pretty}</pre>
    {:else}
      <div role="tree" aria-label="response body">
        <JsonNode v={body} path="$" depth={0} {ctx} />
      </div>
    {/if}
  </div>
</div>

<style>
  .jt {
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
  }
  .toolbar {
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
  .treebody {
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
</style>

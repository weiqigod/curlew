<script lang="ts" context="module">
  export interface PickerEntry {
    /** Selection key: `${slug}|${iteration ?? ''}`. */
    key: string;
    slug: string;
    iteration: number | null;
    name: string;
    method: string;
    kind: 'pair' | 'only_base' | 'only_target';
    changed: boolean;
    glyph: string;
  }
</script>

<script lang="ts">
  // Pair picker (§10.6.4.3): changed-first, then unchanged, then only-in-base
  // / only-in-target sections. Items: Method, name (+#i), delta glyph.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { pushEsc } from '../../keyboard';
  import Icon from '../atoms/Icon.svelte';
  import Method from '../atoms/Method.svelte';

  export let entries: PickerEntry[];
  export let selected: string | null;

  const dispatch = createEventDispatcher<{ pick: string }>();

  let open = false;
  let popEsc: (() => void) | null = null;

  $: current = entries.find((e) => e.key === selected) ?? null;
  $: changed = entries.filter((e) => e.kind === 'pair' && e.changed);
  $: unchanged = entries.filter((e) => e.kind === 'pair' && !e.changed);
  $: onlyBase = entries.filter((e) => e.kind === 'only_base');
  $: onlyTarget = entries.filter((e) => e.kind === 'only_target');

  function toggle(): void {
    if (open) {
      close();
    } else {
      open = true;
      popEsc = pushEsc(close);
    }
  }

  function close(): void {
    open = false;
    popEsc?.();
    popEsc = null;
  }

  function pick(key: string): void {
    dispatch('pick', key);
    close();
  }

  onDestroy(() => popEsc?.());
</script>

<div class="wrap">
  <button class="at-btn" on:click={toggle} aria-haspopup="menu" aria-expanded={open}>
    {#if current !== null}
      <Method m={current.method} />
      <span class="cname">
        {current.name}{current.iteration !== null ? ` #${current.iteration + 1}` : ''}
      </span>
    {:else}
      <span class="cname">select request</span>
    {/if}
    <Icon name="caret" size={12} />
  </button>
  {#if open}
    <div
      class="at-overlay"
      role="presentation"
      on:click={close}
      on:keydown={(e) => e.key === 'Escape' && close()}
    ></div>
    <div class="at-menu menu" role="menu">
      {#each [{ label: changed.length > 0 ? 'changed' : null, list: changed }, { label: unchanged.length > 0 && changed.length > 0 ? 'unchanged' : null, list: unchanged }, { label: onlyBase.length > 0 ? `only in base (${onlyBase.length})` : null, list: onlyBase }, { label: onlyTarget.length > 0 ? `only in target (${onlyTarget.length})` : null, list: onlyTarget }] as section}
        {#if section.label !== null}
          <div class="sect">{section.label}</div>
        {/if}
        {#each section.list as e (e.key)}
          <button class="at-menu-item" role="menuitem" on:click={() => pick(e.key)}>
            <span class="chk">
              {#if e.key === selected}<Icon name="check" size={12} />{/if}
            </span>
            <Method m={e.method} />
            <span class="iname">{e.name}{e.iteration !== null ? ` #${e.iteration + 1}` : ''}</span>
            <span class="hint at-mono">{e.glyph}</span>
          </button>
        {/each}
      {/each}
    </div>
  {/if}
</div>

<style>
  .wrap {
    position: relative;
  }
  .cname {
    font-weight: 600;
  }
  .menu {
    left: 0;
    top: 32px;
    max-height: 360px;
    overflow-y: auto;
    min-width: 300px;
  }
  .sect {
    padding: 5px 8px 3px;
    font: 600 10px var(--font-sans);
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg3);
  }
  .chk {
    width: 12px;
    flex: none;
    display: inline-flex;
  }
  .iname {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>

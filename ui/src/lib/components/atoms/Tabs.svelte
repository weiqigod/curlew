<script lang="ts" context="module">
  export interface TabDef {
    id: string;
    label: string;
    count?: string;
    /** Red count affix (assertions n✗). */
    countErr?: boolean;
    /** Error-tab tint. */
    accent?: boolean;
    disabled?: boolean;
    title?: string;
  }
</script>

<script lang="ts">
  // Tab strip with tablist semantics + arrow-key movement (§10.5.3, §10.8.2).
  import { createEventDispatcher } from 'svelte';

  export let tabs: TabDef[];
  export let active: string;

  const dispatch = createEventDispatcher<{ change: string }>();

  let buttons: Array<HTMLButtonElement | null> = [];

  function select(id: string, disabled: boolean | undefined): void {
    if (disabled) return;
    dispatch('change', id);
  }

  function onKeydown(e: KeyboardEvent, i: number): void {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    e.preventDefault();
    const d = e.key === 'ArrowRight' ? 1 : -1;
    let j = i;
    for (let step = 0; step < tabs.length; step++) {
      j = (j + d + tabs.length) % tabs.length;
      if (!tabs[j].disabled) break;
    }
    buttons[j]?.focus();
    dispatch('change', tabs[j].id);
  }
</script>

<div class="at-tabs" role="tablist">
  {#each tabs as t, i (t.id)}
    <button
      class="at-tab"
      class:active={active === t.id}
      class:accent={t.accent}
      role="tab"
      aria-selected={active === t.id}
      tabindex={active === t.id ? 0 : -1}
      disabled={t.disabled}
      title={t.title}
      bind:this={buttons[i]}
      on:click={() => select(t.id, t.disabled)}
      on:keydown={(e) => onKeydown(e, i)}
    >
      {t.label}
      {#if t.count !== undefined}<span class="n" class:nerr={t.countErr}>{t.count}</span>{/if}
    </button>
  {/each}
</div>

<style>
  .at-tab.accent {
    color: var(--err2);
  }
  .at-tab.accent.active {
    color: var(--err2);
    border-bottom-color: var(--err2);
  }
  .at-tab:disabled {
    color: var(--fg3);
    cursor: default;
  }
  .n.nerr {
    color: var(--err);
  }
</style>

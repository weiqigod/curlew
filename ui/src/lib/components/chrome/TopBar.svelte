<script lang="ts">
  // Top bar (§10.6.1.1): project name · watching indicator · History (3-state,
  // §10.6.1.3) · env switcher · theme/density popover · Run split-button.
  import { onDestroy } from 'svelte';
  import { pushEsc } from '../../keyboard';
  import { navigate, route } from '../../router';
  import { meta } from '../../stores/meta';
  import { prefs, type Prefs } from '../../stores/ui';
  import Icon from '../atoms/Icon.svelte';
  import EnvMenu from './EnvMenu.svelte';
  import RunSplitButton from './RunSplitButton.svelte';
  import WatchIndicator from './WatchIndicator.svelte';

  let prefsOpen = false;
  let popEsc: (() => void) | null = null;

  $: historyConfigDisabled = $meta?.history.enabled === false;
  $: onCompare = $route.name === 'compare';

  function toggleHistory(): void {
    if (onCompare) {
      navigate({ name: 'run' });
    } else {
      navigate({ name: 'compare' });
    }
  }

  function togglePrefs(): void {
    if (prefsOpen) {
      closePrefs();
    } else {
      prefsOpen = true;
      popEsc = pushEsc(closePrefs);
    }
  }

  function closePrefs(): void {
    prefsOpen = false;
    popEsc?.();
    popEsc = null;
  }

  function setPref<K extends keyof Prefs>(key: K, value: Prefs[K]): void {
    prefs.update((p) => ({ ...p, [key]: value }));
  }

  onDestroy(() => popEsc?.());

  const THEMES: Array<Prefs['theme']> = ['dark', 'light', 'system'];
  const DENSITIES: Array<Prefs['density']> = ['dense', 'comfortable', 'compact'];
</script>

<header class="bar">
  <div class="proj">
    <span class="name">{$meta?.project.name ?? 'apitest'}</span>
  </div>

  <WatchIndicator />

  <span class="sp"></span>

  <button
    class="at-btn ghost"
    class:active={onCompare}
    on:click={toggleHistory}
    title={historyConfigDisabled
      ? 'history is disabled (ui.history.enabled: false)'
      : 'run history & compare'}
  >
    <Icon name="clock" size={13} />
    History
  </button>

  <EnvMenu />

  <div class="prefswrap">
    <button
      class="at-btn ghost dots"
      aria-label="theme and density"
      aria-haspopup="menu"
      aria-expanded={prefsOpen}
      on:click={togglePrefs}>⋯</button
    >
    {#if prefsOpen}
      <div
        class="at-overlay"
        role="presentation"
        on:click={closePrefs}
        on:keydown={(e) => e.key === 'Escape' && closePrefs()}
      ></div>
      <div class="at-menu menu" role="menu">
        <div class="grp">theme</div>
        <div class="at-seg seg">
          {#each THEMES as t}
            <button class:on={$prefs.theme === t} on:click={() => setPref('theme', t)}>{t}</button>
          {/each}
        </div>
        <div class="grp">density</div>
        <div class="at-seg seg">
          {#each DENSITIES as d}
            <button class:on={$prefs.density === d} on:click={() => setPref('density', d)}
              >{d}</button
            >
          {/each}
        </div>
      </div>
    {/if}
  </div>

  <RunSplitButton />
</header>

<style>
  .bar {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 44px;
    padding: 0 12px;
    background: var(--bg1);
    border-bottom: 1px solid var(--bd0);
    flex: none;
    position: relative;
    z-index: 30;
  }
  .proj {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .name {
    font-weight: 600;
    font-size: var(--fs-md);
    white-space: nowrap;
  }
  .sp {
    flex: 1;
  }
  .active {
    background: var(--bg3);
    color: var(--fg0);
  }
  .prefswrap {
    position: relative;
  }
  .dots {
    font-weight: 600;
    letter-spacing: 1px;
  }
  .menu {
    right: 0;
    top: 32px;
    min-width: 220px;
    padding: 8px;
  }
  .grp {
    font: 600 var(--fs-xs) var(--font-sans);
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg2);
    margin: 4px 0;
  }
  .seg {
    margin-bottom: 6px;
  }
</style>

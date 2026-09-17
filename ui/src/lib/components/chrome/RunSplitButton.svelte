<script lang="ts">
  // Run split-button (§10.6.1.5): parallel toggle (Professional only), primary
  // Run/Cancel segment, caret menu with the §10.6.1.5 enablement table.
  import { onDestroy, onMount } from 'svelte';
  import { cancelActiveRun, startRun } from '../../controller';
  import { fileBasename } from '../../format';
  import { pushEsc, registerKey } from '../../keyboard';
  import { route } from '../../router';
  import { focusedRequests } from '../../stores/focused-run';
  import { meta } from '../../stores/meta';
  import { selectedEnv } from '../../stores/environments';
  import { runMeta, runState, summary } from '../../stores/run';
  import { tree } from '../../stores/tree';
  import { prefs, runShake, sidebarSelection } from '../../stores/ui';
  import type { StartParams } from '../../types/run';
  import Icon from '../atoms/Icon.svelte';

  let menuOpen = false;
  let popEsc: (() => void) | null = null;
  let shaking = false;
  let shakeTimer: ReturnType<typeof setTimeout> | null = null;

  $: busy = $runState === 'starting' || $runState === 'running' || $runState === 'cancelling';
  $: parallelOn = $prefs.parallel;

  $: validCollections = ($tree?.collections ?? []).filter((c) => c.valid);
  $: totalMain = validCollections.reduce((n, c) => n + (c.counts?.main ?? 0), 0);
  $: runAllEnabled = validCollections.length >= 1;

  // Focused collection: route #/tree/<path>, or the inspector request's file.
  $: focusedCollection = (() => {
    if ($route.name === 'tree') return $route.path;
    if ($route.name === 'inspector') {
      const row = $focusedRequests.find((r) => r.request_id === $route.requestId);
      return row?.source_file ?? null;
    }
    return null;
  })();
  $: focusedCollectionName = fileBasename(focusedCollection ?? '');

  $: selCount = $sidebarSelection.names.length;
  $: terminal = $runState === 'completed' || $runState === 'cancelled' || $runState === 'error';
  $: failedCount = terminal ? $summary.failed + $summary.error : 0;
  $: rerunEnabled = terminal && $runMeta.run_id !== null && failedCount > 0;

  function envName(): string {
    return $selectedEnv ?? $meta?.project.default_env ?? '';
  }

  function baseParams(): Pick<StartParams, 'env' | 'parallel' | 'selection' | 'rerun_of'> {
    return { env: envName(), parallel: parallelOn, selection: null, rerun_of: null };
  }

  function runAll(): void {
    if (busy || !runAllEnabled) {
      shake();
      return;
    }
    const collection = validCollections.length === 1 ? validCollections[0].path : null;
    void startRun({ ...baseParams(), collection, mode: 'all' });
    closeMenu();
  }

  function runCurrent(): void {
    if (busy || focusedCollection === null) return;
    void startRun({ ...baseParams(), collection: focusedCollection, mode: 'all' });
    closeMenu();
  }

  function runSelection(): void {
    if (busy || selCount === 0 || $sidebarSelection.collection === null) return;
    void startRun({
      ...baseParams(),
      collection: $sidebarSelection.collection,
      mode: 'selection',
      selection: $sidebarSelection.names,
    });
    closeMenu();
  }

  function rerunFailed(): void {
    if (busy || !rerunEnabled || $runMeta.run_id === null) {
      shake();
      return;
    }
    void startRun({
      ...baseParams(),
      collection: null,
      mode: 'rerun_failed',
      rerun_of: $runMeta.run_id,
    });
    closeMenu();
  }

  function shake(): void {
    runShake.update((n) => n + 1);
    shaking = true;
    if (shakeTimer !== null) clearTimeout(shakeTimer);
    shakeTimer = setTimeout(() => (shaking = false), 400);
  }

  function toggleMenu(): void {
    if (menuOpen) {
      closeMenu();
    } else if (!busy) {
      menuOpen = true;
      popEsc = pushEsc(closeMenu);
    }
  }

  function closeMenu(): void {
    menuOpen = false;
    popEsc?.();
    popEsc = null;
  }

  let unregister: Array<() => void> = [];
  onMount(() => {
    unregister = [registerKey('runAll', runAll), registerKey('rerunFailed', rerunFailed)];
  });
  onDestroy(() => {
    unregister.forEach((u) => u());
    popEsc?.();
    if (shakeTimer !== null) clearTimeout(shakeTimer);
  });
</script>

<div class="wrap" class:at-shake={shaking}>
  {#if $runState !== 'running' && $runState !== 'cancelling'}
    <button
      class="at-btn seg par"
      class:on={parallelOn}
      title="run requests in parallel waves"
      aria-label="run requests in parallel waves"
      aria-pressed={parallelOn}
      on:click={() => prefs.update((p) => ({ ...p, parallel: !p.parallel }))}
    >
      <Icon name="parallel" size={12} />
    </button>
  {/if}

  {#if $runState === 'running'}
    <button class="at-btn cancel seg-main" on:click={() => void cancelActiveRun()}>
      Cancel
    </button>
  {:else if $runState === 'cancelling'}
    <button class="at-btn cancel seg-main" disabled>Cancelling…</button>
  {:else if $runState === 'starting'}
    <button class="at-btn primary seg-main" disabled>
      <span class="at-spin btn-spin"></span>
      Starting…
    </button>
  {:else}
    <button
      class="at-btn primary seg-main"
      disabled={!runAllEnabled}
      title={runAllEnabled ? undefined : 'no collections'}
      on:click={runAll}
    >
      <Icon name="play" size={12} />
      Run all
    </button>
  {/if}

  <button
    class="at-btn primary seg-caret"
    class:cancelcaret={$runState === 'running' || $runState === 'cancelling'}
    aria-label="run options"
    aria-haspopup="menu"
    aria-expanded={menuOpen}
    disabled={busy}
    on:click={toggleMenu}
  >
    <Icon name="caret" size={12} />
  </button>

  {#if menuOpen}
    <div
      class="at-overlay"
      role="presentation"
      on:click={closeMenu}
      on:keydown={(e) => e.key === 'Escape' && closeMenu()}
    ></div>
    <div class="at-menu menu" role="menu">
      <button class="at-menu-item" role="menuitem" disabled={!runAllEnabled} on:click={runAll}>
        Run all
        <span class="hint at-mono">{totalMain} requests</span>
      </button>
      <button
        class="at-menu-item"
        role="menuitem"
        disabled={focusedCollection === null}
        on:click={runCurrent}
      >
        Run current collection
        {#if focusedCollection !== null}<span class="hint at-mono">{focusedCollectionName}</span>{/if}
      </button>
      <button class="at-menu-item" role="menuitem" disabled={selCount === 0} on:click={runSelection}>
        Run selection
        <span class="hint at-mono">
          {selCount > 0 ? `${selCount} selected` : 'select requests in the sidebar'}
        </span>
      </button>
      <button class="at-menu-item" role="menuitem" disabled={!rerunEnabled} on:click={rerunFailed}>
        Re-run failed
        {#if failedCount > 0}<span class="hint at-mono">{failedCount} failed</span>{/if}
      </button>
    </div>
  {/if}
</div>

<style>
  .wrap {
    display: flex;
    position: relative;
  }
  .seg.par {
    border-radius: var(--rad) 0 0 var(--rad);
    border-right: none;
    padding: 0 7px;
    color: var(--fg2);
  }
  .seg.par.on {
    color: var(--fg0);
    background: var(--bg4);
  }
  .seg-main {
    border-radius: 0;
  }
  .wrap > :first-child {
    border-top-left-radius: var(--rad);
    border-bottom-left-radius: var(--rad);
  }
  .seg-caret {
    border-radius: 0 var(--rad) var(--rad) 0;
    border-left: 1px solid color-mix(in srgb, var(--acc-fg) 25%, transparent);
    padding: 0 5px;
  }
  .seg-caret:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .cancel {
    color: var(--err);
    border-color: color-mix(in srgb, var(--err) 50%, transparent);
    background: transparent;
    font-weight: 600;
  }
  .cancel:hover {
    background: color-mix(in srgb, var(--err) 10%, transparent);
  }
  .cancelcaret {
    background: transparent;
    color: var(--fg2);
    border-color: var(--bd1);
  }
  .btn-spin {
    border-top-color: var(--acc-fg);
  }
  .seg-main:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .menu {
    right: 0;
    top: 32px;
    min-width: 250px;
  }
</style>

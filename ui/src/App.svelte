<script lang="ts">
  // App shell (§10.6): TopBar + WsBanner + Sidebar + route outlet, plus the
  // global overlays (help, gate modal, toasts), the narrow-viewport notice,
  // theme/density plumbing and the global keyboard listener.
  import { onDestroy, onMount, tick } from 'svelte';
  import { initKeyboard } from './lib/keyboard';
  import { booted } from './lib/controller';
  import { route } from './lib/router';
  import { disconnected, serverReachable } from './lib/stores/connection';
  import { tree } from './lib/stores/tree';
  import { helpOpen, prefs } from './lib/stores/ui';
  import HelpOverlay from './lib/components/chrome/HelpOverlay.svelte';
  import Sidebar from './lib/components/chrome/Sidebar.svelte';
  import Toasts from './lib/components/chrome/Toasts.svelte';
  import TopBar from './lib/components/chrome/TopBar.svelte';
  import WsBanner from './lib/components/chrome/WsBanner.svelte';
  import Compare from './lib/components/compare/Compare.svelte';
  import Inspector from './lib/components/inspector/Inspector.svelte';
  import RunView from './lib/components/run/RunView.svelte';
  import Disconnected from './lib/components/states/Disconnected.svelte';
  import EmptyProject from './lib/components/states/EmptyProject.svelte';
  import DefinitionPanel from './lib/components/states/DefinitionPanel.svelte';
  import NotFoundPanel from './lib/components/states/NotFoundPanel.svelte';
  import ValidationPanel from './lib/components/states/ValidationPanel.svelte';

  // ---- theme (dark | light | system, §10.8.1) ------------------------------
  let systemLight = false;
  let mql: MediaQueryList | null = null;
  const onMql = (e: MediaQueryListEvent) => (systemLight = e.matches);

  $: effectiveTheme =
    $prefs.theme === 'system' ? (systemLight ? 'light' : 'dark') : $prefs.theme;

  // ---- keyboard ------------------------------------------------------------
  let teardownKeyboard: (() => void) | null = null;

  // ---- focus the screen heading on route changes (§10.7) -------------------
  let lastRouteName = '';
  $: if ($route.name !== lastRouteName) {
    lastRouteName = $route.name;
    if ($route.name !== 'inspector') {
      void tick().then(() => {
        document.querySelector<HTMLElement>('main [tabindex="-1"]')?.focus();
      });
    }
  }

  $: emptyProject = $tree !== null && $tree.collections.length === 0;

  onMount(() => {
    teardownKeyboard = initKeyboard();
    mql = window.matchMedia('(prefers-color-scheme: light)');
    systemLight = mql.matches;
    mql.addEventListener('change', onMql);
  });
  onDestroy(() => {
    teardownKeyboard?.();
    mql?.removeEventListener('change', onMql);
  });
</script>

{#if $disconnected || !$serverReachable}
  <Disconnected />
{:else}
  <div
    class="at-root shell"
    data-theme={effectiveTheme === 'light' ? 'light' : undefined}
    data-density={$prefs.density}
  >
    {#if !$booted}
      <div class="boot"><span class="at-spin"></span></div>
    {:else}
      <TopBar />
      <WsBanner />
      <div class="body">
        {#if !emptyProject}
          <Sidebar />
        {/if}
        <main class="outlet">
          {#if emptyProject && ($route.name === 'run' || $route.name === 'tree')}
            <EmptyProject />
          {:else if $route.name === 'run' || $route.name === 'tree' || $route.name === 'runDetail'}
            <RunView />
          {:else if $route.name === 'inspector'}
            <Inspector runId={$route.runId} requestId={$route.requestId} tab={$route.tab} />
          {:else if $route.name === 'compare'}
            <Compare
              base={$route.base}
              target={$route.target}
              slug={$route.slug}
              iter={$route.iter}
              changes={$route.changes}
            />
          {:else if $route.name === 'file'}
            <ValidationPanel path={$route.path} />
          {:else if $route.name === 'definition'}
            <DefinitionPanel path={$route.path} slug={$route.slug} />
          {:else}
            <NotFoundPanel />
          {/if}
        </main>
      </div>

      {#if $helpOpen}
        <HelpOverlay />
      {/if}

      <Toasts />

      <div class="narrow at-root" data-theme={effectiveTheme === 'light' ? 'light' : undefined}>
        <div class="narrowbox">
          <div class="ntitle">curlew ui needs more room</div>
          <div class="nsub">widen this window to at least 960 px</div>
        </div>
      </div>
    {/if}
  </div>
{/if}

<style>
  .shell {
    height: 100vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .boot {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .body {
    display: flex;
    flex: 1;
    min-height: 0;
  }
  .outlet {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--bg0);
  }
  /* narrow-viewport notice (§10.6) */
  .narrow {
    display: none;
  }
  @media (max-width: 959px) {
    .narrow {
      display: flex;
      position: fixed;
      inset: 0;
      z-index: 90;
      align-items: center;
      justify-content: center;
      background: var(--bg0);
    }
  }
  .narrowbox {
    text-align: center;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .ntitle {
    font: 600 var(--fs-lg) var(--font-sans);
  }
  .nsub {
    font-size: var(--fs-sm);
    color: var(--fg2);
  }
</style>

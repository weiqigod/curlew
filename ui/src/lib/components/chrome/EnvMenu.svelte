<script lang="ts">
  // Env switcher (§10.6.1.4): menu from GET /environments with a variables
  // flyout (hover/focus 300 ms or →); sensitive values render Redacted chips.
  import { onDestroy } from 'svelte';
  import { pushEsc } from '../../keyboard';
  import { environments, selectedEnv } from '../../stores/environments';
  import type { Environment } from '../../types/tree';
  import Icon from '../atoms/Icon.svelte';
  import Redacted from '../atoms/Redacted.svelte';

  let open = false;
  let flyoutEnv: Environment | null = null;
  let hoverTimer: ReturnType<typeof setTimeout> | null = null;
  let trigger: HTMLButtonElement;
  let popEsc: (() => void) | null = null;

  function baseUrlOf(env: Environment): string {
    const v = env.variables.find((x) => x.name === 'base_url');
    return v !== undefined ? v.value : env.file;
  }

  function toggle(): void {
    if (open) {
      close();
    } else {
      openMenu();
    }
  }

  function openMenu(): void {
    open = true;
    popEsc = pushEsc(close);
  }

  function close(): void {
    open = false;
    flyoutEnv = null;
    clearHover();
    popEsc?.();
    popEsc = null;
    trigger?.focus();
  }

  function clearHover(): void {
    if (hoverTimer !== null) clearTimeout(hoverTimer);
    hoverTimer = null;
  }

  function scheduleFlyout(env: Environment): void {
    clearHover();
    hoverTimer = setTimeout(() => (flyoutEnv = env), 300);
  }

  function pick(env: Environment): void {
    selectedEnv.set(env.name);
    close();
  }

  function rowKeydown(e: KeyboardEvent, env: Environment): void {
    if (e.key === 'ArrowRight') {
      e.preventDefault();
      clearHover();
      flyoutEnv = env;
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      flyoutEnv = null;
    }
  }

  onDestroy(() => {
    clearHover();
    popEsc?.();
  });
</script>

<div class="wrap">
  <button
    class="at-btn"
    bind:this={trigger}
    on:click={toggle}
    title="environments/*.yaml"
    aria-haspopup="menu"
    aria-expanded={open}
  >
    <span class="envlbl">env</span>
    <span class="at-mono envname">{$selectedEnv ?? '—'}</span>
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
      {#each $environments as env (env.name)}
        <button
          class="at-menu-item"
          role="menuitem"
          on:click={() => pick(env)}
          on:mouseenter={() => scheduleFlyout(env)}
          on:mouseleave={clearHover}
          on:focus={() => scheduleFlyout(env)}
          on:blur={clearHover}
          on:keydown={(e) => rowKeydown(e, env)}
        >
          <span class="chk">
            {#if $selectedEnv === env.name}<Icon name="check" size={12} />{/if}
          </span>
          <span class="at-mono name">{env.name}</span>
          <span class="hint at-mono">{baseUrlOf(env)}</span>
        </button>
      {:else}
        <div class="none at-mono">no environments found</div>
      {/each}
    </div>
    {#if flyoutEnv !== null}
      <div class="at-menu flyout" role="presentation" on:mouseenter={clearHover}>
        <div class="fly-title at-mono">{flyoutEnv.name}</div>
        <div class="at-kv vars">
          {#each flyoutEnv.variables as v (v.name)}
            <div class="k">{v.name}</div>
            <div>
              {#if v.value === '[REDACTED]'}<Redacted />{:else}{v.value}{/if}
            </div>
          {/each}
        </div>
        <div class="fly-foot at-mono">{flyoutEnv.file}</div>
      </div>
    {/if}
  {/if}
</div>

<style>
  .wrap {
    position: relative;
  }
  .envlbl {
    color: var(--fg2);
    font-weight: 400;
  }
  .envname {
    font-size: var(--fs-xs);
  }
  .menu {
    right: 0;
    top: 32px;
    min-width: 220px;
    z-index: 50;
  }
  .chk {
    width: 12px;
    flex: none;
    display: inline-flex;
  }
  .name {
    font-size: var(--fs-sm);
  }
  .none {
    padding: 6px 8px;
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
  .flyout {
    right: 226px;
    top: 32px;
    width: 320px;
    max-height: 320px;
    overflow-y: auto;
    z-index: 51;
    padding: 0;
  }
  .fly-title {
    padding: 7px 10px 5px;
    font-size: var(--fs-xs);
    font-weight: 600;
    color: var(--fg2);
    border-bottom: 1px solid var(--bd0);
  }
  .vars {
    grid-template-columns: 130px 1fr;
    font-size: var(--fs-xs);
  }
  .fly-foot {
    padding: 6px 10px;
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
</style>

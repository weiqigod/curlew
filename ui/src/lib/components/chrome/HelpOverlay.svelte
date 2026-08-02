<script lang="ts">
  // Keyboard help overlay (§10.7): centered sheet, .at-kbd keycaps, grouped
  // Run / Navigate / Inspector / Compare. Esc or ? closes; focus-trapped.
  import { onDestroy, onMount } from 'svelte';
  import { pushEsc } from '../../keyboard';
  import { helpOpen } from '../../stores/ui';

  interface Row {
    keys: string[];
    desc: string;
  }
  interface Group {
    title: string;
    rows: Row[];
  }

  const GROUPS: Group[] = [
    {
      title: 'Run',
      rows: [
        { keys: ['r'], desc: 'run all' },
        { keys: ['⇧', 'R'], desc: 're-run failed' },
        { keys: ['c'], desc: 'cancel run (hold)' },
      ],
    },
    {
      title: 'Navigate',
      rows: [
        { keys: ['/'], desc: 'filter requests' },
        { keys: ['j', 'k'], desc: 'move row cursor' },
        { keys: ['Enter'], desc: 'open in inspector' },
        { keys: ['g', 'h'], desc: 'go to history' },
        { keys: ['g', 'r'], desc: 'go to run view' },
        { keys: ['Esc'], desc: 'close / back / clear' },
      ],
    },
    {
      title: 'Inspector',
      rows: [
        { keys: ['j', 'k'], desc: 'next / previous result' },
        { keys: ['1', '…', '6'], desc: 'switch tabs' },
        { keys: ['⌘', 'F'], desc: 'search body' },
        { keys: ['y'], desc: 'copy JSON path' },
        { keys: ['e'], desc: 'open in editor' },
      ],
    },
    {
      title: 'Compare',
      rows: [
        { keys: ['['], desc: 'previous pair' },
        { keys: [']'], desc: 'next pair' },
      ],
    },
  ];

  let sheet: HTMLElement;
  let previousFocus: Element | null = null;
  let popEsc: (() => void) | null = null;

  function close(): void {
    helpOpen.set(false);
  }

  function trap(e: KeyboardEvent): void {
    if (e.key !== 'Tab') return;
    const focusables = sheet.querySelectorAll<HTMLElement>('button, a[href]');
    if (focusables.length === 0) return;
    const first = focusables[0];
    const last = focusables[focusables.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  onMount(() => {
    previousFocus = document.activeElement;
    popEsc = pushEsc(close);
    sheet.querySelector<HTMLElement>('button')?.focus();
  });

  onDestroy(() => {
    popEsc?.();
    if (previousFocus instanceof HTMLElement) previousFocus.focus();
  });
</script>

<svelte:window on:keydown={trap} />

<button class="backdrop" aria-label="close help" tabindex="-1" on:click={close}></button>
<div
  class="sheet at-menu"
  role="dialog"
  aria-modal="true"
  aria-label="keyboard shortcuts"
  bind:this={sheet}
>
  <div class="head">
    <span class="title">Keyboard shortcuts</span>
    <button class="at-btn sm ghost" on:click={close} aria-label="close help">esc</button>
  </div>
  <div class="cols">
    {#each GROUPS as g}
      <div class="grp">
        <div class="gtitle">{g.title}</div>
        {#each g.rows as r}
          <div class="row">
            <span class="keys">
              {#each r.keys as k}<kbd class="at-kbd">{k}</kbd>{/each}
            </span>
            <span class="desc">{r.desc}</span>
          </div>
        {/each}
      </div>
    {/each}
  </div>
</div>

<style>
  .backdrop {
    all: unset;
    position: fixed;
    inset: 0;
    z-index: 60;
    background: rgba(0, 0, 0, 0.4);
    cursor: default;
  }
  .sheet {
    position: fixed;
    z-index: 61;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: 560px;
    max-width: calc(100vw - 40px);
    max-height: calc(100vh - 80px);
    overflow-y: auto;
    padding: 14px 16px;
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
  }
  .title {
    font: 600 var(--fs-lg) var(--font-sans);
  }
  .cols {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 14px 24px;
  }
  .gtitle {
    font: 600 var(--fs-xs) var(--font-sans);
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg2);
    margin-bottom: 6px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 24px;
  }
  .keys {
    display: inline-flex;
    gap: 3px;
    min-width: 76px;
  }
  .desc {
    font-size: var(--fs-sm);
    color: var(--fg1);
  }
</style>

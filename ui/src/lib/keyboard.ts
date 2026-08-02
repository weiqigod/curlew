// Global keyboard model (UI_SPECIFICATION.md §10.7): one listener, a named
// handler registry that components attach to while mounted, and a layered
// Esc stack (menus/overlays close innermost-first before zone fallbacks).

import { get } from 'svelte/store';
import { cancelActiveRun } from './controller';
import { navigate, route } from './router';
import { runState } from './stores/run';
import { toast } from './stores/toast';
import { helpOpen } from './stores/ui';

export type KeyHandlerName =
  | 'runAll' // returns false when disabled → Run-button shake
  | 'rerunFailed'
  | 'focusFilter'
  | 'bodySearch' // returns true when handled (suppress browser find)
  | 'listMove' // arg: +1 | -1
  | 'listOpen'
  | 'listOpenEditor'
  | 'inspectorNav' // arg: +1 | -1
  | 'inspectorBack'
  | 'selectTab' // arg: 1-based displayed tab index
  | 'openEditor'
  | 'copyJsonPath'
  | 'comparePair'; // arg: +1 | -1

type Handler = (arg?: number) => boolean | void;

const registry = new Map<KeyHandlerName, Handler>();

/** Registers a named handler; returns the unregister function. */
export function registerKey(name: KeyHandlerName, fn: Handler): () => void {
  registry.set(name, fn);
  return () => {
    if (registry.get(name) === fn) registry.delete(name);
  };
}

function call(name: KeyHandlerName, arg?: number): boolean | void {
  return registry.get(name)?.(arg);
}

// Layered Esc: overlays push a closer; Esc pops the innermost first.
const escStack: Array<() => void> = [];

/** Pushes an Esc closer (menus, overlays); returns the pop function. */
export function pushEsc(close: () => void): () => void {
  escStack.push(close);
  return () => {
    const i = escStack.indexOf(close);
    if (i >= 0) escStack.splice(i, 1);
  };
}

/** Fallback Esc handlers below the overlay stack (selection clear, etc.). */
const escFallbacks: Array<() => boolean> = [];

export function pushEscFallback(fn: () => boolean): () => void {
  escFallbacks.unshift(fn); // newest first
  return () => {
    const i = escFallbacks.indexOf(fn);
    if (i >= 0) escFallbacks.splice(i, 1);
  };
}

function handleEscape(): boolean {
  const top = escStack.pop();
  if (top !== undefined) {
    top();
    return true;
  }
  for (const fn of escFallbacks) {
    if (fn()) return true;
  }
  if (get(route).name === 'inspector') {
    call('inspectorBack');
    return true;
  }
  return false;
}

function isTextTarget(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false;
  return t.closest('input, textarea, select, [contenteditable="true"]') !== null;
}

const HOLD_MS = 200;

/** Installs the global listener; returns the teardown. */
export function initKeyboard(): () => void {
  let gPending = false;
  let gTimer: ReturnType<typeof setTimeout> | null = null;
  let cTimer: ReturnType<typeof setTimeout> | null = null;
  let cFired = false;

  const clearG = () => {
    gPending = false;
    if (gTimer !== null) clearTimeout(gTimer);
    gTimer = null;
  };

  const onKeydown = (e: KeyboardEvent) => {
    const $route = get(route);

    if (e.key === 'Escape') {
      if (e.defaultPrevented) return;
      if (handleEscape()) e.preventDefault();
      return;
    }
    if (isTextTarget(e.target)) return; // shortcuts suppressed in inputs (except Esc)

    // Ctrl/Cmd+F → body search inside the inspector (§10.7).
    if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.key === 'F')) {
      if ($route.name === 'inspector' && call('bodySearch') === true) {
        e.preventDefault();
      }
      return;
    }
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    if (get(helpOpen) && e.key !== '?') return; // help overlay is focus-trapped

    // g-chords
    if (gPending) {
      clearG();
      if (e.key === 'h') {
        navigate({ name: 'compare' });
        e.preventDefault();
        return;
      }
      if (e.key === 'r') {
        navigate({ name: 'run' });
        e.preventDefault();
        return;
      }
      // fall through: unknown chord second key is processed normally
    }

    switch (e.key) {
      case '?':
        helpOpen.update((v) => !v);
        e.preventDefault();
        return;
      case 'g':
        gPending = true;
        gTimer = setTimeout(clearG, 600);
        return;
      case 'r':
        call('runAll');
        e.preventDefault();
        return;
      case 'R':
        call('rerunFailed');
        e.preventDefault();
        return;
      case 'c':
        // Hold-to-confirm cancel (200 ms) while a run is active.
        if (get(runState) === 'running' && !e.repeat && cTimer === null) {
          cFired = false;
          cTimer = setTimeout(() => {
            cTimer = null;
            cFired = true;
            void cancelActiveRun();
          }, HOLD_MS);
        }
        return;
      case '/':
        call('focusFilter');
        e.preventDefault();
        return;
      case 'j':
      case 'k': {
        const d = e.key === 'j' ? 1 : -1;
        if ($route.name === 'inspector') call('inspectorNav', d);
        else call('listMove', d);
        e.preventDefault();
        return;
      }
      case 'ArrowDown':
      case 'ArrowUp':
        if ($route.name !== 'inspector' && isListZone($route.name)) {
          call('listMove', e.key === 'ArrowDown' ? 1 : -1);
          e.preventDefault();
        }
        return;
      case 'Enter':
      case 'o':
        if (e.key === 'Enter' && targetIsActivatable(e.target)) return;
        if ($route.name !== 'inspector' && isListZone($route.name)) {
          call('listOpen');
          e.preventDefault();
        }
        return;
      case '1':
      case '2':
      case '3':
      case '4':
      case '5':
      case '6':
        if ($route.name === 'inspector') {
          call('selectTab', Number(e.key));
          e.preventDefault();
        }
        return;
      case 'e':
        if ($route.name === 'inspector') call('openEditor');
        else call('listOpenEditor');
        return;
      case 'y':
        if ($route.name === 'inspector') call('copyJsonPath');
        return;
      case '[':
      case ']':
        if ($route.name === 'compare') {
          call('comparePair', e.key === ']' ? 1 : -1);
          e.preventDefault();
        }
        return;
    }
  };

  const onKeyup = (e: KeyboardEvent) => {
    if (e.key === 'c' && cTimer !== null) {
      clearTimeout(cTimer);
      cTimer = null;
      if (!cFired && get(runState) === 'running') {
        toast('hold c to cancel', { durationMs: 2200 });
      }
    }
  };

  window.addEventListener('keydown', onKeydown);
  window.addEventListener('keyup', onKeyup);
  return () => {
    window.removeEventListener('keydown', onKeydown);
    window.removeEventListener('keyup', onKeyup);
    clearG();
    if (cTimer !== null) clearTimeout(cTimer);
  };
}

function isListZone(routeName: string): boolean {
  return routeName === 'run' || routeName === 'tree' || routeName === 'runDetail';
}

function targetIsActivatable(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false;
  return t.closest('button, a, summary') !== null;
}

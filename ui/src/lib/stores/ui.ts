// Client prefs + chrome state (UI_SPECIFICATION.md §10.3.1).
// Prefs persist to localStorage['curlew.prefs'] as JSON.

import { writable } from 'svelte/store';
import type { Route } from '../router';

export interface Prefs {
  theme: 'dark' | 'light' | 'system';
  density: 'compact' | 'dense' | 'comfortable';
  runLayout: 'compact' | 'columns' | 'lanes';
  changesOnly: boolean;
  parallel: boolean;
}

const PREFS_KEY = 'curlew.prefs';

export const DEFAULT_PREFS: Prefs = {
  theme: 'dark',
  density: 'dense',
  runLayout: 'compact',
  changesOnly: false,
  parallel: false,
};

function loadPrefs(): Prefs {
  try {
    const raw = localStorage.getItem(PREFS_KEY);
    if (raw === null) return { ...DEFAULT_PREFS };
    return { ...DEFAULT_PREFS, ...(JSON.parse(raw) as Partial<Prefs>) };
  } catch {
    return { ...DEFAULT_PREFS };
  }
}

export const prefs = writable<Prefs>(loadPrefs());

prefs.subscribe((p) => {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(p));
  } catch {
    // quota exceeded / storage unavailable — prefs just won't persist
  }
});

export const helpOpen = writable(false);

export type FocusZone = 'sidebar' | 'list' | 'inspector' | 'compare' | 'none';
export const focusZone = writable<FocusZone>('list');

/** Sidebar selection mode (§10.6.1.6) — scoped to ONE collection. */
export interface SidebarSelection {
  collection: string | null;
  names: string[];
}
export const sidebarSelection = writable<SidebarSelection>({ collection: null, names: [] });

export function clearSelection(): void {
  sidebarSelection.set({ collection: null, names: [] });
}

/** Watch-indicator pulse on files.changed (§10.6.1.2). seq dedupes pulses. */
export interface WatchEvent {
  label: string;
  seq: number;
}
export const watchEvent = writable<WatchEvent | null>(null);

/** Last files.changed frame — the validation panel refetches off this. */
export const lastFilesChanged = writable<{ paths: string[]; seq: number }>({ paths: [], seq: 0 });

/** Run-view list cursor (request_id) — preserved across inspector visits. */
export const listCursor = writable<string | null>(null);

/** Increment to trigger the Run-button shake (disabled `r` no-op, §10.7). */
export const runShake = writable(0);

/** Route the inspector returns to on Esc/back (preserves tree filters). */
export const returnRoute = writable<Route | null>(null);

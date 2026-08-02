// Compare screen state (UI_SPECIFICATION.md §10.3.1): base/target ids and the
// server-aligned /compare result. Body diffs are computed client-side by the
// Compare screen (phase 2) via lib/diff.ts.

import { writable } from 'svelte/store';
import { getCompare } from '../api/compare';
import type { CompareResult } from '../types/compare';

export interface CompareSelection {
  base: string | null;
  target: string | null;
}

export const compareSelection = writable<CompareSelection>({ base: null, target: null });

export const comparePairs = writable<CompareResult | null>(null);

export async function loadCompare(base: string, target: string): Promise<void> {
  comparePairs.set(await getCompare(base, target));
}

// Run history — list (UI_SPECIFICATION.md §10.3.1).

import { writable } from 'svelte/store';
import { listRuns } from '../api/runs';
import type { RunMeta } from '../types/run';

export const runs = writable<RunMeta[]>([]);
export const historyTotal = writable(0);

export async function loadHistory(limit = 50, offset = 0): Promise<void> {
  const res = await listRuns(limit, offset);
  if (offset === 0) {
    runs.set(res.runs);
  } else {
    runs.update((existing) => [...existing, ...res.runs]);
  }
  historyTotal.set(res.total);
}

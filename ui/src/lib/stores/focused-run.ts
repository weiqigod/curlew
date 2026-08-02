// "The run in focus" (§10.4.2): the session (live/last) run on #/, or a
// pinned run on #/runs/<id>. Pinned runs that are not the session run are
// fetched into a side store so live WS events never fight the view.

import { derived, writable } from 'svelte/store';
import { getRun, getRunRequests } from '../api/runs';
import { seedFromList, type LiveRequest, type RequestMap } from '../event-reducer';
import { route } from '../router';
import type { RunInfo, RunSummary } from '../types/run';
import { lastRunInfo, requests, runMeta, summary, type SummaryView } from './run';

export interface PinnedRun {
  runId: string;
  info: RunInfo;
  requests: RequestMap;
}

export const pinnedRun = writable<PinnedRun | null>(null);

/** Run id that failed to load (expired ring run etc.) → NotFoundPanel. */
export const pinnedRunError = writable<string | null>(null);

/** The run id the current route is looking at (null = session run). */
export const routeRunId = derived(route, ($route): string | null => {
  if ($route.name === 'runDetail' || $route.name === 'inspector') return $route.runId;
  return null;
});

/** Loads a pinned run; no-op when it is the session run (live stores win). */
export async function loadPinnedRun(runId: string, sessionRunId: string | null): Promise<void> {
  if (runId === sessionRunId) {
    pinnedRun.set(null);
    pinnedRunError.set(null);
    return;
  }
  try {
    const [info, list] = await Promise.all([getRun(runId), getRunRequests(runId)]);
    pinnedRun.set({ runId, info, requests: seedFromList(list.requests) });
    pinnedRunError.set(null);
  } catch {
    pinnedRun.set(null);
    pinnedRunError.set(runId);
  }
}

/** True when the route pins a run other than the session run. */
export const viewingPastRun = derived(
  [routeRunId, runMeta],
  ([$routeRunId, $runMeta]) => $routeRunId !== null && $routeRunId !== $runMeta.run_id,
);

function summaryFromRunSummary(s: RunSummary): SummaryView {
  const view: SummaryView = {
    total: s.total,
    pending: 0,
    running: 0,
    passed: s.passed,
    failed: s.failed,
    skipped: s.skipped,
    error: s.error,
    completed: s.passed + s.failed + s.skipped + s.error,
    live: false,
    duration_ms: s.duration_ms,
    parallel: s.parallel,
  };
  if (s.wave_count !== undefined) view.wave_count = s.wave_count;
  if (s.max_parallelism !== undefined) view.max_parallelism = s.max_parallelism;
  if (s.wave_durations_ms !== undefined) view.wave_durations_ms = s.wave_durations_ms;
  return view;
}

/** Ordered rows of the run in focus (REST order — maps are insertion-ordered). */
export const focusedRequests = derived(
  [routeRunId, runMeta, requests, pinnedRun],
  ([$routeRunId, $runMeta, $requests, $pinned]): LiveRequest[] => {
    if ($routeRunId !== null && $routeRunId !== $runMeta.run_id) {
      return $pinned !== null && $pinned.runId === $routeRunId
        ? Array.from($pinned.requests.values())
        : [];
    }
    return Array.from($requests.values());
  },
);

/** Summary of the run in focus (live-derived or authoritative). */
export const focusedSummary = derived(
  [routeRunId, runMeta, summary, pinnedRun],
  ([$routeRunId, $runMeta, $summary, $pinned]): SummaryView => {
    if (
      $routeRunId !== null &&
      $routeRunId !== $runMeta.run_id &&
      $pinned !== null &&
      $pinned.runId === $routeRunId
    ) {
      return summaryFromRunSummary($pinned.info.summary);
    }
    return $summary;
  },
);

/** RunInfo of the run in focus (env echo, created_at, parallel flag). */
export const focusedRunInfo = derived(
  [routeRunId, runMeta, lastRunInfo, pinnedRun],
  ([$routeRunId, $runMeta, $lastInfo, $pinned]): RunInfo | null => {
    if ($routeRunId !== null && $routeRunId !== $runMeta.run_id) {
      return $pinned !== null && $pinned.runId === $routeRunId ? $pinned.info : null;
    }
    return $lastInfo;
  },
);

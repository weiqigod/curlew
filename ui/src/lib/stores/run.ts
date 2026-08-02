// Run lifecycle machine + per-request map + summary (UI_SPECIFICATION.md
// §10.3.1–§10.3.4). The stores here are thin state holders around the pure
// run-machine and event-reducer modules; orchestration (POST /runs, WS
// subscribe, reconcile fetches) is wired by the screens in phase 2.

import { derived, readable, writable } from 'svelte/store';
import { seedFromList, applyEvent, type RequestMap } from '../event-reducer';
import { transition, type RunAction, type RunUiState } from '../run-machine';
import type { EventError, RunEvent } from '../types/events';
import type { RequestListEntry, RunInfo, RunSummary, StartParams } from '../types/run';

export const runState = writable<RunUiState>('idle');

export function dispatchRun(action: RunAction): void {
  runState.update((s) => transition(s, action));
}

/** Map<request_id, LiveRequest> driven by the event reducer. */
export const requests = writable<RequestMap>(new Map());

/** Seeds (or wholesale replaces, at reconcile) the map from the REST light list. */
export function seedRequests(entries: RequestListEntry[]): void {
  requests.set(seedFromList(entries));
}

/** Meta about the run in focus (id + run.error payload, §10.3.3). */
export interface RunMetaState {
  run_id: string | null;
  error: EventError | null;
  /** The params the session run was started/adopted with (env echo etc.). */
  params: StartParams | null;
}

export const runMeta = writable<RunMetaState>({ run_id: null, error: null, params: null });

/**
 * Elapsed-clock basis (§10.6.2.2): performance.now() at run.start arrival.
 * On mid-run reconnect, re-derived as performance.now() − latest at_ms seen.
 */
export const runClock = writable<number | null>(null);

/** Applies one verbatim v1.3 event: run.error → runMeta, rows → reducer. */
export function applyRunEvent(event: RunEvent): void {
  if (event.kind === 'run.start') {
    runClock.set(performance.now() - event.at_ms);
  } else {
    // Mid-run reconnect without a replayed run.start: derive the basis from
    // the first event seen (±network skew, acceptable per spec).
    runClock.update((t0) => t0 ?? performance.now() - event.at_ms);
  }
  if (event.kind === 'run.error') {
    runMeta.update((m) => ({ ...m, error: event.error }));
    return;
  }
  requests.update((map) => applyEvent(map, event));
}

/** Resets run state for a newly started/adopted run. */
export function resetRun(runId: string, params: StartParams | null = null): void {
  runMeta.set({ run_id: runId, error: null, params });
  requests.set(new Map());
  reconciledSummary.set(null);
  lastRunInfo.set(null);
  runClock.set(null);
}

/** Authoritative GET /runs/{id} payload stored at reconcile (§10.3.4). */
export const lastRunInfo = writable<RunInfo | null>(null);

/** 100 ms ticker driving live elapsed displays; only ticks with subscribers. */
export const nowTick = readable(performance.now(), (set) => {
  const t = setInterval(() => set(performance.now()), 100);
  return () => clearInterval(t);
});

/** Authoritative summary from GET /runs/{id} after reconcile (§10.3.4). */
export const reconciledSummary = writable<RunSummary | null>(null);

export interface SummaryView {
  total: number;
  pending: number;
  running: number;
  passed: number;
  failed: number;
  skipped: number;
  error: number;
  completed: number;
  /** False once the reconciled (authoritative) summary replaced live counts. */
  live: boolean;
  duration_ms?: number;
  parallel?: boolean;
  wave_count?: number;
  max_parallelism?: number;
  wave_durations_ms?: number[];
}

/**
 * Summary strip source: counts derive from the request map (replay-safe and
 * idempotent — never a separate counter); the reconciled summary wins once set.
 */
export const summary = derived(
  [requests, reconciledSummary],
  ([$requests, $reconciled]): SummaryView => {
    if ($reconciled !== null) {
      return {
        total: $reconciled.total,
        pending: 0,
        running: 0,
        passed: $reconciled.passed,
        failed: $reconciled.failed,
        skipped: $reconciled.skipped,
        error: $reconciled.error,
        completed: $reconciled.passed + $reconciled.failed + $reconciled.skipped + $reconciled.error,
        live: false,
        duration_ms: $reconciled.duration_ms,
        parallel: $reconciled.parallel,
        wave_count: $reconciled.wave_count,
        max_parallelism: $reconciled.max_parallelism,
        wave_durations_ms: $reconciled.wave_durations_ms,
      };
    }
    const view: SummaryView = {
      total: $requests.size,
      pending: 0,
      running: 0,
      passed: 0,
      failed: 0,
      skipped: 0,
      error: 0,
      completed: 0,
      live: true,
    };
    for (const row of $requests.values()) {
      view[row.status]++;
      if (
        row.status === 'passed' ||
        row.status === 'failed' ||
        row.status === 'skipped' ||
        row.status === 'error'
      ) {
        view.completed++;
      }
    }
    return view;
  },
);

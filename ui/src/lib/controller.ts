// App orchestration glue (UI_SPECIFICATION.md §10.3.2–§10.3.5, §10.4.3):
// boot sequence, WS frame interpretation, and the run-lifecycle side effects
// around the pure run-machine/event-reducer modules.

import { get, writable } from 'svelte/store';
import { ApiError } from './api/client';
import { cancelRun, getCurrentRun, getRun, getRunRequests, startRun as postRun } from './api/runs';
import { navigate } from './router';
import { WsClient } from './ws';
import { environments, loadEnvironments, selectedEnv } from './stores/environments';
import { loadHistory } from './stores/history';
import { loadMeta, meta } from './stores/meta';
import {
  applyRunEvent,
  dispatchRun,
  lastRunInfo,
  reconciledSummary,
  resetRun,
  runMeta,
  runState,
  seedRequests,
} from './stores/run';
import { refreshTree } from './stores/tree';
import { serverReachable, wsStatus } from './stores/connection';
import { lastFilesChanged, watchEvent } from './stores/ui';
import { toast } from './stores/toast';
import type { WsFrame } from './types/events';
import type { StartParams } from './types/run';
import type { TreeIssue } from './types/tree';

/** True once /meta + /tree + /environments resolved and the WS is opening. */
export const booted = writable(false);

let ws: WsClient | null = null;
let watchSeq = 0;
let watchTimer: ReturnType<typeof setTimeout> | null = null;

export function wsClient(): WsClient | null {
  return ws;
}

/** Boot sequence (§10.4.3, steps 3–5). Returns false when the server is gone. */
export async function boot(): Promise<boolean> {
  try {
    const [m] = await Promise.all([loadMeta(), refreshTree(), loadEnvironments()]);
    // selectedEnv defaults to meta.project.default_env when unset; projects
    // without a default fall back to the first listed environment.
    if (get(selectedEnv) === null) {
      const fallback = get(environments)[0]?.name ?? null;
      selectedEnv.set(m.project.default_env !== '' ? m.project.default_env : fallback);
    }
    ws = new WsClient({ onFrame: handleFrame, status: wsStatus });
    ws.connect();
    // History rail data is loaded lazily by the compare screen; pre-warm only
    // the "Re-run failed" affordance via the current run, if any.
    void adoptIfActive();
    booted.set(true);
    return true;
  } catch {
    serverReachable.set(false);
    return false;
  }
}

/** Watch the ws status store: a dead socket means the server is unreachable. */
wsStatus.subscribe((s) => {
  if (s === 'dead') serverReachable.set(false);
});

// ---------------------------------------------------------------------------
// WS frame interpretation (§10.3.5)

function handleFrame(frame: WsFrame): void {
  switch (frame.type) {
    case 'hello': {
      void refreshTree(frame.data.tree_etag);
      const run = frame.data.run;
      if (run !== null) {
        const current = get(runMeta);
        if (current.run_id === run.run_id) {
          // Reconnect to the run we already track: replay from lastEventId.
          ws?.subscribe(run.run_id, ws.getLastEventId());
        } else {
          void adoptRun(run.run_id, run.state);
        }
      }
      break;
    }
    case 'run.event': {
      if (frame.run_id !== get(runMeta).run_id) return;
      applyRunEvent(frame.data);
      if (frame.data.kind === 'run.end') {
        void reconcile(frame.run_id);
      }
      break;
    }
    case 'run.state': {
      if (frame.run_id !== get(runMeta).run_id) return;
      const state = frame.data.state;
      dispatchRun({ type: 'run_state', state });
      if (state === 'completed' || state === 'cancelled' || state === 'error') {
        void reconcile(frame.run_id);
      }
      break;
    }
    case 'files.changed': {
      void refreshTree(frame.data.tree_etag);
      lastFilesChanged.update((p) => ({ paths: frame.data.paths, seq: p.seq + 1 }));
      pulseWatch(frame.data.paths);
      break;
    }
    case 'error':
    case 'pong':
      break;
  }
}

function pulseWatch(paths: string[]): void {
  if (paths.length === 0) return;
  const base = paths[0].split('/').pop() ?? paths[0];
  const extra = paths.length > 1 ? ` +${paths.length - 1}` : '';
  watchSeq++;
  watchEvent.set({ label: `${base}${extra} changed · tree reloaded`, seq: watchSeq });
  if (watchTimer !== null) clearTimeout(watchTimer);
  watchTimer = setTimeout(() => watchEvent.set(null), 2600);
}

// ---------------------------------------------------------------------------
// Run lifecycle (§10.3.2 side effects)

let reconciledFor: string | null = null;

/** POST /runs with the full §10.3.2 outcome handling. */
export async function startRun(params: StartParams): Promise<void> {
  const s = get(runState);
  if (s === 'starting' || s === 'running' || s === 'cancelling') return;
  dispatchRun({ type: 'run_click' }); // optimistic: button → "Starting…"
  try {
    const res = await postRun(params);
    reconciledFor = null;
    resetRun(res.run_id, params);
    dispatchRun({ type: 'start_ok' });
    navigate({ name: 'run' });
    const list = await getRunRequests(res.run_id);
    seedRequests(list.requests);
    ws?.subscribe(res.run_id, 0);
  } catch (err) {
    if (err instanceof ApiError) {
      handleStartError(err);
      return;
    }
    dispatchRun({ type: 'start_network_error' });
    toast('could not reach the curlew ui server', { kind: 'error' });
    serverReachable.set(false);
  }
}

function handleStartError(err: ApiError): void {
  switch (err.code) {
    case 'run_active':
      dispatchRun({ type: 'start_conflict' });
      toast('A run is already in progress', {
        action: { label: 'View', fn: () => void adoptIfActive(true) },
      });
      break;
    case 'collection_invalid': {
      dispatchRun({ type: 'start_invalid' });
      const d = (err.details ?? {}) as { file?: string; issues?: TreeIssue[] };
      if (d.file !== undefined) {
        navigate({ name: 'file', path: d.file });
      } else {
        toast(err.message, { kind: 'error' });
      }
      break;
    }
    case 'env_not_found':
      dispatchRun({ type: 'start_invalid' });
      toast(err.message, { kind: 'error' });
      break;
    case 'network_error':
      dispatchRun({ type: 'start_network_error' });
      toast('could not reach the curlew ui server', { kind: 'error' });
      serverReachable.set(false);
      break;
    default:
      dispatchRun({ type: 'start_invalid' });
      toast(err.message, { kind: 'error' });
  }
}

/** Adopts the active run from GET /runs/current (hello race, 409 View). */
export async function adoptIfActive(navigateHome = false): Promise<void> {
  try {
    const res = await getCurrentRun();
    if (res.run === null) return;
    await adoptRun(res.run.run_id, res.run.state, res.run.params);
    if (navigateHome) navigate({ name: 'run' });
  } catch {
    // server unreachable — the WS/probe flow handles it
  }
}

async function adoptRun(
  runId: string,
  state: 'running' | 'cancelling',
  params: StartParams | null = null,
): Promise<void> {
  reconciledFor = null;
  resetRun(runId, params);
  dispatchRun({ type: 'adopt', state });
  try {
    const list = await getRunRequests(runId);
    seedRequests(list.requests);
  } catch {
    // seeding failed — replay from 0 still rebuilds rows from events
  }
  ws?.subscribe(runId, 0);
}

/** Cancel the active run (Run button / `c` hold). */
export async function cancelActiveRun(): Promise<void> {
  const id = get(runMeta).run_id;
  if (id === null || get(runState) !== 'running') return;
  dispatchRun({ type: 'cancel_click' });
  try {
    await cancelRun(id);
  } catch {
    // 404 = run already finished; the terminal run.state frame wins
  }
}

/** Reconciliation after run end (§10.3.4) — idempotent per run id. */
export async function reconcile(runId: string): Promise<void> {
  if (reconciledFor === runId) return;
  reconciledFor = runId;
  try {
    const [info, list] = await Promise.all([getRun(runId), getRunRequests(runId)]);
    if (get(runMeta).run_id !== runId) return; // a new run took focus meanwhile
    lastRunInfo.set(info);
    reconciledSummary.set(info.summary);
    seedRequests(list.requests);
  } catch {
    reconciledFor = null; // allow a retry on the next terminal frame
  }
  const m = get(meta);
  if (m !== null && m.history.enabled) {
    void loadHistory().catch(() => undefined);
  }
}

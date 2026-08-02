// Pure event-application reducer over verbatim v1.3 events
// (UI_SPECIFICATION.md §10.3.3, docs/EVENTS_SCHEMA_v1.3.md).
//
// Binding rules encoded here:
//   - Structure (wave/phase/grouping) is NEVER derived from events; the map is
//     seeded from the REST light list (seedFromList), where planned rows carry
//     synthetic ids "slug:<slug>" until the runner mints real ones.
//   - request.start re-keys the matching "slug:<request_slug>" planned row to
//     the real request_id (mirrors the server's adoption behavior), preserving
//     display order; truly unknown ids insert fresh with wave_index -1.
//   - request.end NEVER reads the frame's wave_index (omitempty quirk: absent
//     when 0), timing, or the 2 KiB-truncated bodies — REST is authoritative.
//   - Idempotent under replay; tolerates request.end on already-terminal rows
//     and out-of-order assertion.result after request.end.

import type {
  AssertionResultEvent,
  RequestEndEvent,
  RequestStartEvent,
  RunEvent,
} from './types/events';
import type { CompactError, Iteration, Phase, RequestListEntry } from './types/run';

export type LiveStatus = 'pending' | 'running' | 'passed' | 'failed' | 'skipped' | 'error';

export interface LiveRequest {
  request_id: string;
  slug: string;
  name: string;
  method: string;
  phase: Phase;
  status: LiveStatus;
  status_code?: number;
  duration_ms?: number;
  /** Live: derived from assertion.result; authoritative from REST at reconcile. */
  fail_message?: string;
  /** Error outcomes; compact form ({category, code?, message}). */
  error?: CompactError;
  /** From the REST light list only (events don't carry it). */
  skip_reason?: string;
  source_file: string;
  source_line: number;
  /** ALWAYS from REST, never events (omitempty quirk); -1 until reconciled. */
  wave_index: number;
  retry_count: number;
  iteration?: Iteration;
  /** request.start at_ms, for per-row live elapsed. */
  at_ms?: number;
}

export type RequestMap = Map<string, LiveRequest>;

const SYNTHETIC_PREFIX = 'slug:';

/** The planned-entry id used by the server until a real request id exists. */
export function syntheticId(slug: string): string {
  return SYNTHETIC_PREFIX + slug;
}

/** Seeds the map from the REST light list, keyed by request_id (§10.3.3). */
export function seedFromList(entries: RequestListEntry[]): RequestMap {
  const map: RequestMap = new Map();
  for (const e of entries) {
    const row: LiveRequest = {
      request_id: e.request_id,
      slug: e.slug,
      name: e.name,
      method: e.method,
      phase: e.phase,
      status: e.outcome ?? 'pending',
      source_file: e.source_file,
      source_line: e.source_line,
      wave_index: e.wave_index,
      retry_count: e.retry_count,
    };
    if (e.outcome !== null) row.duration_ms = e.duration_ms;
    if (e.status_code !== undefined) row.status_code = e.status_code;
    if (e.fail_message !== null) row.fail_message = e.fail_message;
    if (e.skip_reason !== null) row.skip_reason = e.skip_reason;
    if (e.error !== null) row.error = e.error;
    if (e.iteration !== null) row.iteration = e.iteration;
    map.set(e.request_id, row);
  }
  return map;
}

/**
 * Pure reducer: returns a new map when the event changes a row, the same map
 * otherwise. run.start/run.end/run.error never mutate rows — resets and
 * reconciliation are store-layer concerns (§10.3.3–§10.3.4), and run.error
 * lands on runMeta, not the request map.
 */
export function applyEvent(map: RequestMap, event: RunEvent): RequestMap {
  switch (event.kind) {
    case 'request.start':
      return applyRequestStart(map, event);
    case 'request.end':
      return applyRequestEnd(map, event);
    case 'assertion.result':
      return applyAssertionResult(map, event);
    case 'run.start':
    case 'run.end':
    case 'run.error':
      return map;
  }
}

/** Rebuilds the map with `fromKey` replaced in place by (toKey, row). */
function rekey(map: RequestMap, fromKey: string, toKey: string, row: LiveRequest): RequestMap {
  const next: RequestMap = new Map();
  for (const [k, v] of map) {
    if (k === fromKey) {
      next.set(toKey, row);
    } else {
      next.set(k, v);
    }
  }
  return next;
}

function applyRequestStart(map: RequestMap, e: RequestStartEvent): RequestMap {
  const existing = map.get(e.request_id);
  if (existing !== undefined) {
    const next = new Map(map);
    next.set(e.request_id, { ...existing, status: 'running', at_ms: e.at_ms });
    return next;
  }
  // Unknown real id: adopt the pending planned row keyed "slug:<request_slug>"
  // and re-key it to the real id, preserving display order.
  if (e.request_slug !== undefined) {
    const synKey = syntheticId(e.request_slug);
    const planned = map.get(synKey);
    if (planned !== undefined) {
      return rekey(map, synKey, e.request_id, {
        ...planned,
        request_id: e.request_id,
        status: 'running',
        at_ms: e.at_ms,
      });
    }
  }
  // Truly unplanned (e.g. a data-driven expansion the planner didn't predict):
  // insert fresh from the event's structural fields, wave_index -1 until reconcile.
  const next = new Map(map);
  next.set(e.request_id, {
    request_id: e.request_id,
    slug: e.request_slug ?? '',
    name: e.name ?? e.request_slug ?? e.request_id,
    method: e.method,
    phase: e.phase === 'setup' || e.phase === 'teardown' ? e.phase : 'main',
    status: 'running',
    source_file: e.source_file ?? '',
    source_line: e.source_line ?? 0,
    wave_index: -1,
    retry_count: 0,
    at_ms: e.at_ms,
  });
  return next;
}

function applyRequestEnd(map: RequestMap, e: RequestEndEvent): RequestMap {
  let row = map.get(e.request_id);
  let rekeyFrom: string | undefined;
  if (row === undefined && e.request_slug !== undefined) {
    // request.end without a seen request.start (replay edge): adopt the
    // planned row the same way the server does.
    const synKey = syntheticId(e.request_slug);
    const planned = map.get(synKey);
    if (planned !== undefined) {
      row = planned;
      rekeyFrom = synKey;
    }
  }

  const updated: LiveRequest = row
    ? { ...row, request_id: e.request_id }
    : {
        request_id: e.request_id,
        slug: e.request_slug ?? '',
        name: e.request_slug ?? e.request_id,
        method: '',
        phase: 'main',
        status: 'pending',
        source_file: '',
        source_line: 0,
        wave_index: -1,
        retry_count: 0,
      };

  // Terminal rows are overwritten, never double counted (replay tolerance).
  updated.status = e.outcome;
  updated.duration_ms = e.duration_ms;
  if (e.status_code !== undefined) updated.status_code = e.status_code;
  if (e.error !== undefined) {
    const compact: CompactError = { category: e.error.category, message: e.error.message };
    if (e.error.code !== undefined) compact.code = e.error.code;
    updated.error = compact;
  }
  // Deliberately ignored: e.wave_index (omitempty trap), e.timing, and the
  // 2 KiB-truncated request/response bodies — REST is authoritative for those.

  if (rekeyFrom !== undefined) {
    return rekey(map, rekeyFrom, e.request_id, updated);
  }
  const next = new Map(map);
  next.set(e.request_id, updated);
  return next;
}

function applyAssertionResult(map: RequestMap, e: AssertionResultEvent): RequestMap {
  if (e.passed) return map;
  const row = map.get(e.request_id);
  // Tolerates unknown rows and out-of-order delivery after request.end;
  // first failing assertion wins (the full list comes from REST detail).
  if (row === undefined || row.fail_message !== undefined) return map;
  const next = new Map(map);
  next.set(e.request_id, {
    ...row,
    fail_message: `${e.type}: expected ${e.expected ?? ''}, got ${e.actual ?? ''}`,
  });
  return next;
}

// 1:1 mirrors of the events schema v1.3 (docs/EVENTS_SCHEMA_v1.3.md) and the
// WS frame envelopes (UI_SPECIFICATION.md §5.1). Discriminated unions on the
// event `kind` and the frame `type`. snake_case kept.

/** Common header carried by every v1.3 event. */
export interface EventHeader {
  schema_version: string;
  run_id: string;
  /** Strictly monotonic event counter — the replay cursor (`from_id`). */
  id: number;
  /** Milliseconds since run start. */
  at_ms: number;
}

/** Error payload (run.error.error / request.end.error). */
export interface EventError {
  category: string;
  message: string;
  code?: string;
  hint?: string;
  file?: string;
  line?: number;
}

export interface RunStartEvent extends EventHeader {
  kind: 'run.start';
  started_at: string;
  curlew_version: string;
  cli_args: string[];
  collection_file?: string;
  env_name?: string;
  selection?: string[];
}

export interface RunErrorEvent extends EventHeader {
  kind: 'run.error';
  error: EventError;
}

export interface RequestStartEvent extends EventHeader {
  kind: 'request.start';
  request_id: string;
  method: string;
  url: string;
  request_slug?: string;
  name?: string;
  phase?: string;
  source_file?: string;
  source_line?: number;
}

/** v1.3 timing object on request.end — integer microseconds. */
export interface EventTiming {
  dns_us?: number;
  connect_us?: number;
  tls_us?: number;
  ttfb_us?: number;
  download_us?: number;
  total_us: number;
  connection_reused?: boolean;
  attempts?: number;
}

export interface RequestEndEvent extends EventHeader {
  kind: 'request.end';
  request_id: string;
  outcome: 'passed' | 'failed' | 'skipped' | 'error';
  duration_ms: number;
  request_slug?: string;
  status_code?: number;
  /**
   * QUIRK (omitempty): absent when 0 AND absent for sequential runs — the
   * client must never read this; wave_index comes from the REST light list.
   */
  wave_index?: number;
  request_body?: string;
  request_body_size?: number;
  request_body_truncated?: boolean;
  request_body_encoding?: string;
  response_body?: string;
  response_body_size?: number;
  response_body_truncated?: boolean;
  response_body_encoding?: string;
  error?: EventError;
  timing?: EventTiming;
}

export interface AssertionResultEvent extends EventHeader {
  kind: 'assertion.result';
  request_id: string;
  type: 'status' | 'body' | 'header' | 'schema';
  passed: boolean;
  expected?: string;
  actual?: string;
}

export interface RunEndEvent extends EventHeader {
  kind: 'run.end';
  duration_ms: number;
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  exit_code: number;
  event_count: number;
}

export type RunEvent =
  | RunStartEvent
  | RunErrorEvent
  | RequestStartEvent
  | RequestEndEvent
  | AssertionResultEvent
  | RunEndEvent;

// ---------------------------------------------------------------------------
// WS frames (§5.1)

export interface HelloRun {
  run_id: string;
  state: 'running' | 'cancelling';
  last_event_id: number;
}

export interface HelloData {
  proto: number;
  server_version: string;
  run: HelloRun | null;
  tree_etag: string;
}

export interface RunStateData {
  state: 'running' | 'cancelling' | 'completed' | 'cancelled' | 'error';
  exit_status?: string;
}

export interface FilesChangedData {
  paths: string[];
  tree_etag: string;
}

export interface WsErrorData {
  code: string;
  message: string;
}

export type WsFrame =
  | { type: 'hello'; data: HelloData }
  | { type: 'run.event'; run_id: string; data: RunEvent }
  | { type: 'run.state'; run_id: string; data: RunStateData }
  | { type: 'files.changed'; data: FilesChangedData }
  | { type: 'error'; data: WsErrorData }
  | { type: 'pong'; data: Record<string, never> };

export type ClientFrame =
  | { type: 'subscribe'; run_id: string; from_id: number }
  | { type: 'ping' };

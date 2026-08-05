// 1:1 mirrors of the run endpoints (UI_SPECIFICATION.md §4.7–§4.11, §8.2).
// snake_case kept, matching internal/uiserver JSON struct tags exactly.

export type Phase = 'setup' | 'main' | 'teardown';
export type Outcome = 'passed' | 'failed' | 'skipped' | 'error';
export type RunState = 'running' | 'cancelling' | 'completed' | 'cancelled' | 'error';
export type ExitStatus = 'passed' | 'failed' | 'error' | 'cancelled';

/** Compact error object on light list rows and compare sides (no hint). */
export interface CompactError {
  category: string;
  code?: string;
  message: string;
}

/** Full error object on the detail payload (§4.9). */
export interface DetailError extends CompactError {
  hint?: string;
}

export interface Iteration {
  index: number;
  total: number;
  base_name: string;
  base_slug: string;
}

/**
 * One row of GET /runs/{id}/requests — the light list (§4.8).
 * Planned (pending) rows have synthetic ids of the form "slug:<slug>" and
 * outcome null; the server re-keys them to real ids (e.g. "req-3") when the
 * runner starts the request.
 */
export interface RequestListEntry {
  request_id: string;
  slug: string;
  name: string;
  phase: Phase;
  method: string;
  outcome: Outcome | null;
  status_code?: number;
  duration_ms: number;
  /** Authoritative; -1 for sequential runs. ALWAYS from REST, never events. */
  wave_index: number;
  retry_count: number;
  skip_reason: string | null;
  fail_message: string | null;
  error: CompactError | null;
  iteration: Iteration | null;
  source_file: string;
  source_line: number;
}

export interface RequestListResponse {
  requests: RequestListEntry[];
}

// §4.7 POST /api/v1/runs
export interface StartParams {
  /** Root-relative collection path, or null = batch run (every valid collection). */
  collection: string | null;
  env: string;
  parallel: boolean;
  mode: 'all' | 'selection' | 'rerun_failed';
  selection: string[] | null;
  rerun_of: string | null;
}

export interface StartRunResponse {
  run_id: string;
  state: string;
}

// §4.8 GET /api/v1/runs/current
export interface RunProgress {
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  error: number;
  completed: number;
}

export interface CurrentRun {
  run_id: string;
  state: 'running' | 'cancelling';
  params: StartParams;
  started_at: string;
  last_event_id: number;
  progress: RunProgress;
}

export interface CurrentRunResponse {
  run: CurrentRun | null;
}

// §4.8 run summary block (maps 1:1 from runner.Summary)
export interface RunSummary {
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  error: number;
  duration_ms: number;
  parallel: boolean;
  /** Omitted for batch runs (waves are per-collection) and sequential runs. */
  wave_count?: number;
  max_parallelism?: number;
  wave_durations_ms?: number[];
}

// §8.2 meta.json shape (store schema v1) — also embedded in GET /runs/{id}.
export interface GitInfo {
  branch: string | null;
  commit: string | null;
}

export interface RunMeta {
  schema_version: number;
  run_id: string;
  created_at: string;
  curlew_version: string;
  events_schema_version: string;
  collection_file: string | null;
  collection_name: string | null;
  env_name: string;
  selection: string[] | null;
  parallel: boolean;
  exit_status: string;
  git: GitInfo | null;
  summary: RunSummary;
}

// GET /api/v1/runs/{run_id}
export interface RunInfo {
  run_id: string;
  state: RunState;
  /** Where the data came from (informational). */
  source: 'active' | 'memory' | 'store';
  exit_status?: string;
  meta?: RunMeta;
  summary: RunSummary;
}

// §4.11 GET /api/v1/runs (Solo history list)
export interface RunListResponse {
  runs: RunMeta[];
  total: number;
}

// §4.9 full detail (the inspector)
export interface Timing {
  dns_us?: number;
  connect_us?: number;
  tls_us?: number;
  ttfb_us?: number;
  download_us?: number;
  total_us: number;
  connection_reused: boolean;
  attempts: number;
}

export interface RetryAttempt {
  number: number;
  status_code: number;
  duration_ms: number;
  delay_ms: number;
  error: string | null;
}

export interface Retry {
  count: number;
  warnings: string[];
  attempts: RetryAttempt[];
}

export interface BodyPayload {
  /** Omitted when over the 256 KiB inline limit — fetch via /body instead. */
  content?: string;
  /** "base64" for binary bodies. */
  encoding?: string;
  size: number;
  truncated: boolean;
  content_type?: string;
}

export interface SourceRef {
  file: string;
  line: number;
  snippet: string[];
  snippet_start_line: number;
}

export interface AssertionItem {
  /** Discriminator only. Rendered as the type chip. */
  type: string;
  /** JSONPath, header name, schema path, or assertions[N]. */
  target?: string;
  /** Comparison applied; absent where the type implies it. */
  operator?: string;
  /** Assembled human-readable phrase, e.g. "body $.user.name equals". */
  label: string;
  expected: string;
  actual: string;
  passed: boolean;
}

export interface Assertions {
  passed: boolean;
  items: AssertionItem[];
}

export interface RequestPayload {
  headers: Record<string, string>;
  body: BodyPayload | null;
}

export interface ResponsePayload {
  /** Go http.Header — multi-valued. */
  headers: Record<string, string[]>;
  body: BodyPayload | null;
}

export interface RequestDetail {
  request_id: string;
  slug: string;
  name: string;
  phase: Phase;
  method: string;
  /** Resolved, redacted as-sent URL (raw template lives in the tree). */
  url: string;
  outcome: Outcome | null;
  status_code?: number;
  duration_ms: number;
  wave_index: number;
  timing: Timing | null;
  retry: Retry | null;
  skipped: boolean;
  skip_reason: string | null;
  warnings: string[] | null;
  iteration: Iteration | null;
  source: SourceRef | null;
  request: RequestPayload | null;
  response: ResponsePayload | null;
  assertions: Assertions | null;
  error: DetailError | null;
}

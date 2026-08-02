// 1:1 mirrors of GET /api/v1/compare (UI_SPECIFICATION.md §4.12). snake_case kept.

import type { CompactError, RunInfo, Timing } from './run';

export interface CompareSide {
  request_id: string;
  outcome: string | null;
  status_code?: number;
  duration_ms: number;
  timing: Timing | null;
  fail_message: string | null;
  error: CompactError | null;
}

export interface CompareDelta {
  duration_ms: number;
  outcome_changed: boolean;
  status_changed: boolean;
  /** Keys: dns, connect, tls, ttfb, download, total — signed µs deltas. */
  timing_us?: Record<string, number>;
}

export interface ComparePair {
  slug: string;
  /** Data-driven iteration index; null for non-iterated requests. */
  iteration: number | null;
  name: string;
  method: string;
  base: CompareSide | null;
  target: CompareSide | null;
  delta: CompareDelta | null;
}

export interface CompareOnlyEntry {
  slug: string;
  name: string;
  method: string;
  request_id: string;
}

export interface CompareResult {
  base: RunInfo;
  target: RunInfo;
  pairs: ComparePair[];
  only_in_base: CompareOnlyEntry[];
  only_in_target: CompareOnlyEntry[];
}

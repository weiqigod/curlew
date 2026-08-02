/** Allowed window values for the dashboard aggregation queries. */
export type DashboardWindow = '7d' | '30d' | '90d';

/** Aggregated totals across all runs within the requested window. */
export interface StatsTotals {
	runs: number;
	pass_count: number;
	fail_count: number;
	skipped_count: number;
	/** Fractional pass rate in [0, 1]. Multiply by 100 for percentage display. */
	pass_rate: number;
	avg_duration_ms: number;
	p50_duration_ms: number;
	p95_duration_ms: number;
}

/** One day's aggregated pass-rate entry in the trend array. */
export interface TrendEntry {
	/** YYYY-MM-DD date. */
	date: string;
	runs: number;
	pass_count: number;
	fail_count: number;
	/** Fractional pass rate in [0, 1]. */
	pass_rate: number;
	avg_duration_ms: number;
}

/** Response shape from GET /results/stats. */
export interface StatsResponse {
	window: string;
	/** ISO 8601 UTC start of the requested window. */
	window_start: string;
	/** ISO 8601 UTC end of the requested window. */
	window_end: string;
	totals: StatsTotals;
	/** Sparse daily trend — only days with at least one run are included. */
	trend: TrendEntry[];
}

/** One grouped failure entry from GET /results/failures. */
export interface FailureGroup {
	method: string;
	path_template: string;
	failure_count: number;
	/** ISO 8601 UTC. */
	first_seen_at: string;
	/** ISO 8601 UTC. */
	last_seen_at: string;
	sample_run_ids: string[];
}

/** Response shape from GET /results/failures. */
export interface FailuresResponse {
	window: string;
	limit: number;
	/** True when the requested limit was clamped to the backend maximum. */
	limit_clamped: boolean;
	items: FailureGroup[];
}

/** A single test-run result header returned by the backend. */
export interface Result {
	id: string;
	collection_name: string;
	pass_count: number;
	fail_count: number;
	skipped_count: number;
	duration_ms: number;
	run_at: string; // ISO 8601 UTC
	created_at: string; // ISO 8601 UTC
	triggered_by: string | null;
	git_sha: string | null;
}

/** Aggregated statistics derived from a list of Result records. */
export interface ResultSummary {
	total_runs: number;
	total_pass: number;
	total_fail: number;
	pass_rate: number; // 0–100 with 2 decimal places
	average_duration_ms: number;
}

/** Time-range filter for the results dashboard. */
export type TimeRange = '24h' | '7d' | '30d' | 'all';

/** A single day-bucket data point for the trend chart. */
export interface TrendPoint {
	bucket_start: string; // YYYY-MM-DD
	pass: number;
	fail: number;
}

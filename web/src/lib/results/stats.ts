import type { Result, ResultSummary, TimeRange, TrendPoint } from '$lib/types/results';

/**
 * Filter results to those falling within the given time range.
 * The boundary instant is inclusive.
 */
export function filterByRange(
	results: Result[],
	range: TimeRange,
	now: Date = new Date()
): Result[] {
	if (range === 'all') return results;
	const hours = range === '24h' ? 24 : range === '7d' ? 24 * 7 : 24 * 30;
	const cutoff = now.getTime() - hours * 60 * 60 * 1000;
	return results.filter((r) => Date.parse(r.run_at) >= cutoff);
}

/**
 * Derive aggregate statistics from a list of results.
 * Safe to call with an empty list.
 */
export function summarize(results: Result[]): ResultSummary {
	const total_runs = results.length;
	if (total_runs === 0) {
		return { total_runs: 0, total_pass: 0, total_fail: 0, pass_rate: 0, average_duration_ms: 0 };
	}
	const total_pass = results.reduce((n, r) => n + r.pass_count, 0);
	const total_fail = results.reduce((n, r) => n + r.fail_count, 0);
	const tests = total_pass + total_fail;
	// Guard against zero-test runs to avoid division by zero
	const pass_rate = tests === 0 ? 0 : Math.round((total_pass / tests) * 10_000) / 100;
	const average_duration_ms = Math.round(
		results.reduce((n, r) => n + r.duration_ms, 0) / total_runs
	);
	return { total_runs, total_pass, total_fail, pass_rate, average_duration_ms };
}

/**
 * Group results into day-buckets suitable for the trend chart.
 * Returns buckets sorted ascending by date.
 */
export function bucketByDay(results: Result[]): TrendPoint[] {
	const buckets = new Map<string, TrendPoint>();
	for (const r of results) {
		const day = r.run_at.slice(0, 10); // YYYY-MM-DD
		const point = buckets.get(day) ?? { bucket_start: day, pass: 0, fail: 0 };
		point.pass += r.pass_count;
		point.fail += r.fail_count;
		buckets.set(day, point);
	}
	return [...buckets.values()].sort((a, b) => a.bucket_start.localeCompare(b.bucket_start));
}

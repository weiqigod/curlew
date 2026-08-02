/**
 * Formats an ISO 8601 instant as a coarse relative time.
 * Examples: "5m ago", "3h ago", "2d ago".
 * Returns "just now" for future timestamps, or the original string for invalid ISO values.
 */
export function formatRelativeTime(iso: string, now: Date = new Date()): string {
	const then = Date.parse(iso);
	if (Number.isNaN(then)) return iso;
	const diffSec = Math.floor((now.getTime() - then) / 1000);
	if (diffSec < 0) return 'just now';
	if (diffSec < 60) return `${diffSec}s ago`;
	if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
	if (diffSec < 86_400) return `${Math.floor(diffSec / 3600)}h ago`;
	return `${Math.floor(diffSec / 86_400)}d ago`;
}

/**
 * Renders a 0–1 fractional pass rate as a percentage string.
 * Example: 0.9883 → "98.83%".
 */
export function formatPassRate(rate: number): string {
	return `${(rate * 100).toFixed(2)}%`;
}

/**
 * Renders a millisecond count in compact form.
 * Values under 1000ms render as "300ms"; values 1000ms and above render as "1.2s".
 */
export function formatDuration(ms: number): string {
	if (ms < 1000) return `${ms}ms`;
	return `${(ms / 1000).toFixed(1)}s`;
}

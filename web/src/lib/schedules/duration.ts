/**
 * Computes a human-readable duration string from two ISO-8601 timestamps.
 * Returns an em dash when either timestamp is absent.
 */
export function computeDuration(
	startedAt: string | null,
	completedAt: string | null
): string {
	if (!startedAt || !completedAt) return '—';
	const ms = new Date(completedAt).getTime() - new Date(startedAt).getTime();
	if (ms < 0) return '—';
	if (ms < 1000) return `${ms}ms`;
	if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
	return `${Math.floor(ms / 60_000)}m${Math.floor((ms % 60_000) / 1000)}s`;
}

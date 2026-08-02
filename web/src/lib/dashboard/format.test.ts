import { describe, it, expect } from 'vitest';
import { formatRelativeTime, formatPassRate, formatDuration } from './format';

describe('formatRelativeTime', () => {
	const now = new Date('2026-05-12T12:00:00Z');

	const tests: Array<{ name: string; iso: string; want: string }> = [
		{ name: 'just now (zero delta)', iso: '2026-05-12T12:00:00Z', want: '0s ago' },
		{ name: 'seconds ago', iso: '2026-05-12T11:59:30Z', want: '30s ago' },
		{ name: 'minutes ago', iso: '2026-05-12T11:55:00Z', want: '5m ago' },
		{ name: 'hours ago', iso: '2026-05-12T09:00:00Z', want: '3h ago' },
		{ name: 'days ago', iso: '2026-05-10T12:00:00Z', want: '2d ago' },
		{ name: 'future timestamp → just now', iso: '2026-05-12T12:00:30Z', want: 'just now' },
		{ name: 'invalid input → echo back', iso: 'not-an-iso', want: 'not-an-iso' }
	];

	for (const t of tests) {
		it(t.name, () => expect(formatRelativeTime(t.iso, now)).toBe(t.want));
	}
});

describe('formatPassRate', () => {
	it.each([
		{ rate: 1, want: '100.00%' },
		{ rate: 0.9883, want: '98.83%' },
		{ rate: 0, want: '0.00%' }
	])('rate $rate → $want', ({ rate, want }) => expect(formatPassRate(rate)).toBe(want));
});

describe('formatDuration', () => {
	it.each([
		{ ms: 100, want: '100ms' },
		{ ms: 999, want: '999ms' },
		{ ms: 1000, want: '1.0s' },
		{ ms: 2347, want: '2.3s' }
	])('$ms ms → $want', ({ ms, want }) => expect(formatDuration(ms)).toBe(want));
});

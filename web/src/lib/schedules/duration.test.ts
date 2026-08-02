import { describe, it, expect } from 'vitest';
import { computeDuration } from './duration';

describe('computeDuration', () => {
	it('returns em dash when either timestamp missing', () => {
		expect(computeDuration(null, null)).toBe('—');
		expect(computeDuration('2026-05-11T00:00:00Z', null)).toBe('—');
		expect(computeDuration(null, '2026-05-11T00:00:00Z')).toBe('—');
	});

	it('formats sub-second as ms', () => {
		const started = '2026-05-11T00:00:00.000Z';
		const completed = '2026-05-11T00:00:00.500Z';
		expect(computeDuration(started, completed)).toBe('500ms');
	});

	it('formats sub-minute as seconds', () => {
		const started = '2026-05-11T00:00:00Z';
		const completed = '2026-05-11T00:00:05Z';
		expect(computeDuration(started, completed)).toBe('5.0s');
	});

	it('formats minutes as Xm Ys', () => {
		const started = '2026-05-11T00:00:00Z';
		const completed = '2026-05-11T00:02:30Z';
		expect(computeDuration(started, completed)).toBe('2m30s');
	});

	it('returns em dash for negative duration (clock skew)', () => {
		const started = '2026-05-11T00:00:05Z';
		const completed = '2026-05-11T00:00:00Z';
		expect(computeDuration(started, completed)).toBe('—');
	});
});

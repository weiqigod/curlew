import { describe, it, expect } from 'vitest';
import { summarize, filterByRange, bucketByDay } from './stats';
import type { Result } from '$lib/types/results';

function mk(opts: { pass?: number; fail?: number; dur?: number; runAt?: string }): Result {
	return {
		id: Math.random().toString(36).slice(2),
		collection_name: 'test',
		pass_count: opts.pass ?? 1,
		fail_count: opts.fail ?? 0,
		skipped_count: 0,
		duration_ms: opts.dur ?? 500,
		run_at: opts.runAt ?? '2026-04-15T12:00:00Z',
		created_at: '2026-04-15T12:00:00Z',
		triggered_by: 'cli',
		git_sha: null
	};
}

describe('summarize', () => {
	const cases = [
		{
			name: 'empty list',
			input: [],
			expected: { total_runs: 0, total_pass: 0, total_fail: 0, pass_rate: 0, average_duration_ms: 0 }
		},
		{
			name: 'single all-pass run',
			input: [mk({ pass: 3, fail: 0, dur: 1000 })],
			expected: { total_runs: 1, pass_rate: 100, average_duration_ms: 1000 }
		},
		{
			name: 'mixed passes and failures',
			input: [mk({ pass: 7, fail: 3 }), mk({ pass: 8, fail: 2 })],
			expected: { pass_rate: 75 }
		},
		{
			name: 'zero tests does not divide by zero',
			input: [mk({ pass: 0, fail: 0 })],
			expected: { pass_rate: 0 }
		},
		{
			name: 'average duration rounds to int',
			input: [mk({ dur: 100 }), mk({ dur: 201 })],
			expected: { average_duration_ms: 151 }
		}
	];

	for (const c of cases) {
		it(c.name, () => {
			const result = summarize(c.input);
			expect(result).toMatchObject(c.expected);
		});
	}
});

describe('filterByRange', () => {
	const now = new Date('2026-04-15T12:00:00Z');

	const cases = [
		{
			name: 'range=all returns all',
			results: [
				mk({ runAt: '2026-01-01T00:00:00Z' }),
				mk({ runAt: '2026-04-14T00:00:00Z' }),
				mk({ runAt: '2026-04-15T11:00:00Z' })
			],
			range: 'all' as const,
			expectedCount: 3
		},
		{
			name: 'range=7d drops older than 7 days',
			results: [
				mk({ runAt: '2026-04-07T00:00:00Z' }), // 8 days ago — excluded
				mk({ runAt: '2026-04-09T12:00:00Z' }), // 5.5 days ago — included
				mk({ runAt: '2026-04-15T11:00:00Z' }) // today — included
			],
			range: '7d' as const,
			expectedCount: 2
		},
		{
			name: 'range=24h keeps only last 24h',
			results: [
				mk({ runAt: '2026-04-13T00:00:00Z' }), // old — excluded
				mk({ runAt: '2026-04-14T13:00:00Z' }), // 23h ago — included
				mk({ runAt: '2026-04-15T11:00:00Z' }) // 1h ago — included
			],
			range: '24h' as const,
			expectedCount: 2
		},
		{
			name: 'range=30d keeps 29-day-old entry',
			results: [
				mk({ runAt: '2026-03-17T12:00:00Z' }), // 29 days ago — included
				mk({ runAt: '2026-03-14T00:00:00Z' }) // 32 days ago — excluded
			],
			range: '30d' as const,
			expectedCount: 1
		},
		{
			name: 'equal boundary is inclusive',
			results: [
				mk({ runAt: '2026-04-08T12:00:00Z' }) // exactly 7 days ago
			],
			range: '7d' as const,
			expectedCount: 1
		}
	];

	for (const c of cases) {
		it(c.name, () => {
			const result = filterByRange(c.results, c.range, now);
			expect(result).toHaveLength(c.expectedCount);
		});
	}
});

describe('bucketByDay', () => {
	const cases = [
		{
			name: 'single day',
			input: [mk({ runAt: '2026-04-15T10:00:00Z' }), mk({ runAt: '2026-04-15T14:00:00Z' })],
			expectedBuckets: 1
		},
		{
			name: 'multiple days sorted ascending',
			input: [
				mk({ runAt: '2026-04-14T00:00:00Z' }),
				mk({ runAt: '2026-04-15T00:00:00Z' }),
				mk({ runAt: '2026-04-13T00:00:00Z' })
			],
			expectedOrder: ['2026-04-13', '2026-04-14', '2026-04-15']
		},
		{
			name: 'same day passes and fails merge',
			input: [
				mk({ pass: 5, fail: 1, runAt: '2026-04-15T10:00:00Z' }),
				mk({ pass: 3, fail: 2, runAt: '2026-04-15T14:00:00Z' })
			],
			expectedBuckets: 1,
			expectedMerged: { pass: 8, fail: 3 }
		}
	];

	for (const c of cases) {
		it(c.name, () => {
			const result = bucketByDay(c.input);
			if (c.expectedBuckets !== undefined) {
				expect(result).toHaveLength(c.expectedBuckets);
			}
			if (c.expectedOrder) {
				expect(result.map((r) => r.bucket_start)).toEqual(c.expectedOrder);
			}
			if (c.expectedMerged) {
				expect(result[0].pass).toBe(c.expectedMerged.pass);
				expect(result[0].fail).toBe(c.expectedMerged.fail);
			}
		});
	}
});

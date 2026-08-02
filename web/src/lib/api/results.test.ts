import { describe, it, expect, vi } from 'vitest';
import { resultsApi } from './results';
import type { Result } from '$lib/types/results';

function makeFetchReturning(results: Partial<Result>[]): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status: 200,
		json: () => Promise.resolve({ results })
	}) as unknown as typeof fetch;
}

describe('resultsApi.list', () => {
	const cases = [
		{
			name: 'returns results for org',
			results: [{ id: 'res_1', collection_name: 'smoke' }],
			expected: 1
		},
		{
			name: 'returns empty list',
			results: [],
			expected: 0
		},
		{
			name: 'passes limit param to backend',
			results: [],
			limit: 50,
			assertParamLimit: '50'
		}
	];

	for (const c of cases) {
		it(c.name, async () => {
			const fetchFn = makeFetchReturning(c.results);
			const result = await resultsApi.list('org_123', { fetch: fetchFn, limit: c.limit });

			if (c.expected !== undefined) {
				expect(result).toHaveLength(c.expected);
			}

			if (c.assertParamLimit) {
				const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
				const url = call[0] as string;
				expect(url).toContain(`limit=${c.assertParamLimit}`);
			}
		});
	}
});

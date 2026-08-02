import { describe, it, expect, vi } from 'vitest';
import { organizationsApi } from './organizations';
import type { Organization } from '$lib/types/organization';

function makeFetchReturning(orgs: Partial<Organization>[]): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status: 200,
		json: () => Promise.resolve({ organizations: orgs })
	}) as unknown as typeof fetch;
}

describe('organizationsApi.findBySlug', () => {
	const cases = [
		{
			name: 'returns matching org',
			orgs: [{ id: 'org_1', slug: 'acme', name: 'Acme' }],
			slug: 'acme',
			expected: { slug: 'acme' }
		},
		{
			name: 'returns null on no match',
			orgs: [{ id: 'org_2', slug: 'other', name: 'Other' }],
			slug: 'acme',
			expected: null
		},
		{
			name: 'returns null on empty list',
			orgs: [],
			slug: 'acme',
			expected: null
		}
	];

	for (const c of cases) {
		it(c.name, async () => {
			const fetchFn = makeFetchReturning(c.orgs);
			const result = await organizationsApi.findBySlug(c.slug, { fetch: fetchFn });
			if (c.expected === null) {
				expect(result).toBeNull();
			} else {
				expect(result).toMatchObject(c.expected);
			}
		});
	}
});

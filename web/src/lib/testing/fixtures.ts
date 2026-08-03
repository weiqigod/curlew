import type { Organization } from '$lib/types/organization';

/**
 * Builds a complete {@link Organization} for page-component tests.
 *
 * Route `PageData` types are structural, so a partial org literal fails
 * `svelte-check` even though the component only reads a couple of fields.
 * Defaults describe a team-tier org the current user owns.
 */
export function makeOrg(overrides: Partial<Organization> = {}): Organization {
	return {
		id: 'org_test',
		name: 'Acme',
		slug: 'acme',
		role: 'owner',
		seat_count: 3,
		seat_limit: 10,
		status: 'active',
		created_at: '2026-04-01T00:00:00Z',
		tier: 'team',
		...overrides
	};
}

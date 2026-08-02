import { describe, it, expect } from 'vitest';
import { requireAuth, requireTeamTier, requireOrgAdmin, requireOrgOwner, requireEnterpriseTier } from './guards';
import type { Organization } from '$lib/types/organization';

function makeOrg(tier: Organization['tier'] = 'free'): Organization {
	return {
		id: 'org_1',
		name: 'Acme',
		slug: 'acme',
		role: 'owner',
		seat_count: 1,
		seat_limit: 10,
		status: 'active',
		created_at: '2026-04-01T00:00:00Z',
		tier
	};
}

describe('requireAuth', () => {
	it('returns token when present', () => {
		const token = requireAuth('tok123', '/org/acme/results');
		expect(token).toBe('tok123');
	});

	it('throws redirect when no token', () => {
		expect(() => requireAuth(null, '/org/acme/results')).toThrow();
	});
});

describe('requireTeamTier', () => {
	const cases: Array<{
		name: string;
		org?: Organization | null;
		tier?: Organization['tier'];
		expected: 'pass' | 'redirect' | 'error';
	}> = [
		{ name: 'team tier passes through', tier: 'team', expected: 'pass' },
		{ name: 'enterprise tier passes through', tier: 'enterprise', expected: 'pass' },
		{ name: 'professional tier redirects', tier: 'professional', expected: 'redirect' },
		{ name: 'solo tier redirects', tier: 'solo', expected: 'redirect' },
		{ name: 'free tier redirects', tier: 'free', expected: 'redirect' },
		{ name: 'null org throws 404', org: null, expected: 'error' }
	];

	for (const c of cases) {
		it(c.name, () => {
			const org: Organization | null = 'org' in c && c.org !== undefined ? c.org : makeOrg(c.tier);
			if (c.expected === 'pass') {
				const result = requireTeamTier(org, 'acme');
				expect(result).toBeTruthy();
			} else {
				expect(() => requireTeamTier(org, 'acme')).toThrow();
			}
		});
	}
});

describe('requireOrgAdmin', () => {
	const cases: Array<{
		name: string;
		role: Organization['role'];
		expected: 'pass' | 'redirect';
	}> = [
		{ name: 'owner passes', role: 'owner', expected: 'pass' },
		{ name: 'admin passes', role: 'admin', expected: 'pass' },
		{ name: 'member redirects', role: 'member', expected: 'redirect' }
	];

	for (const c of cases) {
		it(c.name, () => {
			const org: Organization = { ...makeOrg('team'), role: c.role };
			if (c.expected === 'pass') {
				const result = requireOrgAdmin(org, 'acme');
				expect(result).toBeTruthy();
				expect(result.role).toBe(c.role);
			} else {
				expect(() => requireOrgAdmin(org, 'acme')).toThrow();
			}
		});
	}
});

describe('requireOrgOwner', () => {
	const cases: Array<{
		name: string;
		role: Organization['role'];
		expected: 'pass' | 'redirect';
	}> = [
		{ name: 'owner passes', role: 'owner', expected: 'pass' },
		{ name: 'admin redirects', role: 'admin', expected: 'redirect' },
		{ name: 'member redirects', role: 'member', expected: 'redirect' }
	];

	for (const c of cases) {
		it(c.name, () => {
			const org: Organization = { ...makeOrg('team'), role: c.role };
			if (c.expected === 'pass') {
				const result = requireOrgOwner(org, 'acme');
				expect(result).toBeTruthy();
				expect(result.role).toBe('owner');
			} else {
				expect(() => requireOrgOwner(org, 'acme')).toThrow();
			}
		});
	}
});

describe('requireEnterpriseTier', () => {
	const cases: Array<{
		name: string;
		org?: Organization | null;
		tier?: Organization['tier'];
		expected: 'pass' | 'redirect' | 'error';
	}> = [
		{ name: 'enterprise tier passes', tier: 'enterprise', expected: 'pass' },
		{ name: 'team tier redirects', tier: 'team', expected: 'redirect' },
		{ name: 'professional tier redirects', tier: 'professional', expected: 'redirect' },
		{ name: 'solo tier redirects', tier: 'solo', expected: 'redirect' },
		{ name: 'free tier redirects', tier: 'free', expected: 'redirect' },
		{ name: 'null org throws 404', org: null, expected: 'error' }
	];

	for (const c of cases) {
		it(c.name, () => {
			const org: Organization | null = 'org' in c && c.org !== undefined ? c.org : makeOrg(c.tier);
			if (c.expected === 'pass') {
				const result = requireEnterpriseTier(org, 'acme');
				expect(result).toBeTruthy();
			} else {
				expect(() => requireEnterpriseTier(org, 'acme')).toThrow();
			}
		});
	}
});

import { redirect, error } from '@sveltejs/kit';
import type { Organization, OrgRole } from '$lib/types/organization';

const ADMIN_ROLES: OrgRole[] = ['owner', 'admin'];

/**
 * Asserts that a valid access token is present.
 * Throws a 303 redirect to `/login` if not authenticated.
 * Returns the token string for further use.
 */
export function requireAuth(accessToken: string | null, redirectTo: string): string {
	if (!accessToken) {
		throw redirect(303, `/login?redirect=${encodeURIComponent(redirectTo)}`);
	}
	return accessToken;
}

/**
 * Asserts that the current user's role in the given org is owner or admin.
 * Throws a 303 redirect to /org/[slug]?toast=admin_required for members.
 * Use AFTER requireTeamTier so non-team-tier orgs are caught first.
 */
export function requireOrgAdmin(org: Organization, slug: string): Organization {
	if (ADMIN_ROLES.includes(org.role)) return org;
	throw redirect(303, `/org/${slug}?toast=admin_required`);
}

/**
 * Asserts the current user is the owner of the given org.
 * Redirects admins and members to /org/[slug]?toast=owner_required.
 * Use AFTER requireTeamTier so non-team-tier orgs are caught first.
 */
export function requireOrgOwner(org: Organization, slug: string): Organization {
	if (org.role === 'owner') return org;
	throw redirect(303, `/org/${slug}?toast=owner_required`);
}

/**
 * Asserts that the given organization exists and is on Team or Enterprise.
 * - Throws 404 if org is null.
 * - Throws 303 redirect to /org/[slug] below Team tier.
 */
export function requireTeamTier(org: Organization | null, slug: string): Organization {
	if (!org) {
		throw error(404, 'Organization not found');
	}
	if (org.tier !== 'team' && org.tier !== 'enterprise') {
		throw redirect(303, `/org/${slug}?toast=team_tier_required`);
	}
	return org;
}

/**
 * Asserts the org is on the Enterprise tier.
 * - Throws 404 if org is null.
 * - Throws 303 redirect to /org/[slug]?toast=enterprise_tier_required for free/solo/professional tiers.
 */
export function requireEnterpriseTier(org: Organization | null, slug: string): Organization {
	if (!org) {
		throw error(404, 'Organization not found');
	}
	if (org.tier === 'enterprise') return org;
	throw redirect(303, `/org/${slug}?toast=enterprise_tier_required`);
}

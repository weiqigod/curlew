import type { PageServerLoad } from './$types';
import { requireAuth, requireEnterpriseTier, requireOrgOwner } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { rolesApi } from '$lib/api/roles';
import { membersApi } from '$lib/api/members';
import type { RoleView } from '$lib/types/roles';
import type { Member } from '$lib/types/members';

/** Loads the roles list and per-role member counts for an owner of an enterprise-tier org. */
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const entOrg = requireEnterpriseTier(org, params.slug);
	const ownerOrg = requireOrgOwner(entOrg, params.slug);

	try {
		const [roles, members] = await Promise.all([
			rolesApi.list(ownerOrg.id, { token, fetch }),
			membersApi.list(ownerOrg.id, { token, fetch })
		]);
		return {
			org: ownerOrg,
			roles,
			memberCounts: countByRole(roles, members),
			error: null as string | null
		};
	} catch (e) {
		const message = e instanceof Error ? e.message : 'Failed to load roles.';
		return {
			org: ownerOrg,
			roles: [] as RoleView[],
			memberCounts: {} as Record<string, number>,
			error: message
		};
	}
};

/**
 * Counts how many members reference each role.
 * Members with a custom role assignment populate `role_id`; built-in roles
 * are identified by the `role` string field (owner | admin | member).
 *
 * The returned record is keyed by `role.id` so the page can render counts
 * using `memberCounts[role.id]`. For built-in roles, `role.id` (e.g.
 * `"builtin_owner"`) differs from `role.name` (e.g. `"owner"`), which is
 * what the member's `role` field holds. We build an alias map from name →
 * id for built-in roles and resolve members through it.
 */
function countByRole(roles: RoleView[], members: Member[]): Record<string, number> {
	// Seed every role to zero, keyed by id.
	const counts: Record<string, number> = {};
	for (const r of roles) {
		counts[r.id] = 0;
	}

	// For built-in roles, build a reverse lookup: name (e.g. "owner") → id.
	const builtinNameToId: Record<string, string> = {};
	for (const r of roles) {
		if (r.is_builtin) {
			builtinNameToId[r.name] = r.id;
		}
	}

	for (const m of members) {
		// Custom-role members carry role_id; built-in members carry only role (name).
		const key = m.role_id ? m.role_id : (builtinNameToId[m.role] ?? m.role);
		if (key in counts) {
			counts[key]++;
		}
	}
	return counts;
}

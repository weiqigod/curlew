import { api, type RequestOptions } from './client';
import type {
	RoleView,
	ListRolesResponse,
	CreateRoleRequest,
	UpdateRoleRequest
} from '$lib/types/roles';

/** Typed API client for organization custom-role endpoints. */
export const rolesApi = {
	/** Lists all roles (built-in and custom) for the given organization. Owner-only. */
	async list(orgId: string, opts?: RequestOptions): Promise<RoleView[]> {
		const { roles } = await api.get<ListRolesResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/roles`,
			opts
		);
		return roles;
	},

	/** Creates a new custom role for the given organization. Owner-only. */
	async create(
		orgId: string,
		body: CreateRoleRequest,
		opts?: RequestOptions
	): Promise<RoleView> {
		return api.post<RoleView>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/roles`,
			body,
			opts
		);
	},

	/**
	 * Replaces the name and permission set of a custom role. Owner-only.
	 * The UI sends a full replacement body — no delta computation.
	 */
	async update(
		orgId: string,
		roleId: string,
		body: UpdateRoleRequest,
		opts?: RequestOptions
	): Promise<RoleView> {
		return api.patch<RoleView>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(roleId)}`,
			body,
			opts
		);
	},

	/** Deletes a custom role. Owner-only. Throws 409 role_in_use if members use the role. */
	async remove(orgId: string, roleId: string, opts?: RequestOptions): Promise<void> {
		await api.delete<void>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(roleId)}`,
			opts
		);
	}
};

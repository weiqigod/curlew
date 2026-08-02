/** Wire DTO for a role (built-in or custom) returned by GET/POST. */
export interface RoleView {
	id: string;
	name: string;
	permissions: string[];
	is_builtin: boolean;
	created_at: string | null;
}

/** Response shape for GET /organizations/{id}/roles. */
export interface ListRolesResponse {
	roles: RoleView[];
}

/** Request body for POST /organizations/{id}/roles. */
export interface CreateRoleRequest {
	name: string;
	permissions: string[];
}

/** Request body for PATCH /organizations/{id}/roles/{role_id}.
 * Uses full-replace semantics — the entire permission set is sent,
 * mirroring POST.
 */
export interface UpdateRoleRequest {
	name: string;
	permissions: string[];
}

/** Categories used to group the permission picker. Order matters — it is the
 * display order in the UI and mirrors the spec's permission matrix. */
export type PermissionCategory =
	| 'Member Management'
	| 'Billing & Subscription'
	| 'Organization Settings'
	| 'Service Tokens'
	| 'Test Resources'
	| 'Team Dashboard';

/** A single entry in the permission catalogue. */
export interface PermissionDef {
	key: string;
	label: string;
	description: string;
	category: PermissionCategory;
}

/** Full permission catalogue. Keep in sync with backend Permissions.cs. */
export const PERMISSION_CATALOGUE: readonly PermissionDef[] = [
	// Member Management
	{
		category: 'Member Management',
		key: 'members.invite',
		label: 'Invite members',
		description: 'Invite new members'
	},
	{
		category: 'Member Management',
		key: 'members.remove',
		label: 'Remove members',
		description: 'Remove members'
	},
	{
		category: 'Member Management',
		key: 'members.view',
		label: 'View members',
		description: 'View member list'
	},
	{
		category: 'Member Management',
		key: 'roles.change',
		label: 'Change member roles',
		description: 'Change member roles'
	},
	{
		category: 'Member Management',
		key: 'roles.view',
		label: 'View member roles',
		description: 'View member roles'
	},
	// Billing & Subscription
	{
		category: 'Billing & Subscription',
		key: 'billing.manage',
		label: 'Manage billing',
		description: 'Update payment, change plan'
	},
	{
		category: 'Billing & Subscription',
		key: 'billing.view',
		label: 'View billing',
		description: 'View invoices, status'
	},
	{
		category: 'Billing & Subscription',
		key: 'seats.add',
		label: 'Add seats',
		description: 'Add seats to subscription'
	},
	{
		category: 'Billing & Subscription',
		key: 'seats.remove',
		label: 'Remove seats',
		description: 'Remove seats'
	},
	// Organization Settings
	{
		category: 'Organization Settings',
		key: 'org.settings.manage',
		label: 'Manage settings',
		description: 'Update name, logo, settings'
	},
	{
		category: 'Organization Settings',
		key: 'org.settings.view',
		label: 'View settings',
		description: 'View settings'
	},
	{
		category: 'Organization Settings',
		key: 'org.delete',
		label: 'Delete organization',
		description: 'Delete organization'
	},
	{
		category: 'Organization Settings',
		key: 'org.transfer',
		label: 'Transfer ownership',
		description: 'Transfer ownership'
	},
	// Service Tokens
	{
		category: 'Service Tokens',
		key: 'tokens.create',
		label: 'Create tokens',
		description: 'Create org-scoped tokens'
	},
	{
		category: 'Service Tokens',
		key: 'tokens.revoke',
		label: 'Revoke tokens',
		description: 'Revoke org tokens'
	},
	{
		category: 'Service Tokens',
		key: 'tokens.view',
		label: 'View tokens',
		description: 'View org token list'
	},
	// Test Resources
	{
		category: 'Test Resources',
		key: 'results.view',
		label: 'View results',
		description: 'View test results'
	},
	{
		category: 'Test Resources',
		key: 'results.upload',
		label: 'Upload results',
		description: 'Upload test results'
	},
	{
		category: 'Test Resources',
		key: 'results.delete',
		label: 'Delete results',
		description: 'Delete test results'
	},
	{
		category: 'Test Resources',
		key: 'vault_config.manage',
		label: 'Manage vault config',
		description: 'Manage shared vault templates'
	},
	{
		category: 'Test Resources',
		key: 'vault_config.view',
		label: 'View vault config',
		description: 'Use shared vault configurations'
	},
	// Team Dashboard
	{
		category: 'Team Dashboard',
		key: 'dashboard.view',
		label: 'View dashboard',
		description: 'Access team dashboard'
	},
	{
		category: 'Team Dashboard',
		key: 'dashboard.export',
		label: 'Export reports',
		description: 'Export reports'
	}
];

/** Ordered category list — drives section rendering in the picker. */
export const PERMISSION_CATEGORIES: readonly PermissionCategory[] = [
	'Member Management',
	'Billing & Subscription',
	'Organization Settings',
	'Service Tokens',
	'Test Resources',
	'Team Dashboard'
];

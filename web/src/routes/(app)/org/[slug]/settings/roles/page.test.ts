import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import type { RoleView } from '$lib/types/roles';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

const invalidateAll = vi.fn();
vi.mock('$app/navigation', () => ({
	invalidateAll: () => invalidateAll()
}));

const create = vi.fn();
const update = vi.fn();
const remove = vi.fn();
vi.mock('$lib/api/roles', () => ({
	rolesApi: {
		create: (...args: unknown[]) => create(...args),
		update: (...args: unknown[]) => update(...args),
		remove: (...args: unknown[]) => remove(...args)
	}
}));

const ORG_ID = 'org_roles_test';

const builtinOwner: RoleView = {
	id: 'builtin_owner',
	name: 'owner',
	permissions: [],
	is_builtin: true,
	created_at: null
};
const builtinAdmin: RoleView = { ...builtinOwner, id: 'builtin_admin', name: 'admin' };
const builtinMember: RoleView = { ...builtinOwner, id: 'builtin_member', name: 'member' };

const customQaLead: RoleView = {
	id: 'role_qa_lead',
	name: 'qa-lead',
	permissions: ['results.view', 'results.upload', 'dashboard.view', 'members.view'],
	is_builtin: false,
	created_at: '2026-04-15T00:00:00Z'
};

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: ORG_ID, tier: 'enterprise' }),
		roles: [builtinOwner, builtinAdmin, builtinMember, customQaLead],
		// qa-lead is assigned to 2 members; owner to 1.
		memberCounts: { builtin_owner: 1, builtin_admin: 0, builtin_member: 0, role_qa_lead: 2 },
		error: null,
		...overrides
	};
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('roles page', () => {
	it('lists builtin + custom rows with member counts', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.getByTestId('roles-heading')).toBeVisible();
		expect(screen.getByTestId('roles-table')).toBeVisible();

		expect(screen.getByTestId('role-row-builtin_owner')).toBeVisible();
		expect(screen.getByTestId('role-row-builtin_admin')).toBeVisible();
		expect(screen.getByTestId('role-row-builtin_member')).toBeVisible();

		const qaRow = screen.getByTestId('role-row-role_qa_lead');
		expect(qaRow).toBeVisible();
		expect(qaRow).toHaveTextContent('qa-lead');
		// 4 permissions, 2 members assigned.
		expect(qaRow).toHaveTextContent('4');
		expect(qaRow).toHaveTextContent('2');
	});

	it('renders the server-supplied error instead of the table', () => {
		render(Page, { props: { data: makeData({ error: 'Failed to load roles.' }) } });

		expect(screen.queryByTestId('roles-table')).not.toBeInTheDocument();
		expect(screen.getByText('Failed to load roles.')).toBeVisible();
	});

	it('create modal submits the typed body and reloads on success', async () => {
		const user = userEvent.setup();
		const newRole: RoleView = {
			id: 'role_new_001',
			name: 'release-manager',
			permissions: ['results.view', 'results.upload', 'tokens.view', 'dashboard.view'],
			is_builtin: false,
			created_at: '2026-04-19T00:00:00Z'
		};
		create.mockResolvedValue(newRole);

		const { rerender } = render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('roles-create-button'));
		expect(screen.getByTestId('role-modal')).toBeVisible();

		await user.type(screen.getByTestId('role-name-input'), 'release-manager');
		await user.click(screen.getByTestId('perm-results.view'));
		await user.click(screen.getByTestId('perm-results.upload'));
		await user.click(screen.getByTestId('perm-tokens.view'));
		await user.click(screen.getByTestId('perm-dashboard.view'));

		await user.click(screen.getByTestId('role-submit'));

		await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
		expect(create).toHaveBeenCalledWith(ORG_ID, {
			name: 'release-manager',
			permissions: ['results.view', 'results.upload', 'tokens.view', 'dashboard.view']
		});

		// Modal closes, success toast shows, and the page asks SvelteKit to re-load.
		await waitFor(() => expect(screen.queryByTestId('role-modal')).not.toBeInTheDocument());
		expect(screen.getByTestId('toast-message')).toHaveTextContent('Role created');
		expect(invalidateAll).toHaveBeenCalledTimes(1);

		// The re-load is what surfaces the new row.
		await rerender({
			data: makeData({
				roles: [builtinOwner, builtinAdmin, builtinMember, customQaLead, newRole],
				memberCounts: { role_new_001: 0 }
			})
		});
		expect(screen.getByTestId('role-row-role_new_001')).toBeVisible();
	});

	it('surfaces a create failure in the modal without closing it', async () => {
		const user = userEvent.setup();
		create.mockRejectedValue(new ApiError('role_exists', 'Role already exists', 409));

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('roles-create-button'));
		await user.type(screen.getByTestId('role-name-input'), 'qa-lead');
		await user.click(screen.getByTestId('role-submit'));

		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Role already exists'));
		expect(screen.getByTestId('role-modal')).toBeVisible();
		expect(invalidateAll).not.toHaveBeenCalled();
	});

	it('edit modal pre-fills the role and submits a full-replacement PATCH body', async () => {
		const user = userEvent.setup();
		update.mockResolvedValue({ ...customQaLead });

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('role-edit-role_qa_lead'));
		expect(screen.getByTestId('role-modal')).toBeVisible();
		expect(screen.getByTestId('role-name-input')).toHaveValue('qa-lead');
		// Existing permissions arrive pre-checked.
		expect(screen.getByTestId('perm-results.view')).toBeChecked();

		await user.click(screen.getByTestId('perm-results.delete'));
		await user.click(screen.getByTestId('role-submit'));

		await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
		const [orgId, roleId, body] = update.mock.calls[0] as [
			string,
			string,
			{ name: string; permissions: string[] }
		];
		expect(orgId).toBe(ORG_ID);
		expect(roleId).toBe('role_qa_lead');
		expect(body.name).toBe('qa-lead');
		expect(body.permissions).toContain('results.delete');
		// Full replace — the untouched permissions are still sent.
		expect(body.permissions).toEqual(expect.arrayContaining(customQaLead.permissions));

		expect(screen.getByTestId('toast-message')).toHaveTextContent('Role updated');
	});

	it('shows a 409 role_in_use toast carrying the member count', async () => {
		const user = userEvent.setup();
		vi.stubGlobal('confirm', vi.fn().mockReturnValue(true));
		remove.mockRejectedValue(
			new ApiError('role_in_use', 'Role is currently in use', 409, undefined, {
				member_count: 3
			})
		);

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('role-delete-role_qa_lead'));

		await waitFor(() =>
			expect(screen.getByTestId('toast-message')).toHaveTextContent(
				'Role is assigned to 3 members'
			)
		);
		expect(invalidateAll).not.toHaveBeenCalled();
	});

	it('does not delete when the confirm dialog is dismissed', async () => {
		const user = userEvent.setup();
		vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));

		render(Page, { props: { data: makeData() } });
		await user.click(screen.getByTestId('role-delete-role_qa_lead'));

		expect(remove).not.toHaveBeenCalled();
	});

	it('renders the 6 permission categories in spec order', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('roles-create-button'));

		const expectedSections = [
			'perm-section-member-management',
			'perm-section-billing-subscription',
			'perm-section-organization-settings',
			'perm-section-service-tokens',
			'perm-section-test-resources',
			'perm-section-team-dashboard'
		];
		for (const id of expectedSections) {
			expect(screen.getByTestId(id)).toBeVisible();
		}

		// Order matters — it mirrors the spec's permission matrix.
		const picker = screen.getByTestId('permission-picker');
		const rendered = [...picker.querySelectorAll('[data-testid^="perm-section-"]')].map((el) =>
			el.getAttribute('data-testid')
		);
		expect(rendered).toEqual(expectedSections);
	});

	it('gives built-in rows no edit/delete controls', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.queryByTestId('role-edit-builtin_owner')).not.toBeInTheDocument();
		expect(screen.queryByTestId('role-delete-builtin_owner')).not.toBeInTheDocument();

		expect(screen.getByTestId('role-edit-role_qa_lead')).toBeVisible();
		expect(screen.getByTestId('role-delete-role_qa_lead')).toBeVisible();
	});
});

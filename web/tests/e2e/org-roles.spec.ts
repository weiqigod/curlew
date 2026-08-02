import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, MEMBER_EMAIL } from './helpers/fixtures';

const ROLES_URL = `/org/${SEEDED_ORG_SLUG}/settings/roles`;
const ORG_ID = 'org_roles_test';

function makeOrgPayload(role: 'owner' | 'admin' | 'member', tier = 'enterprise') {
	return {
		organizations: [
			{
				id: ORG_ID,
				slug: SEEDED_ORG_SLUG,
				name: 'Acme',
				role,
				seat_count: 3,
				seat_limit: 50,
				status: 'active',
				created_at: '2026-04-01T00:00:00Z',
				tier
			}
		]
	};
}

const builtinOwner = {
	id: 'builtin_owner',
	name: 'owner',
	permissions: [],
	is_builtin: true,
	created_at: null
};

const builtinAdmin = {
	id: 'builtin_admin',
	name: 'admin',
	permissions: [],
	is_builtin: true,
	created_at: null
};

const builtinMember = {
	id: 'builtin_member',
	name: 'member',
	permissions: [],
	is_builtin: true,
	created_at: null
};

const customQaLead = {
	id: 'role_qa_lead',
	name: 'qa-lead',
	permissions: ['results.view', 'results.upload', 'dashboard.view', 'members.view'],
	is_builtin: false,
	created_at: '2026-04-15T00:00:00Z'
};

const membersList = {
	members: [
		{ user_id: 'u1', role: 'owner', joined_at: '2026-01-01T00:00:00Z' },
		{ user_id: 'u2', role: 'member', role_id: 'role_qa_lead', joined_at: '2026-02-01T00:00:00Z' },
		{ user_id: 'u3', role: 'member', role_id: 'role_qa_lead', joined_at: '2026-03-01T00:00:00Z' }
	]
};

test.describe('Custom role editor', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);
		await context.route('**/api/v1/organizations', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(makeOrgPayload('owner')) })
		);
		await context.route('**/api/v1/organizations/*/roles', (r) => {
			if (r.request().method() === 'GET') {
				return r.fulfill({
					status: 200,
					body: JSON.stringify({ roles: [builtinOwner, builtinAdmin, builtinMember, customQaLead] })
				});
			}
			return r.continue();
		});
		await context.route('**/api/v1/organizations/*/members', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(membersList) })
		);
	});

	// Behavior 1
	test('loads roles page and lists builtin + custom rows with member counts', async ({
		page
	}) => {
		await page.goto(ROLES_URL);
		await expect(page.getByTestId('roles-heading')).toBeVisible();
		await expect(page.getByTestId('roles-table')).toBeVisible();

		// Three built-in rows
		await expect(page.getByTestId('role-row-builtin_owner')).toBeVisible();
		await expect(page.getByTestId('role-row-builtin_admin')).toBeVisible();
		await expect(page.getByTestId('role-row-builtin_member')).toBeVisible();

		// One custom row
		await expect(page.getByTestId('role-row-role_qa_lead')).toBeVisible();
		await expect(page.getByTestId('role-row-role_qa_lead')).toContainText('qa-lead');

		// Member count for qa-lead: 2 members have role_id=role_qa_lead
		await expect(page.getByTestId('role-row-role_qa_lead')).toContainText('2');
	});

	// Behavior 2
	test('create role modal submits POST and new row appears', async ({ page, context }) => {
		const newRole = {
			id: 'role_new_001',
			name: 'release-manager',
			permissions: ['results.view', 'results.upload', 'tokens.view', 'dashboard.view'],
			is_builtin: false,
			created_at: '2026-04-19T00:00:00Z'
		};

		await context.route('**/api/v1/organizations/*/roles', async (r) => {
			if (r.request().method() === 'POST') {
				return r.fulfill({ status: 201, body: JSON.stringify(newRole) });
			}
			if (r.request().method() === 'GET') {
				// After invalidateAll, return updated list
				return r.fulfill({
					status: 200,
					body: JSON.stringify({
						roles: [builtinOwner, builtinAdmin, builtinMember, customQaLead, newRole]
					})
				});
			}
			return r.continue();
		});

		await page.goto(ROLES_URL);
		await expect(page.getByTestId('roles-heading')).toBeVisible();

		// Open create modal
		await page.getByTestId('roles-create-button').click();
		await expect(page.getByTestId('role-modal')).toBeVisible();

		// Fill form
		await page.getByTestId('role-name-input').fill('release-manager');

		// Check 4 permissions
		await page.getByTestId('perm-results.view').check();
		await page.getByTestId('perm-results.upload').check();
		await page.getByTestId('perm-tokens.view').check();
		await page.getByTestId('perm-dashboard.view').check();

		// Submit
		await page.getByTestId('role-submit').click();

		// Modal closes and success toast appears
		await expect(page.getByTestId('role-modal')).not.toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId('toast-message')).toContainText('Role created', {
			timeout: 5000
		});

		// New role row appears after re-load
		await expect(page.getByTestId('role-row-role_new_001')).toBeVisible({ timeout: 5000 });
	});

	// Behavior 3
	test('edit existing custom role fires PATCH', async ({ page, context }) => {
		const updatedRole = {
			...customQaLead,
			permissions: [
				'results.view',
				'results.upload',
				'dashboard.view',
				'members.view',
				'results.delete'
			]
		};

		let patchBody: unknown = null;
		await context.route('**/api/v1/organizations/*/roles/*', async (r) => {
			if (r.request().method() === 'PATCH') {
				patchBody = JSON.parse(r.request().postData() ?? '{}');
				return r.fulfill({ status: 200, body: JSON.stringify(updatedRole) });
			}
			return r.continue();
		});

		await page.goto(ROLES_URL);
		await expect(page.getByTestId('roles-heading')).toBeVisible();

		// Open edit modal for custom role
		await page.getByTestId('role-edit-role_qa_lead').click();
		await expect(page.getByTestId('role-modal')).toBeVisible();

		// Add one more permission
		await page.getByTestId('perm-results.delete').check();

		// Submit
		await page.getByTestId('role-submit').click();

		await expect(page.getByTestId('toast-message')).toContainText('Role updated', {
			timeout: 5000
		});

		// Verify PATCH was called with the right body
		expect(patchBody).toBeTruthy();
		const body = patchBody as { name: string; permissions: string[] };
		expect(body.name).toBe('qa-lead');
		expect(body.permissions).toContain('results.delete');
	});

	// Behavior 4
	test('delete role in use shows 409 toast with member count', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/roles/*', async (r) => {
			if (r.request().method() === 'DELETE') {
				return r.fulfill({
					status: 409,
					body: JSON.stringify({
						code: 'role_in_use',
						message: 'Role is currently in use',
						member_count: 3
					})
				});
			}
			return r.continue();
		});

		await page.goto(ROLES_URL);
		await expect(page.getByTestId('roles-heading')).toBeVisible();

		// Set up dialog handler to auto-confirm
		page.on('dialog', (dialog) => dialog.accept());

		// Click delete on custom role
		await page.getByTestId('role-delete-role_qa_lead').click();

		// Error toast with member count should appear
		await expect(page.getByTestId('toast-message')).toContainText('Role is assigned to 3 members', {
			timeout: 5000
		});
	});

	// Behavior 5 — permission matrix categories
	test('permission picker renders 6 categories in spec order', async ({ page }) => {
		await page.goto(ROLES_URL);
		await page.getByTestId('roles-create-button').click();
		await expect(page.getByTestId('role-modal')).toBeVisible();

		const expectedSections = [
			'perm-section-member-management',
			'perm-section-billing-subscription',
			'perm-section-organization-settings',
			'perm-section-service-tokens',
			'perm-section-test-resources',
			'perm-section-team-dashboard'
		];

		for (const sectionId of expectedSections) {
			await expect(page.getByTestId(sectionId)).toBeVisible();
		}
	});

	// Behavior 6 — non-owner redirect
	test('non-owner is redirected with owner_required toast', async ({ page, context }) => {
		await context.unroute('**/api/v1/organizations').catch(() => {});
		await context.clearCookies();
		await seedAuthCookie(context, MEMBER_EMAIL);
		await context.route('**/api/v1/organizations', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(makeOrgPayload('member')) })
		);

		await page.goto(ROLES_URL);

		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}(\\?|$)`));
		await expect(page.getByTestId('toast-owner-required')).toBeVisible();
	});

	// Belt-and-braces: built-in rows have no edit/delete controls
	test('built-in rows have no edit/delete controls', async ({ page }) => {
		await page.goto(ROLES_URL);
		await expect(page.getByTestId('roles-heading')).toBeVisible();

		// Built-in roles should NOT have edit/delete buttons
		await expect(page.getByTestId('role-edit-builtin_owner')).not.toBeVisible();
		await expect(page.getByTestId('role-delete-builtin_owner')).not.toBeVisible();

		// Custom roles should have them
		await expect(page.getByTestId('role-edit-role_qa_lead')).toBeVisible();
		await expect(page.getByTestId('role-delete-role_qa_lead')).toBeVisible();
	});

	// DoD — subnav roles link is visible for owner
	test('roles sub-nav link is visible for owner of enterprise org', async ({ page }) => {
		await page.goto(ROLES_URL);
		await expect(page.getByTestId('subnav-roles-link')).toBeVisible();
	});
});

import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, MEMBER_EMAIL } from './helpers/fixtures';

const AUDIT_URL = `/org/${SEEDED_ORG_SLUG}/audit-log`;

function mockOrg(role: 'owner' | 'admin' | 'member' = 'owner', tier = 'enterprise') {
	return {
		organizations: [
			{
				id: 'org_audit',
				slug: SEEDED_ORG_SLUG,
				name: 'Acme',
				role,
				seat_count: 1,
				seat_limit: 50,
				status: 'active',
				created_at: '2026-04-01T00:00:00Z',
				tier
			}
		]
	};
}

function mockEntries(count: number, eventType = 'member.invited') {
	return Array.from({ length: count }, (_, i) => ({
		event_type: i % 2 === 0 ? eventType : 'sso.login',
		user_id: `user_${i}`,
		target_type: 'invitation',
		target_id: `inv_${i}`,
		created_at: new Date(Date.UTC(2026, 3, 15 - i)).toISOString(),
		ip_address: '10.0.0.1',
		success: true,
		failure_reason: null
	}));
}

test.describe('Audit log viewer', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);
		await context.route('**/api/v1/organizations', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(mockOrg('owner')) })
		);
	});

	// Behaviors 1 + 2
	test('renders 10 newest-first rows with expected columns', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(20) }) })
		);
		await page.goto(AUDIT_URL);
		await expect(page.getByTestId('audit-log-heading')).toBeVisible();
		const rows = page.getByTestId('audit-log-table').locator('tbody tr');
		await expect(rows).toHaveCount(10);
		await expect(rows.first()).toContainText('member.invited');
		// Behavior 1: all five column headers must be present
		const headers = page.getByTestId('audit-log-table').locator('thead th');
		await expect(headers).toHaveCount(5);
	});

	// Behavior 2 — pagination
	test('pagination navigates between pages', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(20) }) })
		);
		await page.goto(AUDIT_URL);
		await page.getByTestId('audit-log-next').click();
		await expect(page).toHaveURL(/page=2/);
		const rows = page.getByTestId('audit-log-table').locator('tbody tr');
		await expect(rows).toHaveCount(10);
	});

	// Behavior 3 — event_type filter
	test('event_type filter updates URL and narrows rows', async ({ page, context }) => {
		let captured: string | null = null;
		await context.route('**/api/v1/organizations/*/audit-log**', (r) => {
			const u = new URL(r.request().url());
			if (u.searchParams.get('event_type') === 'member.invited') {
				captured = 'member.invited';
				return r.fulfill({
					status: 200,
					body: JSON.stringify({ items: mockEntries(5, 'member.invited') })
				});
			}
			return r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(20) }) });
		});
		await page.goto(AUDIT_URL);
		await page.getByTestId('audit-log-event-filter').selectOption('member.invited');
		await expect(page).toHaveURL(/event_type=member\.invited/);
		await expect(page.getByTestId('audit-log-table').locator('tbody tr')).toHaveCount(5);
		expect(captured).toBe('member.invited');
	});

	// Behavior 4 — date range
	test('range "last 7 days" adds ?from= and ?to= to URL', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(10) }) })
		);
		await page.goto(AUDIT_URL);
		await page.getByTestId('range-7d').click();
		await expect(page).toHaveURL(/from=/);
		await expect(page).toHaveURL(/to=/);
	});

	// Behavior 5 — CSV export
	test('Export CSV triggers a download with correct header row', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(3) }) })
		);
		await page.goto(AUDIT_URL);
		const [download] = await Promise.all([
			page.waitForEvent('download'),
			page.getByTestId('audit-log-export').click()
		]);
		const stream = await download.createReadStream();
		const chunks: Buffer[] = [];
		for await (const chunk of stream!) chunks.push(chunk as Buffer);
		const text = Buffer.concat(chunks).toString('utf8');
		expect(text.split('\r\n')[0]).toBe('event_type,user,target,timestamp,ip');
		expect(text).toContain('member.invited');
	});

	// Behavior 6 — non-admin redirect
	test('non-admin is redirected with admin_required toast', async ({ page, context }) => {
		await context.unroute('**/api/v1/organizations').catch(() => {});
		await context.clearCookies();
		await seedAuthCookie(context, MEMBER_EMAIL);
		await context.route('**/api/v1/organizations', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(mockOrg('member')) })
		);
		await page.goto(AUDIT_URL);
		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}(\\?|$)`));
		await expect(page.getByTestId('toast-admin-required')).toBeVisible();
	});

	// DoD — subnav link visible for admins of enterprise-tier orgs
	test('audit-log subnav link is visible for owner of enterprise org', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify({ items: [] }) })
		);
		await page.goto(AUDIT_URL);
		await expect(page.getByTestId('subnav-audit-log-link')).toBeVisible();
	});
});

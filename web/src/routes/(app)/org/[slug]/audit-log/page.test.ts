import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AuditLogEntry } from '$lib/types/audit-log';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

const goto = vi.fn();
vi.mock('$app/navigation', () => ({ goto: (...a: unknown[]) => goto(...a) }));

vi.mock('$app/stores', async () => {
	const { readable } = await import('svelte/store');
	return { page: readable({ url: new URL('http://localhost/org/acme/audit-log?page=2') }) };
});

function entries(count: number, eventType = 'member.invited'): AuditLogEntry[] {
	return Array.from({ length: count }, (_, i) => ({
		event_type: i % 2 === 0 ? eventType : 'sso.login',
		user_id: `user_${i}`,
		user_email: null,
		target_type: 'invitation',
		target_id: `inv_${i}`,
		created_at: new Date(Date.UTC(2026, 3, 15 - i)).toISOString(),
		ip_address: '10.0.0.1',
		success: true,
		failure_reason: null
	}));
}

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: 'org_audit', tier: 'enterprise' }),
		entries: entries(10),
		totalCount: 20,
		page: 2,
		totalPages: 2,
		filter: { event_type: null, from: null, to: null },
		error: null,
		...overrides
	};
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('audit-log page', () => {
	it('renders one row per entry with all six columns', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.getByTestId('audit-log-heading')).toBeVisible();
		const table = screen.getByTestId('audit-log-table');
		expect(table.querySelectorAll('tbody tr')).toHaveLength(10);
		expect(table.querySelectorAll('thead th')).toHaveLength(6);
		expect(table.querySelectorAll('tbody tr')[0]).toHaveTextContent('member.invited');
	});

	it('prefers the actor email over the raw user id, falling back to a dash', () => {
		render(
			Page,
			{
				props: {
					data: makeData({
						entries: [
							{ ...entries(1)[0], user_email: 'owner@example.com', user_id: 'user_0' },
							{ ...entries(1)[0], user_email: null, user_id: 'user_9' },
							{ ...entries(1)[0], user_email: null, user_id: null }
						]
					})
				}
			}
		);

		const rows = screen.getByTestId('audit-log-table').querySelectorAll('tbody tr');
		expect(rows[0]).toHaveTextContent('owner@example.com');
		expect(rows[1]).toHaveTextContent('user_9');
		expect(rows[2]).toHaveTextContent('—');
	});

	it('renders a failure row with its reason', () => {
		render(
			Page,
			{
				props: {
					data: makeData({
						entries: [
							{
								...entries(1)[0],
								event_type: 'sso.login_failed',
								success: false,
								failure_reason: 'bad signature'
							}
						]
					})
				}
			}
		);

		expect(screen.getByTestId('audit-log-table')).toHaveTextContent('failure (bad signature)');
	});

	it('navigates to the next page, preserving the path', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData({ page: 1 }) } });

		await user.click(screen.getByTestId('audit-log-next'));

		expect(goto).toHaveBeenCalledWith(
			'/org/acme/audit-log?page=2',
			expect.objectContaining({ keepFocus: true, noScroll: true })
		);
	});

	it('disables Prev on the first page and Next on the last', () => {
		const { rerender } = render(Page, { props: { data: makeData({ page: 1, totalPages: 2 }) } });
		expect(screen.getByTestId('audit-log-prev')).toBeDisabled();
		expect(screen.getByTestId('audit-log-next')).toBeEnabled();

		rerender({ data: makeData({ page: 2, totalPages: 2 }) });
		expect(screen.getByTestId('audit-log-prev')).toBeEnabled();
		expect(screen.getByTestId('audit-log-next')).toBeDisabled();
	});

	it('applies the event_type filter and resets to page 1', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.selectOptions(screen.getByTestId('audit-log-event-filter'), 'member.invited');

		await waitFor(() => expect(goto).toHaveBeenCalledTimes(1));
		const [target] = goto.mock.calls[0] as [string];
		expect(target).toContain('event_type=member.invited');
		// Changing a filter must drop the stale page cursor.
		expect(target).not.toContain('page=');
	});

	it('clearing the event filter removes the param entirely', async () => {
		const user = userEvent.setup();
		render(
			Page,
			{ props: { data: makeData({ filter: { event_type: 'sso.login', from: null, to: null } }) } }
		);

		await user.selectOptions(screen.getByTestId('audit-log-event-filter'), '');

		await waitFor(() => expect(goto).toHaveBeenCalledTimes(1));
		expect((goto.mock.calls[0] as [string])[0]).not.toContain('event_type');
	});

	it('the 7d range preset adds from and to bounds', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('range-7d'));

		await waitFor(() => expect(goto).toHaveBeenCalledTimes(1));
		const target = (goto.mock.calls[0] as [string])[0];
		const params = new URL(target, 'http://localhost').searchParams;
		expect(params.get('from')).toBeTruthy();
		expect(params.get('to')).toBeTruthy();

		const from = Date.parse(params.get('from')!);
		const to = Date.parse(params.get('to')!);
		expect(to - from).toBe(7 * 86_400_000);
	});

	it('the All range preset clears both bounds', async () => {
		const user = userEvent.setup();
		render(
			Page,
			{
				props: {
					data: makeData({
						filter: { event_type: null, from: '2026-04-01T00:00:00Z', to: '2026-04-08T00:00:00Z' }
					})
				}
			}
		);

		await user.click(screen.getByTestId('range-all'));

		await waitFor(() => expect(goto).toHaveBeenCalledTimes(1));
		const target = (goto.mock.calls[0] as [string])[0];
		expect(target).not.toContain('from=');
		expect(target).not.toContain('to=');
	});

	it('points Export CSV at the export route, carrying filters but not the page cursor', () => {
		render(Page, { props: { data: makeData() } });

		const link = screen.getByTestId('audit-log-export');
		expect(link).toHaveAttribute('download');
		const href = link.getAttribute('href')!;
		expect(href.startsWith('/org/acme/audit-log/export')).toBe(true);
		expect(href).not.toContain('page=');
	});

	it('shows the empty state when the filter matches nothing', () => {
		render(Page, { props: { data: makeData({ entries: [], totalCount: 0, totalPages: 1 }) } });

		expect(screen.getByTestId('empty-state')).toHaveTextContent(
			'No audit-log entries for the selected filter.'
		);
		expect(screen.queryByTestId('audit-log-table')).not.toBeInTheDocument();
		// Filters stay usable so the user can widen the search.
		expect(screen.getByTestId('audit-log-filters')).toBeVisible();
	});

	it('shows an error state instead of the table when load failed', () => {
		render(Page, { props: { data: makeData({ error: 'Failed to load audit log.' }) } });

		expect(screen.getByTestId('error-state')).toHaveTextContent('Failed to load audit log.');
		expect(screen.queryByTestId('audit-log-table')).not.toBeInTheDocument();
	});
});

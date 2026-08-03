import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import type { UserExportRequest } from '$lib/types/user-data';
import Page from './+page.svelte';

const createExportRequest = vi.fn();
const getExportRequest = vi.fn();
vi.mock('$lib/api/user-data', () => ({
	userDataApi: {
		createExportRequest: (...a: unknown[]) => createExportRequest(...a),
		getExportRequest: (...a: unknown[]) => getExportRequest(...a)
	}
}));

// The delete panel has its own suite; stub its API so it stays inert here.
vi.mock('$lib/api/user-deletion', () => ({
	userDeletionApi: { issueReauthToken: vi.fn(), requestDeletion: vi.fn() }
}));

const QUEUED: UserExportRequest = {
	id: 'e2e-req-001',
	status: 'queued',
	created_at: '2026-05-18T12:00:00Z'
};

const READY: UserExportRequest = {
	...QUEUED,
	status: 'ready',
	ready_at: '2026-05-18T12:01:00Z',
	expires_at: '2026-05-19T12:01:00Z',
	signed_url: 'http://test/objects/bundle.json?exp=1748001660'
};

function requestButton() {
	return screen.getByTestId('request-export-button');
}

beforeEach(() => {
	vi.clearAllMocks();
	vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
	vi.useRealTimers();
});

describe('account data export', () => {
	it('requests an export, shows the queued badge, then polls through to ready', async () => {
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		createExportRequest.mockResolvedValue(QUEUED);
		getExportRequest.mockResolvedValueOnce(QUEUED).mockResolvedValueOnce(READY);

		render(Page);

		expect(requestButton()).toBeEnabled();
		await user.click(requestButton());

		await waitFor(() => expect(document.querySelector('[data-status="queued"]')).toBeVisible());
		// A non-terminal request keeps the button disabled so no duplicate is filed.
		expect(requestButton()).toBeDisabled();

		// Two 5s poll ticks: queued again, then ready.
		await vi.advanceTimersByTimeAsync(5000);
		await vi.advanceTimersByTimeAsync(5000);

		const link = await screen.findByRole('link', { name: 'Download bundle' });
		expect(link).toHaveAttribute('href', READY.signed_url!);
		expect(link).toHaveAttribute('download', 'curlew-export.json');
		expect(getExportRequest).toHaveBeenCalledWith('e2e-req-001');
	});

	it('stops polling once the request reaches a terminal status', async () => {
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		createExportRequest.mockResolvedValue(READY);

		render(Page);
		await user.click(requestButton());

		await screen.findByRole('link', { name: 'Download bundle' });

		await vi.advanceTimersByTimeAsync(20_000);
		// A terminal response is never polled.
		expect(getExportRequest).not.toHaveBeenCalled();
	});

	it('explains the 24-hour limit when the backend rate-limits the request', async () => {
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		createExportRequest.mockRejectedValue(
			new ApiError('export_rate_limited', 'Export rate limited', 429)
		);

		render(Page);
		await user.click(requestButton());

		const alert = await screen.findByRole('alert');
		expect(alert).toHaveTextContent('24 hours');
		// The button frees up so the user can retry later.
		expect(requestButton()).toBeEnabled();
	});

	it('shows a generic error for a non-429 failure', async () => {
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		createExportRequest.mockRejectedValue(new ApiError('server_error', 'boom', 500));

		render(Page);
		await user.click(requestButton());

		const alert = await screen.findByRole('alert');
		expect(alert).toHaveTextContent('Failed to request data export.');
		expect(alert).not.toHaveTextContent('24 hours');
	});

	it('reports the failure reason when the export build fails', async () => {
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		createExportRequest.mockResolvedValue({
			...QUEUED,
			status: 'failed',
			failure_reason: 'object store unreachable'
		});

		render(Page);
		await user.click(requestButton());

		expect(await screen.findByText(/object store unreachable/)).toBeVisible();
		expect(screen.queryByRole('link', { name: 'Download bundle' })).not.toBeInTheDocument();
	});

	it('keeps rendering when a poll fails, rather than crashing the page', async () => {
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		createExportRequest.mockResolvedValue(QUEUED);
		getExportRequest.mockRejectedValue(new Error('network blip'));

		render(Page);
		await user.click(requestButton());
		await waitFor(() => expect(document.querySelector('[data-status="queued"]')).toBeVisible());

		await vi.advanceTimersByTimeAsync(5000);

		// Still on the queued badge; polling gave up quietly.
		expect(document.querySelector('[data-status="queued"]')).toBeVisible();
	});

	it('links the footer to the data-inventory document', () => {
		render(Page);

		const link = document.querySelector('footer a[href*="data-inventory"]');
		expect(link).toBeVisible();
		expect(link).toHaveAttribute('target', '_blank');
		expect(link?.getAttribute('rel')).toContain('noopener');
	});
});

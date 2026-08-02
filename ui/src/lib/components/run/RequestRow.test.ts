// RequestRow component tests (§13.2): all six live statuses, verbatim skip
// reason, error rendering, terminal-only clickability.
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { LiveRequest } from '../../event-reducer';
import RequestRow from './RequestRow.svelte';

function row(overrides: Partial<LiveRequest> = {}): LiveRequest {
  return {
    request_id: 'req-1',
    slug: 'create-user',
    name: 'Create user',
    method: 'POST',
    phase: 'main',
    status: 'pending',
    source_file: 'collections/users.yaml',
    source_line: 12,
    wave_index: -1,
    retry_count: 0,
    ...overrides,
  };
}

afterEach(cleanup);

describe('RequestRow', () => {
  it('renders a pending row dimmed and not clickable', async () => {
    const onOpen = vi.fn();
    const { component, container } = render(RequestRow, { props: { row: row() } });
    component.$on('open', onOpen);
    const el = container.querySelector('.rr');
    expect(el?.classList.contains('pending')).toBe(true);
    expect(el?.classList.contains('clickable')).toBe(false);
    await fireEvent.click(el as Element);
    expect(onOpen).not.toHaveBeenCalled();
  });

  it('renders a running row with live elapsed in the middle slot', () => {
    render(RequestRow, {
      props: { row: row({ status: 'running', at_ms: 0 }), liveElapsedMs: 412 },
    });
    expect(screen.getByText('412 ms')).toBeTruthy();
  });

  it('renders a passed row with duration and status text, clickable', async () => {
    const onOpen = vi.fn();
    const { component, container } = render(RequestRow, {
      props: { row: row({ status: 'passed', status_code: 201, duration_ms: 184 }) },
    });
    component.$on('open', onOpen);
    expect(screen.getByText('184 ms')).toBeTruthy();
    expect(screen.getByText('201 Created')).toBeTruthy();
    await fireEvent.click(container.querySelector('.rr') as Element);
    expect(onOpen).toHaveBeenCalledOnce();
  });

  it('renders a failed row with the fail message in --err', () => {
    const { container } = render(RequestRow, {
      props: {
        row: row({
          status: 'failed',
          status_code: 422,
          duration_ms: 90,
          fail_message: 'status: expected 201, got 422',
        }),
      },
    });
    const msg = container.querySelector('.mid .err');
    expect(msg?.textContent).toBe('status: expected 201, got 422');
  });

  it('renders an error row with category + first message line in --err2', () => {
    const { container } = render(RequestRow, {
      props: {
        row: row({
          status: 'error',
          error: {
            category: 'network',
            code: 'NETWORK_CONNECTION_REFUSED',
            message: 'dial tcp 10.0.0.5:443: connect: connection refused\nsecond line',
          },
        }),
      },
    });
    const msg = container.querySelector('.mid .err2');
    expect(msg?.textContent).toBe('network: dial tcp 10.0.0.5:443: connect: connection refused');
  });

  it('renders the verbatim skip reason and stays clickable (skip panel)', async () => {
    const onOpen = vi.fn();
    const { component, container } = render(RequestRow, {
      props: { row: row({ status: 'skipped', skip_reason: 'dependency "Get Token" failed' }) },
    });
    component.$on('open', onOpen);
    expect(screen.getByText('dependency "Get Token" failed')).toBeTruthy();
    await fireEvent.click(container.querySelector('.rr') as Element);
    expect(onOpen).toHaveBeenCalledOnce();
  });

  it('shows the generic placeholder until reconcile fills the skip reason', () => {
    render(RequestRow, { props: { row: row({ status: 'skipped' }) } });
    expect(screen.getByText('skipped')).toBeTruthy();
  });

  it('names iteration rows "<base_name> i/N"', () => {
    render(RequestRow, {
      props: {
        row: row({
          status: 'passed',
          status_code: 200,
          duration_ms: 10,
          iteration: { index: 1, total: 3, base_name: 'Create user', base_slug: 'create-user' },
        }),
      },
    });
    expect(screen.getByText('Create user 2/3')).toBeTruthy();
  });
});

// DefinitionPanel tests: renders the raw definition from the tree store,
// falls back gracefully when the request is gone, and never shows resolved
// values (the URL stays a template).
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { startRun } from '../../controller';
import { getRequestDetail } from '../../api/runs';
import { navigate } from '../../router';
import { selectedEnv } from '../../stores/environments';
import { lastRunInfo, resetRun, runMeta, runState, seedRequests } from '../../stores/run';
import { tree } from '../../stores/tree';
import type { RequestDetail, RequestListEntry } from '../../types/run';
import type { Tree } from '../../types/tree';
import DefinitionPanel from './DefinitionPanel.svelte';

vi.mock('../../controller', () => ({ startRun: vi.fn(), cancelActiveRun: vi.fn() }));
vi.mock('../../api/runs', () => ({ getRequestDetail: vi.fn() }));
vi.mock('../../router', async (original) => ({
  ...await original<typeof import('../../router')>(),
  navigate: vi.fn(),
}));

afterEach(cleanup);

const FIXTURE: Tree = {
  etag: 't1',
  collections: [
    {
      path: 'collections/users.yaml',
      name: 'User Flow',
      valid: true,
      counts: { setup: 0, main: 1, teardown: 0 },
      requests: [
        {
          name: 'Create user',
          slug: 'create-user',
          phase: 'main',
          method: 'POST',
          url: '{{base_url}}/users',
          source_line: 7,
          data_driven: false,
          required: true,
        },
      ],
      issues: [],
    },
  ],
};

beforeEach(() => {
  vi.clearAllMocks();
  resetRun('previous-run');
  runState.set('idle');
  selectedEnv.set('test');
  tree.set(FIXTURE);
});

describe('DefinitionPanel', () => {
  it('renders method, name, template URL, and source location', () => {
    render(DefinitionPanel, { props: { path: 'collections/users.yaml', slug: 'create-user' } });
    expect(screen.getByText('POST')).toBeTruthy();
    expect(screen.getByText('Create user')).toBeTruthy();
    // Template span split by TemplateUrl: the {{base_url}} part stays raw.
    expect(screen.getByText('{{base_url}}')).toBeTruthy();
    expect(screen.getByText('collections/users.yaml:7')).toBeTruthy();
    expect(screen.getByText('required')).toBeTruthy();
    expect(screen.getByRole('button', { name: /run this request/i })).toBeTruthy();
  });

  it('falls back to a not-found card when the slug is missing from the tree', () => {
    render(DefinitionPanel, { props: { path: 'collections/users.yaml', slug: 'gone' } });
    expect(screen.getByText('request not found')).toBeTruthy();
  });

  it('falls back when the collection path is unknown', () => {
    render(DefinitionPanel, { props: { path: 'collections/nope.yaml', slug: 'create-user' } });
    expect(screen.getByText('request not found')).toBeTruthy();
  });

  it('runs a main request with the selected environment', async () => {
    render(DefinitionPanel, { props: { path: 'collections/users.yaml', slug: 'create-user' } });
    await fireEvent.click(screen.getByRole('button', { name: /run this request/i }));
    expect(startRun).toHaveBeenCalledWith({
      collection: 'collections/users.yaml',
      env: 'test',
      parallel: false,
      mode: 'selection',
      selection: ['Create user'],
      rerun_of: null,
    }, { navigate: false });
  });

  it.each(['setup', 'teardown'])('uses the correct action for a %s request', async (phase) => {
    const collection = FIXTURE.collections[0];
    tree.set({
      ...FIXTURE,
      collections: [{
        ...collection,
        requests: [
          ...collection.requests,
          { ...collection.requests[0], phase, slug: `${phase}-create-user` },
        ],
      }],
    });
    const { component } = render(DefinitionPanel, {
      props: { path: 'collections/users.yaml', slug: 'create-user' },
    });
    expect(screen.getByRole('button', { name: /run this request/i })).toBeTruthy();
    await component.$set({ slug: `${phase}-create-user` });
    if (phase === 'setup') {
      await fireEvent.click(screen.getByRole('button', { name: /run this request/i }));
      expect(startRun).toHaveBeenCalledWith({
        collection: 'collections/users.yaml', env: 'test', parallel: false,
        mode: 'setup', selection: ['Create user'], rerun_of: null,
      }, { navigate: false });
    } else {
      expect(screen.queryByRole('button', { name: /run this request/i })).toBeNull();
      expect(startRun).not.toHaveBeenCalled();
    }
  });

  it.each(['starting', 'running', 'cancelling'] as const)('does not start a request while %s', async (state) => {
    runState.set(state);
    render(DefinitionPanel, { props: { path: 'collections/users.yaml', slug: 'create-user' } });
    const button = screen.getByRole('button', { name: /run this request/i });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    await fireEvent.click(button);
    expect(startRun).not.toHaveBeenCalled();
  });

  it('shows errors inline and runs again without leaving the request or losing the tabs', async () => {
    const entry: RequestListEntry = {
      request_id: 'req-2', slug: 'create-user', name: 'Create user', phase: 'main',
      method: 'POST', outcome: 'error', duration_ms: 0, wave_index: -1, retry_count: 0,
      skip_reason: null, fail_message: null, error: { category: 'network', message: 'EOF' },
      iteration: null, source_file: 'collections/users.yaml', source_line: 7,
    };
    const detail: RequestDetail = {
      ...entry, url: 'http://127.0.0.1/users', timing: null, retry: null, skipped: false,
      warnings: null,
      source: { file: 'collections/users.yaml', line: 7, snippet: null, snippet_start_line: 0 },
      request: null, response: null, assertions: null,
    };
    vi.mocked(getRequestDetail).mockResolvedValue(detail);
    resetRun('first-run', {
      collection: 'collections/users.yaml', env: 'test', mode: 'selection',
      selection: ['Create user'], parallel: false, rerun_of: null,
    });
    seedRequests([entry]);
    runState.set('error');
    render(DefinitionPanel, { props: { path: 'collections/users.yaml', slug: 'create-user' } });
    await screen.findByText('EOF');
    expect(screen.queryByRole('button', { name: 'back to run view' })).toBeNull();
    await fireEvent.click(screen.getByRole('tab', { name: 'Request' }));
    expect(screen.getByText(/definition in collections\/users.yaml:7/)).toBeTruthy();
    expect(navigate).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByRole('button', { name: /run this request/i }));
    expect(startRun).toHaveBeenLastCalledWith(expect.objectContaining({
      collection: 'collections/users.yaml', selection: ['Create user'], env: 'test',
    }), { navigate: false });
    resetRun('second-run', {
      collection: 'collections/users.yaml', env: 'test', mode: 'selection',
      selection: ['Create user'], parallel: false, rerun_of: null,
    });
    runState.set('running');
    await waitFor(() => expect(screen.queryByText('EOF')).toBeNull());
    vi.mocked(getRequestDetail).mockResolvedValue({ ...detail, error: { category: 'network', message: 'New failure' } });
    seedRequests([entry]);
    runState.set('error');
    await screen.findByText('New failure');
    expect(screen.getByRole('button', { name: /run this request/i })).toBeTruthy();
    expect(navigate).not.toHaveBeenCalled();
  });

  it('shows successful external auth beside a rejected main request and opens either result in place', async () => {
    const auth: RequestListEntry = {
      request_id: 'req-1', slug: 'get-token', name: 'Get token', phase: 'setup',
      method: 'POST', outcome: 'passed', status_code: 200, duration_ms: 1, wave_index: -1,
      retry_count: 0, skip_reason: null, fail_message: null, error: null, iteration: null,
      source_file: 'auth/token.yaml', source_line: 1,
    };
    const main: RequestListEntry = {
      ...auth, request_id: 'req-2', slug: 'create-user', name: 'Create user', phase: 'main',
      outcome: 'failed', status_code: 401, source_file: 'collections/users.yaml',
    };
    vi.mocked(getRequestDetail).mockImplementation(async (_runId, requestId) => ({
      ...(requestId === 'req-1' ? auth : main), url: 'http://localhost/test',
      timing: null, retry: null, skipped: false, warnings: null, source: null,
      request: null, response: null, assertions: null,
    }));
    resetRun('auth-run', {
      collection: 'collections/users.yaml', env: 'test', mode: 'selection',
      selection: ['Create user'], parallel: false, rerun_of: null,
    });
    seedRequests([auth, main]);
    runState.set('completed');
    render(DefinitionPanel, { props: { path: 'collections/users.yaml', slug: 'create-user' } });
    const authResult = screen.getByRole('button', { name: /setup.*Get token.*passed.*200/i });
    const mainResult = screen.getByRole('button', { name: /main.*Create user.*failed.*401/i });
    await waitFor(() => expect(getRequestDetail).toHaveBeenLastCalledWith('auth-run', 'req-2'));
    await fireEvent.click(authResult);
    await waitFor(() => expect(getRequestDetail).toHaveBeenLastCalledWith('auth-run', 'req-1'));
    await fireEvent.click(mainResult);
    await waitFor(() => expect(getRequestDetail).toHaveBeenLastCalledWith('auth-run', 'req-2'));
    expect(navigate).not.toHaveBeenCalled();
    expect(startRun).not.toHaveBeenCalled();
  });

  it('refreshes an early response when final run details become available', async () => {
    const entry: RequestListEntry = {
      request_id: 'req-1', slug: 'create-user', name: 'Create user', phase: 'main',
      method: 'POST', outcome: 'passed', status_code: 200, duration_ms: 1, wave_index: -1,
      retry_count: 0, skip_reason: null, fail_message: null, error: null, iteration: null,
      source_file: 'collections/users.yaml', source_line: 7,
    };
    const detail: RequestDetail = {
      ...entry, url: 'http://localhost/users', timing: null, retry: null, skipped: false,
      warnings: null, source: null, request: null, response: null, assertions: null,
    };
    vi.mocked(getRequestDetail).mockResolvedValue(detail);
    resetRun('finishing-run', {
      collection: 'collections/users.yaml', env: 'test', mode: 'selection',
      selection: ['Create user'], parallel: false, rerun_of: null,
    });
    seedRequests([entry]);
    runState.set('running');
    render(DefinitionPanel, { props: { path: 'collections/users.yaml', slug: 'create-user' } });
    await screen.findByText('no response body');
    await fireEvent.click(screen.getByRole('tab', { name: /Assertions/ }));
    vi.mocked(getRequestDetail).mockResolvedValue({
      ...detail,
      assertions: { passed: true, items: [{ type: 'status', label: 'Final status assertion', expected: '200', actual: '200', passed: true }] },
    });
    lastRunInfo.set({
      run_id: 'finishing-run', state: 'completed', source: 'memory',
      summary: { total: 1, passed: 1, failed: 0, skipped: 0, error: 0, duration_ms: 1, parallel: false },
    });
    runState.set('completed');
    await screen.findByText(/Final status assertion.*expected 200/);
    expect(getRequestDetail).toHaveBeenCalledTimes(2);
    expect(navigate).not.toHaveBeenCalled();
  });

  it('shows a run configuration failure beside the persistent run control', async () => {
    resetRun('failed-run', {
      collection: 'collections/users.yaml', env: 'test', mode: 'selection',
      selection: ['Create user'], parallel: false, rerun_of: null,
    });
    runMeta.update((value) => ({ ...value, error: { category: 'config', message: 'Missing test configuration' } }));
    runState.set('error');
    render(DefinitionPanel, { props: { path: 'collections/users.yaml', slug: 'create-user' } });
    await screen.findByText('Missing test configuration');
    expect((screen.getByRole('button', { name: /run this request/i }) as HTMLButtonElement).disabled).toBe(false);
  });
});

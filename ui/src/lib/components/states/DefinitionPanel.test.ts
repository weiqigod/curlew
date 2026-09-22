// DefinitionPanel tests: renders the raw definition from the tree store,
// falls back gracefully when the request is gone, and never shows resolved
// values (the URL stays a template).
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { startRun } from '../../controller';
import { selectedEnv } from '../../stores/environments';
import { runState } from '../../stores/run';
import { tree } from '../../stores/tree';
import type { Tree } from '../../types/tree';
import DefinitionPanel from './DefinitionPanel.svelte';

vi.mock('../../controller', () => ({ startRun: vi.fn() }));

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
    });
  });

  it.each(['setup', 'teardown'])('does not offer a main selection for a %s request', async (phase) => {
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
    expect(screen.queryByRole('button', { name: /run this request/i })).toBeNull();
    expect(startRun).not.toHaveBeenCalled();
  });

  it.each(['starting', 'running', 'cancelling'] as const)('does not start a request while %s', async (state) => {
    runState.set(state);
    render(DefinitionPanel, { props: { path: 'collections/users.yaml', slug: 'create-user' } });
    const button = screen.getByRole('button', { name: /run this request/i });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    await fireEvent.click(button);
    expect(startRun).not.toHaveBeenCalled();
  });
});

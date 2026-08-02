// DefinitionPanel tests: renders the raw definition from the tree store,
// falls back gracefully when the request is gone, and never shows resolved
// values (the URL stays a template).
import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { tree } from '../../stores/tree';
import type { Tree } from '../../types/tree';
import DefinitionPanel from './DefinitionPanel.svelte';

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
});

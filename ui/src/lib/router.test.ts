import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { formatRoute, parseHash, route, type Route } from './router';

describe('parseHash', () => {
  it('parses the root run view', () => {
    expect(parseHash('')).toEqual({ name: 'run' });
    expect(parseHash('#')).toEqual({ name: 'run' });
    expect(parseHash('#/')).toEqual({ name: 'run' });
  });

  it('parses #/tree/<collectionPath> with URL-encoded slashes', () => {
    expect(parseHash('#/tree/collections%2Fusers.yaml')).toEqual({
      name: 'tree',
      path: 'collections/users.yaml',
    });
    // Lenient: unencoded slashes also resolve.
    expect(parseHash('#/tree/collections/users.yaml')).toEqual({
      name: 'tree',
      path: 'collections/users.yaml',
    });
  });

  it('parses #/runs/<run_id>', () => {
    expect(parseHash('#/runs/a1b2c3')).toEqual({ name: 'runDetail', runId: 'a1b2c3' });
  });

  it('parses the inspector with and without a tab', () => {
    expect(parseHash('#/runs/a1b2c3/requests/req-3')).toEqual({
      name: 'inspector',
      runId: 'a1b2c3',
      requestId: 'req-3',
    });
    expect(parseHash('#/runs/a1b2c3/requests/req-3?tab=timing')).toEqual({
      name: 'inspector',
      runId: 'a1b2c3',
      requestId: 'req-3',
      tab: 'timing',
    });
    // Unknown tab values are dropped (default applies downstream).
    expect(parseHash('#/runs/a1b2c3/requests/req-3?tab=bogus')).toEqual({
      name: 'inspector',
      runId: 'a1b2c3',
      requestId: 'req-3',
    });
  });

  it('decodes synthetic request ids ("slug:<slug>")', () => {
    expect(parseHash('#/runs/a1b2c3/requests/slug%3Acreate-user')).toEqual({
      name: 'inspector',
      runId: 'a1b2c3',
      requestId: 'slug:create-user',
    });
  });

  it('parses #/compare with and without query params', () => {
    expect(parseHash('#/compare')).toEqual({ name: 'compare' });
    expect(parseHash('#/compare?base=r1&target=r2&slug=create-user&iter=2&changes=1')).toEqual({
      name: 'compare',
      base: 'r1',
      target: 'r2',
      slug: 'create-user',
      iter: 2,
      changes: true,
    });
  });

  it('parses #/file/<collectionPath>', () => {
    expect(parseHash('#/file/collections%2Fbroken.yaml')).toEqual({
      name: 'file',
      path: 'collections/broken.yaml',
    });
  });

  it('parses #/def/<collectionPath>/<slug>', () => {
    expect(parseHash('#/def/collections%2Fusers.yaml/create-user')).toEqual({
      name: 'definition',
      path: 'collections/users.yaml',
      slug: 'create-user',
    });
    expect(parseHash('#/def/onlypath')).toEqual({ name: 'notFound', hash: '#/def/onlypath' });
    expect(parseHash('#/def//slug')).toEqual({ name: 'notFound', hash: '#/def//slug' });
  });

  it('returns notFound for unknown hashes', () => {
    expect(parseHash('#/nope')).toEqual({ name: 'notFound', hash: '#/nope' });
    expect(parseHash('#/runs/')).toEqual({ name: 'notFound', hash: '#/runs/' });
    expect(parseHash('#/runs/x/requests/')).toEqual({ name: 'notFound', hash: '#/runs/x/requests/' });
    expect(parseHash('#/tree/')).toEqual({ name: 'notFound', hash: '#/tree/' });
    expect(parseHash('#bare')).toEqual({ name: 'notFound', hash: '#bare' });
  });
});

describe('formatRoute', () => {
  it('round-trips every route shape', () => {
    const routes: Route[] = [
      { name: 'run' },
      { name: 'tree', path: 'collections/users.yaml' },
      { name: 'runDetail', runId: 'a1b2c3' },
      { name: 'inspector', runId: 'a1b2c3', requestId: 'req-3' },
      { name: 'inspector', runId: 'a1b2c3', requestId: 'slug:create-user', tab: 'error' },
      { name: 'compare' },
      { name: 'compare', base: 'r1', target: 'r2', slug: 's', iter: 0, changes: true },
      { name: 'file', path: 'collections/broken.yaml' },
      { name: 'definition', path: 'collections/users.yaml', slug: 'create-user' },
    ];
    for (const r of routes) {
      expect(parseHash(formatRoute(r)), formatRoute(r)).toEqual(r);
    }
  });

  it('URL-encodes collection paths', () => {
    expect(formatRoute({ name: 'tree', path: 'collections/users.yaml' })).toBe(
      '#/tree/collections%2Fusers.yaml',
    );
  });

  it('encodes the changes-only toggle as changes=1', () => {
    expect(formatRoute({ name: 'compare', base: 'a', target: 'b', changes: true })).toBe(
      '#/compare?base=a&target=b&changes=1',
    );
  });
});

describe('route store', () => {
  it('tracks hashchange events', async () => {
    location.hash = '#/';
    expect(get(route)).toEqual({ name: 'run' });

    const changed = new Promise<void>((resolve) => {
      window.addEventListener('hashchange', () => resolve(), { once: true });
    });
    location.hash = '#/runs/abc123';
    await changed;
    expect(get(route)).toEqual({ name: 'runDetail', runId: 'abc123' });

    location.hash = '#/';
  });
});

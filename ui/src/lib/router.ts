// Hand-rolled, store-backed hash router (UI_SPECIFICATION.md §10.4.1–§10.4.2).
//
//   #/                                        run view (current/last run)
//   #/tree/<collectionPath>                   run view filtered to a collection
//   #/runs/<run_id>                           run view pinned to a run
//   #/runs/<run_id>/requests/<request_id>     inspector (?tab=…)
//   #/compare                                 compare (?base=&target=&slug=&iter=&changes=1)
//   #/file/<collectionPath>                   validation panel

import { readable } from 'svelte/store';

export type InspectorTab = 'error' | 'body' | 'headers' | 'assertions' | 'timing' | 'request';

export type Route =
  | { name: 'run' }
  | { name: 'tree'; path: string }
  | { name: 'runDetail'; runId: string }
  | { name: 'inspector'; runId: string; requestId: string; tab?: InspectorTab }
  | {
      name: 'compare';
      base?: string;
      target?: string;
      slug?: string;
      iter?: number;
      changes?: boolean;
    }
  | { name: 'file'; path: string }
  | { name: 'notFound'; hash: string };

const INSPECTOR_TABS: readonly string[] = ['error', 'body', 'headers', 'assertions', 'timing', 'request'];

/** Parses a location.hash value (with or without the leading '#') into a Route. */
export function parseHash(hash: string): Route {
  let h = hash.startsWith('#') ? hash.slice(1) : hash;
  let query = new URLSearchParams();
  const qIdx = h.indexOf('?');
  if (qIdx >= 0) {
    query = new URLSearchParams(h.slice(qIdx + 1));
    h = h.slice(0, qIdx);
  }
  if (h === '' || h === '/') {
    return { name: 'run' };
  }
  if (!h.startsWith('/')) {
    return { name: 'notFound', hash };
  }
  const rest = h.slice(1);

  if (rest.startsWith('tree/')) {
    const path = decodeURIComponent(rest.slice('tree/'.length));
    return path === '' ? { name: 'notFound', hash } : { name: 'tree', path };
  }
  if (rest.startsWith('file/')) {
    const path = decodeURIComponent(rest.slice('file/'.length));
    return path === '' ? { name: 'notFound', hash } : { name: 'file', path };
  }
  if (rest === 'compare') {
    const route: Route = { name: 'compare' };
    const base = query.get('base');
    const target = query.get('target');
    const slug = query.get('slug');
    const iter = query.get('iter');
    if (base !== null) route.base = base;
    if (target !== null) route.target = target;
    if (slug !== null) route.slug = slug;
    if (iter !== null && /^\d+$/.test(iter)) route.iter = parseInt(iter, 10);
    if (query.get('changes') === '1') route.changes = true;
    return route;
  }
  if (rest.startsWith('runs/')) {
    const segments = rest.slice('runs/'.length).split('/');
    if (segments.length === 1 && segments[0] !== '') {
      return { name: 'runDetail', runId: decodeURIComponent(segments[0]) };
    }
    if (segments.length === 3 && segments[1] === 'requests' && segments[0] !== '' && segments[2] !== '') {
      const route: Route = {
        name: 'inspector',
        runId: decodeURIComponent(segments[0]),
        requestId: decodeURIComponent(segments[2]),
      };
      const tab = query.get('tab');
      if (tab !== null && INSPECTOR_TABS.includes(tab)) route.tab = tab as InspectorTab;
      return route;
    }
  }
  return { name: 'notFound', hash };
}

/** Formats a Route back into a '#/…' hash string (inverse of parseHash). */
export function formatRoute(route: Route): string {
  switch (route.name) {
    case 'run':
      return '#/';
    case 'tree':
      return `#/tree/${encodeURIComponent(route.path)}`;
    case 'runDetail':
      return `#/runs/${encodeURIComponent(route.runId)}`;
    case 'inspector': {
      const base = `#/runs/${encodeURIComponent(route.runId)}/requests/${encodeURIComponent(route.requestId)}`;
      return route.tab !== undefined ? `${base}?tab=${route.tab}` : base;
    }
    case 'compare': {
      const q = new URLSearchParams();
      if (route.base !== undefined) q.set('base', route.base);
      if (route.target !== undefined) q.set('target', route.target);
      if (route.slug !== undefined) q.set('slug', route.slug);
      if (route.iter !== undefined) q.set('iter', String(route.iter));
      if (route.changes) q.set('changes', '1');
      const qs = q.toString();
      return qs === '' ? '#/compare' : `#/compare?${qs}`;
    }
    case 'file':
      return `#/file/${encodeURIComponent(route.path)}`;
    case 'notFound':
      return route.hash;
  }
}

/** Readable store of the current route, driven by hashchange. */
export const route = readable<Route>(
  parseHash(typeof location !== 'undefined' ? location.hash : ''),
  (set) => {
    const update = () => set(parseHash(location.hash));
    update();
    window.addEventListener('hashchange', update);
    return () => window.removeEventListener('hashchange', update);
  },
);

/** Navigates by setting location.hash (pushes a history entry). */
export function navigate(to: Route): void {
  location.hash = formatRoute(to);
}

// /tree payload + etag-guarded refresh (UI_SPECIFICATION.md §10.3.1).

import { writable } from 'svelte/store';
import { getTree } from '../api/tree';
import type { Tree } from '../types/tree';

export const tree = writable<Tree | null>(null);

/** Collection-path filter driving #/tree/<path> (null = no filter). */
export const treeFilter = writable<string | null>(null);

let currentEtag: string | null = null;
tree.subscribe(($tree) => {
  currentEtag = $tree?.etag ?? null;
});

/**
 * Refetches /tree. When `etag` is supplied (from hello or files.changed
 * frames) and matches the loaded tree, the fetch is skipped.
 */
export async function refreshTree(etag?: string): Promise<void> {
  if (etag !== undefined && etag === currentEtag) return;
  tree.set(await getTree());
}

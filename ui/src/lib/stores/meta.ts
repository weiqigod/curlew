// /meta payload + feature helper (UI_SPECIFICATION.md §10.3.1).

import { writable } from 'svelte/store';
import { getMeta } from '../api/meta';
import type { Meta } from '../types/meta';

export const meta = writable<Meta | null>(null);

export async function loadMeta(): Promise<Meta> {
  const m = await getMeta();
  meta.set(m);
  return m;
}

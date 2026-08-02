import { apiFetch, apiFetchRaw } from './client';
import type { Tree } from '../types/tree';

/** GET /api/v1/tree (§4.3). */
export function getTree(): Promise<Tree> {
  return apiFetch<Tree>('/tree');
}

/** GET /api/v1/files?path= (§4.6) — raw YAML source, jailed to the project root. */
export async function getFile(path: string): Promise<string> {
  const res = await apiFetchRaw(`/files?path=${encodeURIComponent(path)}`);
  return res.text();
}

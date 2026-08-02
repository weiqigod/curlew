import { apiFetch } from './client';
import type { Meta } from '../types/meta';

/** GET /api/v1/meta (§4.2). */
export function getMeta(): Promise<Meta> {
  return apiFetch<Meta>('/meta');
}

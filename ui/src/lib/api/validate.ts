import { apiFetch } from './client';
import type { ValidateResult } from '../types/tree';

/** GET /api/v1/validate?path= (§4.5) — all tree files when path is omitted. */
export function validate(path?: string): Promise<ValidateResult> {
  const qs = path !== undefined ? `?path=${encodeURIComponent(path)}` : '';
  return apiFetch<ValidateResult>(`/validate${qs}`);
}

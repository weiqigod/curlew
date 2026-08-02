import { apiFetch } from './client';
import type { CompareResult } from '../types/compare';

/** GET /api/v1/compare?base=&target= (§4.12, Solo). */
export function getCompare(base: string, target: string): Promise<CompareResult> {
  return apiFetch<CompareResult>(
    `/compare?base=${encodeURIComponent(base)}&target=${encodeURIComponent(target)}`,
  );
}

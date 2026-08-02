import { apiFetch } from './client';
import type { Environment, EnvironmentsResponse } from '../types/tree';

/** GET /api/v1/environments (§4.4). Values are pre-redacted server-side. */
export async function getEnvironments(): Promise<Environment[]> {
  const res = await apiFetch<EnvironmentsResponse>('/environments');
  return res.environments;
}

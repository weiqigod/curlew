import { apiFetch } from './client';

/**
 * POST /api/v1/open (§4.13) — launch the user's editor at file:line.
 * 204 on launch; 409 no_editor when nothing is configured.
 */
export function openInEditor(file: string, line: number): Promise<void> {
  return apiFetch<void>('/open', {
    method: 'POST',
    body: JSON.stringify({ file, line }),
  });
}

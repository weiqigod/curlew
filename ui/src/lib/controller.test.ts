import { get } from 'svelte/store';
import { expect, it, vi } from 'vitest';
import { getRun, getRunRequests } from './api/runs';
import { reconcile } from './controller';
import { meta } from './stores/meta';
import { reconciledSummary, resetRun } from './stores/run';
import type { RunInfo } from './types/run';

vi.mock('./api/runs', () => ({ getRun: vi.fn(), getRunRequests: vi.fn() }));

it('does not cache a pre-terminal summary instead of the final run results', async () => {
  meta.set(null);
  resetRun('finishing-run');
  const interim: RunInfo = {
    run_id: 'finishing-run', state: 'running', source: 'active',
    summary: { total: 3, passed: 3, failed: 0, skipped: 0, error: 0, duration_ms: 0, parallel: false },
  };
  const final: RunInfo = {
    ...interim, state: 'completed', source: 'memory',
    summary: { ...interim.summary, parallel: true, wave_count: 2, max_parallelism: 2, duration_ms: 42 },
  };
  vi.mocked(getRun).mockResolvedValueOnce(interim).mockResolvedValueOnce(final);
  vi.mocked(getRunRequests).mockResolvedValue({ requests: [] });
  await reconcile('finishing-run');
  expect(get(reconciledSummary)).toBeNull();
  await reconcile('finishing-run');
  expect(get(reconciledSummary)).toEqual(final.summary);
  await reconcile('finishing-run');
  expect(getRun).toHaveBeenCalledTimes(2);
});
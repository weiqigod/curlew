// SummaryStrip component tests (§13.2): error bucket, parallel affordances,
// env echo, past-run chip, elapsed/authoritative duration swap.
import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import type { SummaryView } from '../../stores/run';
import SummaryStrip from './SummaryStrip.svelte';

afterEach(cleanup);

function summary(overrides: Partial<SummaryView> = {}): SummaryView {
  return {
    total: 14,
    pending: 0,
    running: 0,
    passed: 11,
    failed: 2,
    skipped: 1,
    error: 0,
    completed: 14,
    live: false,
    duration_ms: 3400,
    parallel: false,
    ...overrides,
  };
}

describe('SummaryStrip', () => {
  it('hides the error bucket when zero and shows it (with count) when > 0', () => {
    const { unmount } = render(SummaryStrip, { props: { summary: summary() } });
    expect(screen.queryByText('error')).toBeNull();
    unmount();
    render(SummaryStrip, {
      props: { summary: summary({ error: 2, passed: 9, completed: 14 }) },
    });
    expect(screen.getByText('error')).toBeTruthy();
    expect(screen.getByLabelText('2 error')).toBeTruthy();
  });

  it('shows wave affordances for finished parallel single-collection runs', () => {
    render(SummaryStrip, {
      props: {
        summary: summary({ parallel: true, wave_count: 3, max_parallelism: 4 }),
        running: false,
        batch: false,
      },
    });
    expect(screen.getByText(/3 waves/)).toBeTruthy();
    expect(screen.getByText(/max 4∥/)).toBeTruthy();
  });

  it('omits wave affordances for batch runs', () => {
    render(SummaryStrip, {
      props: {
        summary: summary({ parallel: true, wave_count: 3, max_parallelism: 4 }),
        running: false,
        batch: true,
      },
    });
    expect(screen.queryByText(/3 waves/)).toBeNull();
  });

  it('shows live elapsed + streaming while running, authoritative total after', () => {
    const { unmount } = render(SummaryStrip, {
      props: {
        summary: summary({ running: 2, completed: 8, live: true }),
        running: true,
        elapsedMs: 1240,
      },
    });
    expect(screen.getByText('1.2s elapsed')).toBeTruthy();
    expect(screen.getByText(/streaming/)).toBeTruthy();
    expect(screen.getByText('running')).toBeTruthy();
    unmount();
    render(SummaryStrip, { props: { summary: summary(), running: false } });
    expect(screen.getByText('3.4 s total')).toBeTruthy();
    expect(screen.getByText(/run finished/)).toBeTruthy();
  });

  it('echoes the env the run used and the past-run chip', () => {
    render(SummaryStrip, {
      props: { summary: summary(), envName: 'staging', pastRunRelative: '4m ago' },
    });
    expect(screen.getByText(/env: staging/)).toBeTruthy();
    expect(screen.getByText(/viewing past run · 4m ago/)).toBeTruthy();
  });
});

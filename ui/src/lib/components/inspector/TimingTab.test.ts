// TimingTab component tests (§13.2): full / reused / nil-timing / attempts.
import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import type { Retry, Timing } from '../../types/run';
import TimingTab from './TimingTab.svelte';

afterEach(cleanup);

const fullTiming: Timing = {
  dns_us: 1234,
  connect_us: 2100,
  tls_us: 15400,
  ttfb_us: 48200,
  download_us: 900,
  total_us: 68100,
  connection_reused: false,
  attempts: 1,
};

describe('TimingTab', () => {
  it('renders the full waterfall with all phases and a pinned total', () => {
    render(TimingTab, { props: { timing: fullTiming } });
    expect(screen.getByText('DNS lookup')).toBeTruthy();
    expect(screen.getByText('TCP connect')).toBeTruthy();
    expect(screen.getByText('TLS handshake')).toBeTruthy();
    expect(screen.getByText('Waiting (TTFB)')).toBeTruthy();
    expect(screen.getByText('Content download')).toBeTruthy();
    expect(screen.getByText('total')).toBeTruthy();
    expect(screen.getByText('68.1 ms')).toBeTruthy();
    // accounted sum 67834/68100 — unaccounted < 5% → no hatched "other"
    expect(screen.queryByText('other')).toBeNull();
  });

  it('omits absent phases (plain http → no TLS row)', () => {
    const t: Timing = { ...fullTiming };
    delete t.tls_us;
    render(TimingTab, { props: { timing: t } });
    expect(screen.queryByText('TLS handshake')).toBeNull();
    expect(screen.getByText('DNS lookup')).toBeTruthy();
  });

  it('renders a hatched "other" segment when unaccounted time exceeds 5%', () => {
    render(TimingTab, {
      props: {
        timing: {
          ttfb_us: 1000,
          download_us: 500,
          total_us: 10_000,
          connection_reused: true,
          attempts: 1,
        },
      },
    });
    expect(screen.getByText('other')).toBeTruthy();
  });

  it('shows only TTFB + download with an info line for reused connections', () => {
    render(TimingTab, {
      props: {
        timing: {
          dns_us: 99, // present but must be hidden — connection was reused
          ttfb_us: 4000,
          download_us: 1000,
          total_us: 5000,
          connection_reused: true,
          attempts: 1,
        },
      },
    });
    expect(screen.getByText('connection reused — no DNS/connect/TLS phases')).toBeTruthy();
    expect(screen.queryByText('DNS lookup')).toBeNull();
    expect(screen.getByText('Waiting (TTFB)')).toBeTruthy();
    expect(screen.getByText('Content download')).toBeTruthy();
  });

  it('renders a single total bar + caption when timing is null', () => {
    render(TimingTab, { props: { timing: null, durationMs: 320 } });
    expect(screen.getByText('phase timing not available for this request type')).toBeTruthy();
    expect(screen.getByText('320 ms')).toBeTruthy();
  });

  it('renders the Attempts section with backoff chips and the final-attempt caption', () => {
    const retry: Retry = {
      count: 2,
      warnings: [],
      attempts: [
        { number: 1, status_code: 503, duration_ms: 120, delay_ms: 0, error: null },
        { number: 2, status_code: 0, duration_ms: 80, delay_ms: 200, error: 'connection reset' },
        { number: 3, status_code: 200, duration_ms: 95, delay_ms: 400, error: null },
      ],
    };
    render(TimingTab, { props: { timing: fullTiming, retry } });
    expect(screen.getByText('Attempts (3)')).toBeTruthy();
    expect(screen.getByText('#1')).toBeTruthy();
    expect(screen.getByText('503 Service Unavailable')).toBeTruthy();
    expect(screen.getByText('connection reset')).toBeTruthy();
    expect(screen.getByText('+200 ms backoff')).toBeTruthy();
    expect(screen.getByText('+400 ms backoff')).toBeTruthy();
    expect(screen.getByText('the waterfall above describes the final attempt')).toBeTruthy();
  });

  it('hides the Attempts section when there were no retries', () => {
    render(TimingTab, {
      props: { timing: fullTiming, retry: { count: 0, warnings: [], attempts: [] } },
    });
    expect(screen.queryByText(/Attempts/)).toBeNull();
  });
});

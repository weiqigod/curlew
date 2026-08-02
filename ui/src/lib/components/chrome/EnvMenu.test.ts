// EnvMenu component tests (§13.2): redaction — sensitive values render the
// Redacted chip in the variables flyout; the real value never exists client-side.
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { environments, selectedEnv } from '../../stores/environments';
import EnvMenu from './EnvMenu.svelte';

beforeEach(() => {
  environments.set([
    {
      name: 'dev',
      file: 'environments/dev.yaml',
      variables: [
        { name: 'base_url', value: 'http://localhost:3000', sensitive: false },
        { name: 'api_token', value: '[REDACTED]', sensitive: true },
      ],
    },
    {
      name: 'staging',
      file: 'environments/staging.yaml',
      variables: [],
    },
  ]);
  selectedEnv.set('dev');
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('EnvMenu', () => {
  it('lists environments with the base_url hint and the selection mark', async () => {
    render(EnvMenu);
    await fireEvent.click(screen.getByRole('button', { name: /env/ }));
    expect(screen.getByRole('menuitem', { name: /dev/ })).toBeTruthy();
    expect(screen.getByRole('menuitem', { name: /staging/ })).toBeTruthy();
    expect(screen.getByText('http://localhost:3000')).toBeTruthy();
    // env without base_url falls back to its file path
    expect(screen.getByText('environments/staging.yaml')).toBeTruthy();
  });

  it('renders the Redacted chip — never the value — in the variables flyout', async () => {
    vi.useFakeTimers();
    render(EnvMenu);
    await fireEvent.click(screen.getByRole('button', { name: /env/ }));
    const devRow = screen.getByRole('menuitem', { name: /dev/ });
    await fireEvent.mouseEnter(devRow);
    vi.advanceTimersByTime(320);
    await vi.runOnlyPendingTimersAsync();
    expect(screen.getByText('redacted')).toBeTruthy();
    expect(screen.getByText('api_token')).toBeTruthy();
    // the literal marker never renders as text
    expect(screen.queryByText('[REDACTED]')).toBeNull();
    // non-sensitive values render plainly (menu hint + flyout value)
    expect(screen.getAllByText('http://localhost:3000').length).toBeGreaterThan(0);
  });

  it('opens the flyout with ArrowRight for keyboard users', async () => {
    render(EnvMenu);
    await fireEvent.click(screen.getByRole('button', { name: /env/ }));
    const devRow = screen.getByRole('menuitem', { name: /dev/ });
    await fireEvent.keyDown(devRow, { key: 'ArrowRight' });
    expect(screen.getByText('redacted')).toBeTruthy();
  });

  it('selects an environment on click (session-scoped store)', async () => {
    render(EnvMenu);
    await fireEvent.click(screen.getByRole('button', { name: /env/ }));
    await fireEvent.click(screen.getByRole('menuitem', { name: /staging/ }));
    let current: string | null = null;
    selectedEnv.subscribe((v) => (current = v))();
    expect(current).toBe('staging');
  });
});

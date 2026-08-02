// JsonTree component tests (§13.2): expansion defaults, >20-array show-more,
// search force-expand, copy-path string body.$.data[3].amount.
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import JsonTree from './JsonTree.svelte';

const writeText = vi.fn().mockResolvedValue(undefined);

beforeEach(() => {
  writeText.mockClear();
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText },
    configurable: true,
  });
});

afterEach(cleanup);

function rows(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('[data-jtpath]')).map(
    (el) => el.getAttribute('data-jtpath') ?? '',
  );
}

describe('JsonTree', () => {
  it('auto-expands depth < 2 but collapses arrays with > 10 items', () => {
    const body = {
      user: { name: 'x', deep: { hidden: 1 } },
      big: Array.from({ length: 12 }, (_, i) => i),
      small: [1, 2],
    };
    const { container } = render(JsonTree, { props: { body, shortcuts: false } });
    const paths = rows(container);
    // depth-1 nodes visible and expanded
    expect(paths).toContain('$.user.name');
    // depth-2 object collapsed: its children invisible, preview shown
    expect(paths).toContain('$.user.deep');
    expect(paths).not.toContain('$.user.deep.hidden');
    expect(screen.getByText('{…} 1 keys')).toBeTruthy();
    // array > 10 items starts collapsed even at depth < 2
    expect(paths).not.toContain('$.big[0]');
    expect(screen.getByText('[…] 12 items')).toBeTruthy();
    // small array expanded
    expect(paths).toContain('$.small[0]');
  });

  it('shows the first 20 array items plus a show-more button', async () => {
    const body = { data: Array.from({ length: 25 }, (_, i) => i) };
    const { container } = render(JsonTree, { props: { body, shortcuts: false } });
    // 25 > 10 → collapsed; expand it via its preview
    await fireEvent.click(screen.getByText('[…] 25 items'));
    expect(rows(container)).toContain('$.data[19]');
    expect(rows(container)).not.toContain('$.data[20]');
    const more = screen.getByText('… show 5 more items');
    await fireEvent.click(more);
    expect(rows(container)).toContain('$.data[24]');
  });

  it('force-expands ancestors of search hits and counts matches', async () => {
    const body = { deep: { nested: { needle: 'findme' } }, other: 1 };
    const { container } = render(JsonTree, { props: { body, shortcuts: false } });
    expect(rows(container)).not.toContain('$.deep.nested.needle');
    const input = screen.getByLabelText('search body');
    await fireEvent.input(input, { target: { value: 'findme' } });
    expect(rows(container)).toContain('$.deep.nested.needle');
    expect(screen.getByText('1 match')).toBeTruthy();
    const hit = container.querySelector('[data-jtpath="$.deep.nested.needle"]');
    expect(hit?.classList.contains('jt-hit')).toBe(true);
  });

  it('copies body.$ + JSONPath from the per-row copy button', async () => {
    const body = { data: [0, 1, 2, { amount: 42 }] };
    const { container } = render(JsonTree, { props: { body, shortcuts: false } });
    // data[3] sits at depth 2 → collapsed by default; expand via its preview
    await fireEvent.click(screen.getByText('{…} 1 keys'));
    const row = container.querySelector('[data-jtpath="$.data[3].amount"]');
    expect(row).not.toBeNull();
    const btn = row?.querySelector('button[aria-label="copy path"]');
    await fireEvent.click(btn as Element);
    expect(writeText).toHaveBeenCalledWith('body.$.data[3].amount');
  });

  it('renders Redacted chips for "[REDACTED]" string leaves', () => {
    const body = { token: '[REDACTED]' };
    render(JsonTree, { props: { body, shortcuts: false } });
    expect(screen.getByText('redacted')).toBeTruthy();
    expect(screen.queryByText('"[REDACTED]"')).toBeNull();
  });
});

// Shared helpers for the curlew ui e2e suite. The global setup writes the
// server origin/token to STATE_FILE; tests read it synchronously.
import * as os from 'node:os';
import * as path from 'node:path';
import * as fs from 'node:fs';
import { expect, type Page } from '@playwright/test';

export const STATE_FILE = path.join(os.tmpdir(), 'curlew-ui-e2e-state.json');

export interface UiServer {
  /** http://127.0.0.1:<port> */
  origin: string;
  token: string;
  pid: number;
  /** fixture project dir (cwd of the server) */
  dir: string;
}

export interface E2EState {
  binDir: string;
  echoPort: number;
  refusedPort: number;
  server: UiServer;
}

let cached: E2EState | null = null;

export function readState(): E2EState {
  if (cached === null) {
    cached = JSON.parse(fs.readFileSync(STATE_FILE, 'utf8')) as E2EState;
  }
  return cached;
}

export function server(): UiServer {
  return readState().server;
}

/** Full app URL carrying the session token (stripped by the SPA on boot). */
export function appUrl(hash = '#/'): string {
  const s = server();
  return `${s.origin}/?token=${s.token}${hash}`;
}

/** Navigates to the app with the token and waits for the shell to boot. */
export async function gotoApp(page: Page, hash = '#/'): Promise<void> {
  await page.goto(appUrl(hash));
  // TopBar renders only after /meta + /tree + /environments resolve.
  await expect(page.getByRole('button', { name: 'run options' })).toBeVisible();
}

/** The primary segment of the Run split-button (Run all / Starting… / Cancel). */
export function runButton(page: Page) {
  return page.locator('button.seg-main');
}

/** Waits until no run is active (the primary segment reads "Run all"). */
export async function expectRunDone(page: Page, timeoutMs = 30_000): Promise<void> {
  await expect(runButton(page)).toHaveText(/Run all/, { timeout: timeoutMs });
}

/**
 * Runs one collection via the split-button caret menu ("Run current
 * collection" follows the #/tree/<path> route) and waits for the end.
 */
export async function runCollectionAndWait(page: Page, collectionPath: string): Promise<void> {
  await page.evaluate((p) => {
    window.location.hash = `#/tree/${encodeURIComponent(p)}`;
  }, collectionPath);
  await page.getByRole('button', { name: 'run options' }).click();
  await page.getByRole('menuitem', { name: 'Run current collection' }).click();
  await expect(runButton(page)).toHaveText(/Starting…|Cancel/);
  await expectRunDone(page);
}

/** A compact-list result row by request name. */
export function row(page: Page, name: string) {
  return page.locator('.rr').filter({ hasText: name });
}

/** Deletes every persisted run via the API — a clean slate for history tests. */
export async function deleteAllRuns(page: Page): Promise<void> {
  const { origin, token } = server();
  const auth = { Authorization: `Bearer ${token}` };
  const list = await page.request.get(`${origin}/api/v1/runs?limit=50&offset=0`, {
    headers: auth,
  });
  expect(list.ok()).toBe(true);
  const body = (await list.json()) as { runs: Array<{ run_id: string }> };
  for (const r of body.runs) {
    const res = await page.request.delete(`${origin}/api/v1/runs/${r.run_id}`, { headers: auth });
    expect(res.ok()).toBe(true);
  }
}

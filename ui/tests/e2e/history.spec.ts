// Run history + compare scenarios (UI_SPECIFICATION.md §13.3): persisted run
// history, compare diff (the echo /json body carries an incrementing counter,
// so two runs differ), identical-state panel, and delete.
//
// The suite shares one server, so the earlier specs leave persisted runs
// behind — the beforeAll wipes them via the API for deterministic counts.
import { test, expect, type Page } from '@playwright/test';
import { deleteAllRuns, gotoApp, runCollectionAndWait, server } from './helpers';

test.describe.configure({ mode: 'serial' });

let page: Page;

test.beforeAll(async ({ browser }) => {
  page = await browser.newPage();
  await deleteAllRuns(page);
});

test.afterAll(async () => {
  await page.close();
});

test('history rail populates after two runs', async () => {
  await gotoApp(page);
  await runCollectionAndWait(page, 'collections/basic.yaml');
  await runCollectionAndWait(page, 'collections/basic.yaml');

  await page.getByRole('button', { name: 'History' }).click();
  await expect(page).toHaveURL(/#\/compare/);
  const rows = page.locator('.runrow');
  await expect(rows).toHaveCount(2);
  // Newest run is B by default; counts per run: 6✓ 1✗ 1! 1–.
  await expect(rows.first()).toContainText('B');
  await expect(rows.first()).toContainText('6✓');
  await expect(rows.first()).toContainText('1✗');
  await expect(rows.first()).toContainText('1!');
  await expect(rows.first()).toContainText('1–');
});

test('compare diff renders changed lines between the two runs', async () => {
  // Pick the older run as A (the newest is already B).
  await page.locator('.runrow').nth(1).click();
  await expect(page).toHaveURL(/base=/);

  // Select the happy-path pair explicitly via the request picker.
  await page.locator('.pickrow button[aria-haspopup="menu"]').click();
  await page.getByRole('menuitem', { name: 'Get json ok' }).click();

  // Run header cards for A and B…
  await expect(page.locator('.card')).toHaveCount(2);
  // …and a real line diff: the counter differs between runs.
  const diff = page.locator('.diffbody');
  await expect(diff).toBeVisible();
  await expect(diff.locator('.dline.del', { hasText: '"counter"' })).toHaveCount(1);
  await expect(diff.locator('.dline.add', { hasText: '"counter"' })).toHaveCount(1);
  await expect(page.locator('.dvhead')).toContainText('keys changed');
});

test('identical bodies show the identical-state panel', async () => {
  // /slow returns a static body — identical across the two runs.
  await page.locator('.pickrow button[aria-haspopup="menu"]').click();
  await page.getByRole('menuitem', { name: 'Slow request' }).click();
  await expect(
    page.getByText('Response bodies are identical between these runs'),
  ).toBeVisible();
});

test('deleting a run removes it from the history rail', async () => {
  const { origin, token } = server();
  const auth = { Authorization: `Bearer ${token}` };

  // No delete affordance exists in the UI (§10.6.4 defines none) — exercise
  // the DELETE endpoint directly, then assert through the UI.
  const list = await page.request.get(`${origin}/api/v1/runs?limit=50&offset=0`, {
    headers: auth,
  });
  expect(list.ok()).toBe(true);
  const body = (await list.json()) as { runs: Array<{ run_id: string }>; total: number };
  expect(body.total).toBe(2);
  const res = await page.request.delete(`${origin}/api/v1/runs/${body.runs[0].run_id}`, {
    headers: auth,
  });
  expect(res.ok()).toBe(true);

  await gotoApp(page, '#/compare');
  await expect(page.locator('.runrow')).toHaveCount(1);
});

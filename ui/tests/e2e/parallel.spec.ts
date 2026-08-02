// Parallel-run scenarios (UI_SPECIFICATION.md §13.3): the parallel toggle,
// wave headers from a depends_on chain, and the columns layout.
import { test, expect, type Page } from '@playwright/test';
import { gotoApp, runCollectionAndWait } from './helpers';

test.describe.configure({ mode: 'serial' });

let page: Page;

test.beforeAll(async ({ browser }) => {
  page = await browser.newPage();
});

test.afterAll(async () => {
  await page.close();
});

test('parallel toggle is visible and toggles on', async () => {
  await gotoApp(page);
  const toggle = page.getByRole('button', { name: 'run requests in parallel waves' });
  await expect(toggle).toBeVisible();
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
});

test('parallel run groups rows under wave headers', async () => {
  await runCollectionAndWait(page, 'collections/parallel.yaml');
  // depends_on chain → wave 1 {alpha, beta}, wave 2 {gamma}.
  const headers = page.locator('.at-group-h');
  await expect(headers).toHaveCount(2);
  // The "·" separator is its own styled span, so match the normalized text.
  await expect(headers.nth(0)).toContainText(/Wave 1\s*·?\s*2 req/);
  await expect(headers.nth(1)).toContainText(/Wave 2\s*·?\s*1 req/);
  // Summary advertises the wave shape.
  await expect(page.locator('.strip')).toContainText('2 waves');
  await expect(page.locator('.strip')).toContainText('max 2∥');
});

test('columns layout is enabled for parallel runs and lays out per wave', async () => {
  const layout = page.getByRole('group', { name: 'run layout' });
  await expect(layout.getByRole('button', { name: 'columns' })).toBeEnabled();
  await layout.getByRole('button', { name: 'columns' }).click();

  const cols = page.locator('.col');
  await expect(cols).toHaveCount(2);
  await expect(cols.nth(0)).toContainText('Wave 1');
  await expect(cols.nth(0)).toContainText('Create alpha');
  await expect(cols.nth(0)).toContainText('Create beta');
  await expect(cols.nth(1)).toContainText('Wave 2');
  await expect(cols.nth(1)).toContainText('Join gamma');

  // Back to compact for cleanliness.
  await layout.getByRole('button', { name: 'compact' }).click();
  await expect(page.getByRole('listbox', { name: 'run results' })).toBeVisible();
});

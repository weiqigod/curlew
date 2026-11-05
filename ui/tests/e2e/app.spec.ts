// Core app scenarios (UI_SPECIFICATION.md §13.3) against the real binary:
// boot/token handling, batch "Run all", row outcomes, inspector panels,
// validation panel, env redaction, mid-run recovery, and keyboard driving.
//
// The whole file is serial on one shared page: the server keeps no "last run"
// across page loads (hello only carries the *active* run), so scenarios that
// inspect a finished run reuse the run produced by the earlier test instead
// of paying for a fresh run each.
import { test, expect, type Page } from '@playwright/test';
import { appUrl, gotoApp, row, runButton, expectRunDone, server } from './helpers';

test.describe.configure({ mode: 'serial' });

let page: Page;

test.beforeAll(async ({ browser }) => {
  page = await browser.newPage();
});

test.afterAll(async () => {
  await page.close();
});

test('boot strips the token from the URL into sessionStorage', async () => {
  await gotoApp(page);
  expect(page.url()).not.toContain('token=');
  const stored = await page.evaluate(() => sessionStorage.getItem('curlew.token'));
  expect(stored).toBe(server().token);
  // Shell chrome is up: project name + sidebar tree.
  await expect(page.getByText('e2e-fixture')).toBeVisible();
  await expect(page.locator('.crow', { hasText: 'basic.yaml' })).toBeVisible();
});

test('run all: batch across collections, live rows, summary reconciles', async () => {
  await runButton(page).click();
  // Live transition: the slow request's row shows the running spinner.
  await expect(
    row(page, 'Slow request').locator('[role="img"][aria-label="running"]'),
  ).toBeVisible({ timeout: 15_000 });
  await expectRunDone(page);
  // Batch run groups rows under one header per valid collection
  // (basic.yaml, parallel.yaml, retried.yaml — broken.yaml is excluded).
  // The setup/teardown phase headers share the class, so filter on ".yaml".
  const groups = page.locator('.at-group-h', { hasText: '.yaml' });
  await expect(groups).toHaveText(['basic.yaml', 'parallel.yaml', 'retried.yaml']);
  // basic.yaml: 6 passed / 1 failed / 1 error / 1 skipped (9 rows);
  // parallel.yaml: 3 passed; retried.yaml: 1 passed → 13 total, 10 passed.
  await expect(page.locator('span[aria-label="10 passed"]')).toBeVisible();
  await expect(page.locator('span[aria-label="1 failed"]')).toBeVisible();
  await expect(page.locator('span[aria-label="1 error"]')).toBeVisible();
  await expect(page.locator('span[aria-label="1 skipped"]')).toBeVisible();
  await expect(page.locator('.strip')).toContainText('13/13');
  await expect(page.locator('.strip')).toContainText('run finished');
  await expect(page.locator('.strip')).toContainText('env: dev');
});

test('failed row shows the assertion message; error row opens the Error tab', async () => {
  // Verbatim fail message in the row middle column.
  await expect(row(page, 'Failing assertion')).toContainText('expected nope, got yes');
  // Error row: category + first message line.
  await expect(row(page, 'Network error')).toContainText('network:');
  await expect(row(page, 'Network error')).toContainText('Connection refused');

  await row(page, 'Network error').click();
  await expect(page).toHaveURL(/#\/runs\/[0-9a-f]+\/requests\//);
  // Error tab is first and default for error outcomes.
  await expect(page.getByRole('tab', { name: 'Error' })).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('.cat')).toHaveText('network');
  await expect(page.locator('.msg')).toContainText('Connection refused');

  await page.getByRole('button', { name: 'back to run view' }).click();
  await expect(page.getByRole('listbox', { name: 'run results' })).toBeVisible();
});

test('skipped row carries the verbatim reason and opens the skip panel', async () => {
  await expect(row(page, 'Conditionally skipped')).toContainText('if: false');
  await row(page, 'Conditionally skipped').click();
  await expect(page.getByRole('tab', { name: 'Skipped' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect(page.locator('.skreason')).toHaveText('if: false');
  // Response tabs are disabled — the request never executed.
  await expect(page.getByRole('tab', { name: /Body/ })).toBeDisabled();
  await page.getByRole('button', { name: 'back to run view' }).click();
  await expect(page.getByRole('listbox', { name: 'run results' })).toBeVisible();
});

test('big body (>256 KiB) shows the load-on-demand panel, then the JSON tree', async () => {
  await row(page, 'Big body').click();
  await expect(page.getByRole('tab', { name: /Body/ })).toHaveAttribute('aria-selected', 'true');
  const loadBtn = page.getByRole('button', { name: /Load body/ });
  await expect(loadBtn).toBeVisible();
  await expect(loadBtn).toContainText(/29\d(\.\d+)? KB|0\.3 MB/); // ~297 KB
  await loadBtn.click();
  // Loaded + parsed as JSON → JsonTree with its body search input.
  await expect(page.getByLabel('search body')).toBeVisible();
  await page.getByRole('button', { name: 'back to run view' }).click();
  await expect(page.getByRole('listbox', { name: 'run results' })).toBeVisible();
});

test('binary response (image/png) shows the binary panel with preview', async () => {
  await row(page, 'Binary body').click();
  await expect(page.getByRole('tab', { name: /Body/ })).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('.bp')).toContainText('image/png');
  await expect(page.locator('.bp')).toContainText('binary content');
  await expect(page.locator('.bp').getByRole('button', { name: 'Download' })).toBeEnabled();
  await expect(page.locator('img[alt="response body preview"]')).toBeVisible();
  await page.getByRole('button', { name: 'back to run view' }).click();
  await expect(page.getByRole('listbox', { name: 'run results' })).toBeVisible();
});

test('retry badge appears for the retried request', async () => {
  // retried.yaml ran as part of the batch: 3 attempts → 2 retries → the ↻2
  // chip; the final 500 matched the assertion so the row passed.
  const r = row(page, 'Retried request');
  await expect(r).toBeVisible();
  await expect(r.getByLabel('2 retries — see Timing tab')).toHaveText('↻2');
  await expect(r).toContainText('500');
});

test('broken collection opens the validation panel with its issues', async () => {
  await page.locator('.crow', { hasText: 'broken.yaml' }).click();
  await expect(page).toHaveURL(/#\/file\/collections%2Fbroken\.yaml/);
  await expect(page.getByRole('heading', { name: 'Invalid collection file' })).toBeVisible();
  await expect(page.locator('.diag.err')).toContainText("missing required field 'url'");
  await page.getByRole('link', { name: 'back to run view' }).click();
});

test('env menu redacts the sensitive variable', async () => {
  await page.locator('button[title="environments/*.yaml"]').click();
  const devItem = page.getByRole('menuitem', { name: /dev/ });
  await expect(devItem).toBeVisible();
  await devItem.hover(); // flyout opens after the 300 ms hover delay
  const flyout = page.locator('.flyout');
  await expect(flyout).toBeVisible();
  await expect(flyout).toContainText('api_key');
  await expect(flyout.locator('.at-redacted')).toBeVisible();
  // The secret value never reaches the client.
  await expect(page.getByText('super-secret-value')).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(flyout).toBeHidden();
});

test('mid-run page reload recovers the run via replay', async () => {
  await runButton(page).click();
  await expect(runButton(page)).toHaveText(/Cancel/);
  await page.reload(); // token is gone from the URL; sessionStorage survives
  await expect(page.getByRole('button', { name: 'run options' })).toBeVisible();
  // The reloaded client adopts the active run from the hello frame and
  // replays events from 0 — the run completes with full counts.
  await expectRunDone(page);
  await expect(page.locator('span[aria-label="10 passed"]')).toBeVisible();
  await expect(page.locator('.strip')).toContainText('13/13');
});

test('keyboard smoke: r, j/k, Enter, tab digits, Esc, ?', async () => {
  await gotoApp(page);
  // r → run all.
  await page.keyboard.press('r');
  await expect(runButton(page)).toHaveText(/Starting…|Cancel/);
  await expectRunDone(page);

  // j twice, k once → cursor on the first row.
  await page.keyboard.press('j');
  await page.keyboard.press('j');
  await page.keyboard.press('k');
  const cursorRow = page.locator('.rr[aria-selected="true"]');
  await expect(cursorRow).toHaveCount(1);
  await expect(cursorRow).toContainText('Setup ping');

  // Enter → inspector for the cursor row (Body default for passed).
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/#\/runs\/[0-9a-f]+\/requests\//);
  await expect(page.getByRole('tab', { name: /Body/ })).toHaveAttribute('aria-selected', 'true');

  // Digit keys switch tabs (1-based across enabled tabs).
  await page.keyboard.press('2');
  await expect(page.getByRole('tab', { name: /Headers/ })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await page.keyboard.press('4');
  await expect(page.getByRole('tab', { name: /Timing/ })).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('1');
  await expect(page.getByRole('tab', { name: /Body/ })).toHaveAttribute('aria-selected', 'true');

  // Esc → back to the run view.
  await page.keyboard.press('Escape');
  await expect(page.getByRole('listbox', { name: 'run results' })).toBeVisible();

  // ? toggles the help overlay; Esc closes it.
  await page.keyboard.press('?');
  const help = page.getByRole('dialog', { name: 'keyboard shortcuts' });
  await expect(help).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(help).toBeHidden();
});

test('deep link with a token boots straight into the history view', async () => {
  // Cross-cutting: the token strip preserves the deep-link hash. The runs
  // recorded by the earlier tests populate the history rail.
  await page.goto(appUrl('#/compare'));
  await expect(page.locator('.runrow').first()).toBeVisible();
  expect(page.url()).not.toContain('token=');
});

test('request definition panel: deep link renders the raw template and can run one request', async () => {
  await page.goto(appUrl('#/def/collections%2Fbasic.yaml/get-json-ok'));
  await expect(page.getByRole('main').getByText('Get json ok')).toBeVisible();
  // Raw template, never resolved values.
  await expect(page.getByRole('main').locator('.turl')).toContainText('{{base_url}}');
  await expect(page.getByRole('main').getByText(/collections\/basic\.yaml:\d+/)).toBeVisible();
  const started = page.waitForResponse((response) =>
    response.request().method() === 'POST' && response.url().endsWith('/api/v1/runs'),
  );
  await page.getByRole('button', { name: /run this request/i }).click();
  const { run_id: runId } = await (await started).json() as { run_id: string };
  await expect(page.getByRole('button', { name: /run this request/i })).toBeEnabled();
  await expect(page).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/get-json-ok$/);
  await expect(page.getByRole('tab', { name: 'Body', exact: true })).toBeVisible();
  const results = await page.request.get(`${server().origin}/api/v1/runs/${runId}/requests`, {
    headers: { 'X-Curlew-UI-Token': server().token },
  });
  const body = await results.json() as { requests: Array<{ name: string; outcome: string }> };
  expect(body.requests.map((entry) => [entry.name, entry.outcome])).toEqual([
    ['Setup ping', 'passed'], ['Get json ok', 'passed'], ['Teardown ping', 'passed'],
  ]);
  const steps = page.getByRole('group', { name: 'Run steps' });
  await expect(steps.getByRole('button', { name: /setup.*Setup ping.*passed.*200/i })).toBeVisible();
  await steps.getByRole('button', { name: /setup.*Setup ping/i }).click();
  await expect(page.getByRole('main').locator('.response')).toContainText('Setup ping');
  await steps.getByRole('button', { name: /main.*Get json ok/i }).click();
  await expect(page.getByRole('main').locator('.response')).toContainText('Get json ok');
  await expect(page).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/get-json-ok$/);

  // Sidebar click on a request with NO result in the focused run → definition
  // panel (the fallback that replaced the invisible footer hint).
  await page.locator('.rrow').filter({ hasText: 'Create alpha' }).click();
  await expect(page.getByRole('main').getByText('Create alpha')).toBeVisible();
  await expect(page.getByRole('main').locator('.turl')).toContainText('{{base_url}}');

  // Completed requests stay in the same workspace with inline results.
  await page.locator('.rrow').filter({ hasText: 'Get json ok' }).click();
  await expect(page.getByRole('tab', { name: 'Body' })).toBeVisible();
  await expect(page).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/get-json-ok$/);
  await expect(page.getByRole('button', { name: /run this request/i })).toBeEnabled();
});

test('setup runs independently from panel toolbar and keyboard without navigation', async ({ page: definitionPage }, testInfo) => {
  for (const viewport of [{ width: 1280, height: 800 }, { width: 960, height: 844 }]) {
    await definitionPage.setViewportSize(viewport);
    await gotoApp(definitionPage, '#/def/collections%2Fbasic.yaml/setup-ping');
    const runRequest = definitionPage.getByRole('main').getByRole('button', { name: /run this request/i });
    await expect(runRequest).toBeEnabled();
    await expect(runButton(definitionPage)).toBeEnabled();
    for (const action of ['request', 'toolbar', 'keyboard']) {
      const started = definitionPage.waitForResponse((response) =>
        response.request().method() === 'POST' && response.url().endsWith('/api/v1/runs'),
      );
      if (action === 'request') await runRequest.click();
      else if (action === 'toolbar') await runButton(definitionPage).click();
      else await definitionPage.keyboard.press('r');
      const response = await started;
      expect(response.request().postDataJSON()).toMatchObject({
        mode: 'setup', collection: 'collections/basic.yaml', selection: ['Setup ping'], parallel: false,
      });
      const { run_id: runId } = await response.json() as { run_id: string };
      await expect(runRequest).toBeEnabled();
      await expect(definitionPage.getByRole('group', { name: 'Run steps' })).toContainText('passed');
      await expect(definitionPage.getByRole('tab', { name: 'Body', exact: true })).toBeVisible();
      await expect(definitionPage).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/setup-ping$/);
      const results = await definitionPage.request.get(`${server().origin}/api/v1/runs/${runId}/requests`, {
        headers: { 'X-Curlew-UI-Token': server().token },
      });
      const body = await results.json() as { requests: Array<{ name: string; phase: string; outcome: string }> };
      expect(body.requests.map((entry) => [entry.name, entry.phase, entry.outcome])).toEqual([
        ['Setup ping', 'setup', 'passed'],
      ]);
    }
    await definitionPage.screenshot({ path: testInfo.outputPath(`setup-${viewport.width}.png`), fullPage: true });
    await definitionPage.goto(appUrl('#/def/collections%2Fbasic.yaml/teardown-ping'));
    await expect(definitionPage.getByRole('button', { name: /run this request/i })).toHaveCount(0);
    await expect(runButton(definitionPage)).toBeDisabled();
  }
  await definitionPage.setViewportSize({ width: 390, height: 844 });
  await expect(definitionPage.getByText('curlew ui needs more room')).toBeVisible();
  await definitionPage.screenshot({ path: testInfo.outputPath('minimum-width-390.png'), fullPage: true });
});

test('manual request workspace shows network errors and cancellation in place', async ({ page: requestPage }, testInfo) => {
  await gotoApp(requestPage, '#/def/collections%2Fbasic.yaml/network-error');
  const runRequest = requestPage.getByRole('main').getByRole('button', { name: /run this request/i });
  await runRequest.click();
  await expect(requestPage.getByRole('tab', { name: 'Error', exact: true })).toBeVisible();
  await expect(runRequest).toBeEnabled();
  await expect(requestPage.locator('.msg')).toContainText('Connection refused');
  await expect(requestPage).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/network-error$/);
  await requestPage.screenshot({ path: testInfo.outputPath('inline-network-error.png'), fullPage: true });

  await requestPage.locator('.rrow').filter({ hasText: 'Slow request' }).click();
  await runRequest.click();
  await expect(runRequest).toBeDisabled();
  await requestPage.getByRole('main').getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(runRequest).toBeEnabled();
  await expect(requestPage).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/slow-request$/);
  await runRequest.click();
  await expect(requestPage.getByRole('tab', { name: 'Body', exact: true })).toBeVisible();
  await expect(runRequest).toBeEnabled();
  await expect(requestPage.getByRole('main').locator('.response')).toContainText('slow');
  await expect(requestPage).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/slow-request$/);
});

test('manual request workspace keeps repeated runs and results in place', async ({ page: requestPage }, testInfo) => {
  requestPage.on('pageerror', (error) => {
    throw error;
  });
  for (const width of [1280, 960]) {
    await requestPage.setViewportSize({ width, height: 900 });
    await gotoApp(requestPage, '#/def/collections%2Fbasic.yaml/failing-assertion');
    const runRequest = requestPage.getByRole('main').getByRole('button', { name: /run this request/i });
    const runIds: string[] = [];
    for (const action of ['request', 'toolbar', 'keyboard']) {
      const started = requestPage.waitForResponse((response) =>
        response.request().method() === 'POST' && response.url().endsWith('/api/v1/runs'),
      );
      if (action === 'request') await runRequest.click();
      else if (action === 'toolbar') await runButton(requestPage).click();
      else await requestPage.keyboard.press('r');
      const response = await started;
      expect(response.request().postDataJSON()).toMatchObject({
        collection: 'collections/basic.yaml', mode: 'selection',
        selection: ['Failing assertion'], env: 'dev',
      });
      const { run_id: runId } = await response.json() as { run_id: string };
      expect(runIds).not.toContain(runId);
      runIds.push(runId);
      await expect(runRequest).toBeEnabled();
      await expect(requestPage).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/failing-assertion$/);
      await requestPage.getByRole('tab', { name: /Assertions/ }).click();
      await expect(requestPage.getByRole('main')).toContainText('nope');
      await expect(requestPage.getByRole('main')).toContainText('yes');
      await requestPage.getByRole('tab', { name: 'Request', exact: true }).click();
      await expect(requestPage).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/failing-assertion$/);
      const results = await requestPage.request.get(`${server().origin}/api/v1/runs/${runId}/requests`, {
        headers: { 'X-Curlew-UI-Token': server().token },
      });
      const body = await results.json() as { requests: Array<{ name: string; phase: string }> };
      expect(body.requests.map((entry) => entry.name)).toEqual(['Setup ping', 'Failing assertion', 'Teardown ping']);
    }
    await requestPage.locator('.rrow').filter({ hasText: 'Get json ok' }).click();
    await requestPage.locator('.rrow').filter({ hasText: 'Failing assertion' }).click();
    await expect(requestPage).toHaveURL(/#\/def\/collections%2Fbasic.yaml\/failing-assertion$/);
    await expect(runRequest).toBeEnabled();
    await expect(requestPage.getByRole('tab', { name: /Assertions/ })).toBeVisible();
    await requestPage.screenshot({ path: testInfo.outputPath(`request-workspace-${width}.png`), fullPage: true });
  }
});

const { test, expect } = require('./fixtures');

test('opens the app with the expected title and sidebar', async ({ window }) => {
  await expect(window).toHaveTitle('Fistbump');
  await expect(window.locator('#nav-home')).toBeVisible();
});

test('the Go backend answers through the bridge API', async ({ window }) => {
  const info = await window.evaluate(() => window.api.app.info());
  expect(info.version).toMatch(/^\d+\.\d+\.\d+/);
  const connectors = await window.evaluate(() => window.api.connectors.list());
  expect(connectors.error).toBeUndefined();
  const resume = await window.evaluate(() => window.api.resume.get());
  expect(resume.error).toBeUndefined();
});

test('uses a fresh data folder, not the real one', async ({ window }) => {
  const info = await window.evaluate(() => window.api.app.info());
  expect(info.dataDir).toContain('fistbump-e2e-');
});

test('the renderer is isolated from Node', async ({ window }) => {
  const exposed = await window.evaluate(() => ({
    require: typeof require,
    process: typeof process,
    module: typeof module,
    api: typeof window.api,
  }));
  expect(exposed).toEqual({ require: 'undefined', process: 'undefined', module: 'undefined', api: 'object' });
});

test('the page has a Content-Security-Policy that blocks remote scripts', async ({ window }) => {
  const csp = await window.locator('meta[http-equiv="Content-Security-Policy"]').getAttribute('content');
  expect(csp).toContain("default-src 'self'");
  expect(csp).not.toContain('unsafe-eval');
  expect(csp).not.toMatch(/script-src[^;]*unsafe-inline/);
});

test('starts without console errors or page errors', async ({ window }) => {
  const problems = [];
  window.on('pageerror', (e) => problems.push(String(e)));
  window.on('console', (m) => m.type() === 'error' && problems.push(m.text()));
  await window.reload();
  await window.locator('#nav-home').waitFor();
  expect(problems).toEqual([]);
});

test('navigates between the main views', async ({ window }) => {
  await window.locator('#tab-settings').click();
  await expect(window.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible();
  await expect(window.locator('#settings-pane')).toBeVisible();

  await window.locator('#tab-paste').click();
  await expect(window.getByRole('heading', { name: 'Paste a job', level: 1 })).toBeVisible();

  await window.locator('#nav-resume').click();
  await expect(window.locator('#settings-pane')).toBeHidden();

  await window.locator('#tab-find').click();
  await expect(window.locator('#search-form')).toBeVisible();
});

test('master resume saves profile and renders PDF preview without stuck overlay on unmount', async ({ window }) => {
  await window.locator('#nav-resume').click();
  const nameInput = window.locator('input[id$="-full_name"]');
  await nameInput.fill('Ada Lovelace');
  await window.getByRole('button', { name: /Create profile|Save profile/ }).click();
  await expect(window.locator('.resume-doc')).toContainText('Ada Lovelace');

  await window.locator('#resume-tab-pdf').click();
  const preview = window.locator('.pdf-preview');
  await expect(preview).toBeVisible();
  await preview.locator('.pdf-expand').click();
  await expect(window.locator('.pdf-preview.expanded')).toBeVisible();
  await window.keyboard.press('Escape');
  await expect(window.locator('.pdf-preview.expanded')).toHaveCount(0);

  // Expanding again and switching routes programmatically must dispose the overlay cleanly.
  await preview.locator('.pdf-expand').click();
  await expect(window.locator('.pdf-preview.expanded')).toBeVisible();
  await window.evaluate(() => window.App.go('home'));
  await expect(window.locator('.pdf-preview.expanded')).toHaveCount(0);
  await expect(window.locator('.pdf-backdrop')).toHaveCount(0);
});

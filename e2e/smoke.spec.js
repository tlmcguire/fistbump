const { test, expect } = require('./fixtures');

test('opens one window with the app title', async ({ app, window }) => {
  await expect(window).toHaveTitle('Fistbump');
  expect(app.windows()).toHaveLength(1);
  await expect(window.locator('#nav-home')).toBeVisible();
});

test('the Go backend answers through the preload API', async ({ window }) => {
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

test('web preferences are locked down', async ({ app, window }) => {
  const prefs = await app.evaluate(({ BrowserWindow }) => {
    const wp = BrowserWindow.getAllWindows()[0].webContents.getLastWebPreferences();
    return { contextIsolation: wp.contextIsolation, nodeIntegration: wp.nodeIntegration, sandbox: wp.sandbox };
  });
  expect(prefs).toEqual({ contextIsolation: true, nodeIntegration: false, sandbox: true });
});

test('target=_blank links and window.open never create a window', async ({ app, window }) => {
  // http: links are ignored by the app. An https: link would start the system's browser, which hangs on a CI runner.
  await window.evaluate(() => window.open('http://example.invalid/', '_blank'));
  await window.waitForTimeout(500);
  expect(app.windows()).toHaveLength(1);
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

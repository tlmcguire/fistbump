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

test('open link page is merged into search bar and detects pasted Greenhouse URLs', async ({ window }) => {
  await window.locator('#tab-find').click();
  await expect(window.locator('#tab-link')).toHaveCount(0);
  await expect(window.locator('#job-form')).toHaveCount(0);

  const searchInput = window.locator('#search-keywords');
  const submitBtn = window.locator('#search-form button[type="submit"]');
  await expect(submitBtn).toHaveText('Search');

  await searchInput.fill('https://boards.greenhouse.io/acme/jobs/12345');
  await expect(submitBtn).toHaveText('Open link');

  await searchInput.fill('data analyst');
  await expect(submitBtn).toHaveText('Search');
});

test('experience supports date range not applicable and resume preview supports PDF styles', async ({ window }) => {
  await window.locator('#nav-resume').click();
  const nameInput = window.locator('input[id$="-full_name"]');
  await nameInput.fill('Grace Hopper');
  await window.getByRole('button', { name: /Create profile|Save profile/ }).click();
  await expect(window.locator('.resume-doc')).toContainText('Grace Hopper');

  // Switch PDF styles in preview
  const styleSelect = window.locator('#resume-pdf-style');
  await expect(styleSelect).toBeVisible();
  await styleSelect.selectOption('modern');
  await expect(styleSelect).toHaveValue('modern');
  await styleSelect.selectOption('compact');
  await expect(styleSelect).toHaveValue('compact');

  // Add an experience with Date range not applicable checked
  await window.getByRole('button', { name: 'Add experience' }).click();
  await window.locator('input[id$="-job_title"]').fill('Open Source Maintainer');
  await window.locator('input[id$="-company_name"]').fill('Community Project');
  await window.locator('input[id$="-date_na"]').check();
  await window.locator('.entry-form button[type="submit"]').click();

  await expect(window.locator('.resume-doc')).toContainText('Open Source Maintainer');
  await expect(window.locator('.resume-doc')).not.toContainText('to Present');
});

test('cost analysis returns cited sources and 30-day refresh metadata', async ({ window }) => {
  const result = await window.evaluate(() => window.api.cost.analyze({
    title: 'Software Engineer',
    location: 'Austin, TX',
    work_mode: 'Hybrid',
    pay_min: 130000,
    pay_max: 150000,
    text: 'We offer health insurance, dental, 401(k) match, and paid time off.',
  }));
  expect(result.supported).toBe(true);
  expect(result.refresh_days).toBe(30);
  expect(result.data_updated_at).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  expect(result.next_update_at).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  expect(Array.isArray(result.sources)).toBe(true);
  expect(result.sources.length).toBeGreaterThanOrEqual(5);
});

test('themes and accessibility preferences apply cleanly in Settings', async ({ window }) => {
  await window.locator('#tab-settings').click();
  const themeSelect = window.locator('#pref-theme');
  await expect(themeSelect).toBeVisible();

  for (const theme of ['bubblegum', 'cyber', 'retro', 'december']) {
    await themeSelect.selectOption(theme);
    await expect(window.locator('html')).toHaveAttribute('data-theme', theme);
  }
  await themeSelect.selectOption('auto-season');
  await expect(window.locator('html')).toHaveAttribute('data-theme', /^(january|february|march|april|may|june|july|august|september|october|november|december)$/);

  await window.locator('#pref-high-contrast').check();
  await expect(window.locator('html')).toHaveClass(/high-contrast/);
  await window.locator('#pref-underline-links').check();
  await expect(window.locator('html')).toHaveClass(/underline-links/);

  await themeSelect.selectOption('classic');
  await expect(window.locator('html')).not.toHaveAttribute('data-theme', /.+/);
});

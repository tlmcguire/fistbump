const { defineConfig } = require('@playwright/test');

// Smoke tests that run the renderer in Playwright Chromium against the real Go backend. Build the backend first:
//   npm run build:go && npm run test:e2e
module.exports = defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  workers: 1, // one window and one backend at a time
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: { trace: 'retain-on-failure', screenshot: 'only-on-failure' },
});

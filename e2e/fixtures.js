const { test: base, expect, _electron: electron } = require('@playwright/test');
const fs = require('fs');
const os = require('os');
const path = require('path');

// Each test gets a fresh copy of the app with its own data folder, so nothing touches real data.
const test = base.extend({
  app: async ({}, use) => {
    const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fistbump-e2e-'));
    const app = await electron.launch({
      args: [path.join(__dirname, '..'), ...(process.platform === 'linux' ? ['--no-sandbox'] : [])],
      env: { ...process.env, FISTBUMP_USER_DATA: dataDir },
    });
    await use(app);
    await app.close();
    fs.rmSync(dataDir, { recursive: true, force: true });
  },
  window: async ({ app }, use) => {
    const window = await app.firstWindow();
    await window.waitForLoadState('domcontentloaded');
    await use(window);
  },
});

module.exports = { test, expect };

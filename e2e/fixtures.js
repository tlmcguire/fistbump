const { test: base, expect } = require('@playwright/test');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { startHarness } = require('./harness');

// Each test gets a fresh fistbump-core backend with its own temporary data folder.
const test = base.extend({
  harness: async ({}, use) => {
    const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fistbump-e2e-'));
    const harness = await startHarness(dataDir);
    await use(harness);
    await harness.close();
    fs.rmSync(dataDir, { recursive: true, force: true });
  },
  window: async ({ page, harness }, use) => {
    await page.goto(harness.url, { waitUntil: 'domcontentloaded' });
    await use(page);
  },
});

module.exports = { test, expect };

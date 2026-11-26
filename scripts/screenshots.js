#!/usr/bin/env node
// Takes the README screenshots: launches fistbump-core on a throwaway data folder, fills it with the
// sample resume and a few jobs, and saves a picture of each view to docs/images/.
//   npm run build:go && node scripts/screenshots.js
const { chromium } = require('@playwright/test');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { startHarness } = require('../e2e/harness');

const root = path.join(__dirname, '..');
const out = path.join(root, 'docs', 'images');
const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fistbump-shots-'));
const postings = ['data-analyst', 'backend-engineer', 'financial-analyst', 'data-scientist'];

(async () => {
  // First start creates the SQLite schema, then the sample resume goes in.
  let harness = await startHarness(dataDir);
  await harness.close();
  const db = fs.readdirSync(dataDir, { recursive: true }).find((f) => String(f).endsWith('fistbump.db'));
  execFileSync('sqlite3', [path.join(dataDir, db)], {
    input: fs.readFileSync(path.join(root, 'backend/testdata/seed.sql')),
  });

  harness = await startHarness(dataDir);
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  await page.goto(harness.url, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('#nav-home');

  const shot = async (name) => {
    await page.waitForTimeout(900);
    await page.screenshot({ path: path.join(out, `${name}.png`) });
    console.log('saved', name);
  };
  const click = async (sel) => { await page.click(sel); };

  const ids = [];
  for (const name of postings) {
    const text = fs.readFileSync(path.join(root, 'backend/testdata/postings', `${name}.txt`), 'utf8');
    const job = await page.evaluate((raw) => window.api.jobs.create({ source: 'pasted', raw_text: raw }), text);
    ids.push(job.id ?? job.data?.id);
  }
  await page.evaluate((id) => window.api.applications.create({ job_id: id, status: 'Applied', date_applied: '2026-11-10' }), ids[1]);
  await page.evaluate((id) => window.api.applications.create({ job_id: id, status: 'Interviewing', date_applied: '2026-11-03', next_step_date: '2026-11-28' }), ids[2]);
  const rev = await page.evaluate((id) => window.api.revisions.create({ job_id: id, engine: 'auto', language: 'en' }), ids[0]);
  const revId = rev.id ?? rev.data?.id;
  for (let i = 0; i < 40; i++) {
    const r = await page.evaluate((id) => window.api.revisions.get(id), revId);
    if (['done', 'failed'].includes((r.data ?? r).status)) break;
    await page.waitForTimeout(500);
  }
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForSelector('#nav-home');

  await page.waitForTimeout(2500); //Home fills its matches panel after the rest
  await shot('home');
  await click('#nav-resume'); await shot('resume');
  await click('#nav-jobs'); await shot('jobs');
  await page.locator('.jl3-open', { hasText: 'Data Analyst' }).first().click();
  await shot('job');
  await page.evaluate((id) => window.Tailor.open(id), ids[0]);
  await shot('tailor');
  await page.evaluate(() => window.App.go('tracker')); await shot('tracker');
  await click('#tab-settings'); await shot('settings');

  await browser.close();
  await harness.close();
  fs.rmSync(dataDir, { recursive: true, force: true });
})().catch((e) => { console.error(e); process.exit(1); });

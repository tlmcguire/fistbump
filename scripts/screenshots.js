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
    await page.mouse.move(2, 2); //no hover state in the pictures
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

  //plans on the calendar this week, relative to today so the week view always has something on it
  await page.evaluate(async (jobId) => {
    const pad = (n) => String(n).padStart(2, '0');
    const iso = (d) => {
      const o = -d.getTimezoneOffset();
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:00${o >= 0 ? '+' : '-'}${pad(Math.floor(Math.abs(o) / 60))}:${pad(Math.abs(o) % 60)}`;
    };
    const at = (days, h, m = 0) => { const d = new Date(); d.setDate(d.getDate() + days); d.setHours(h, m, 0, 0); return d; };
    const plan = (kind, title, days, h, minutes, extra = {}) => window.api.events.create({ kind, title, starts_at: iso(at(days, h)), ends_at: iso(new Date(at(days, h).getTime() + minutes * 60000)), ...extra });
    await plan('interview', 'Interview with Cobalt Rivers', 1, 10, 60, { job_id: jobId, location: 'Video call' });
    await plan('call', 'Recruiter call, Lumen Fields', 2, 14, 30);
    await plan('interview', 'Onsite, Oakline Software', 3, 9, 180);
    await plan('follow_up', 'Thank-you notes', 3, 16, 30);
  }, ids[2]);

  //the pictures are of where things settle, not of things arriving
  await page.evaluate(() => { window.Prefs.set('reduceMotion', 'on'); window.Prefs.set('barPosition', 'bottom'); window.Prefs.set('mode', 'night'); });
  const go = async (route) => { await page.evaluate((r) => window.App.go(r), route); };

  //Home fills its matches panel after the rest (a live search), so wait for it to settle
  await page.waitForFunction(() => !document.body.innerText.includes('Looking for roles that match'), null, { timeout: 30000 }).catch(() => {});
  await shot('home');
  await go('resume'); await shot('resume');
  await go('list'); await shot('jobs');
  await page.evaluate((id) => window.JobPage.open(id), ids[2]); await shot('job');
  await go('tailor');
  await page.locator('.stage-job', { hasText: 'Data Analyst' }).click();
  await shot('tailor');
  await go('tracker'); await shot('tracker');
  await go('calendar'); await shot('calendar');
  await page.evaluate(() => { window.App.go('jobs'); window.App.openTab('tab-settings'); });
  await shot('settings');

  //the same Home by day and in Darker
  await go('home');
  await page.waitForFunction(() => !document.body.innerText.includes('Looking for roles that match'), null, { timeout: 30000 }).catch(() => {});
  const settled = () => page.waitForFunction(() => !document.body.innerText.includes('Looking for roles that match'), null, { timeout: 30000 }).catch(() => {});
  await page.evaluate(() => window.Prefs.set('mode', 'day'));
  await page.waitForTimeout(300); await settled();
  await shot('home-day');
  await page.evaluate(() => window.Prefs.set('mode', 'darker'));
  await page.waitForTimeout(300); await settled();
  await shot('home-darker');
  await page.evaluate(() => window.Prefs.set('mode', 'night'));

  await browser.close();
  await harness.close();
  fs.rmSync(dataDir, { recursive: true, force: true });
})().catch((e) => { console.error(e); process.exit(1); });

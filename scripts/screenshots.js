#!/usr/bin/env node
// Takes the README screenshots: launches the app on a throwaway data folder, fills it with the sample
// resume and a few jobs, and saves a picture of each view to docs/images/.
//   npm run build:go && node scripts/screenshots.js
const { _electron: electron } = require('@playwright/test');
const { execFileSync } = require('child_process');
const fs = require('fs');
const os = require('os');
const path = require('path');

const root = path.join(__dirname, '..');
const out = path.join(root, 'docs', 'images');
const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fistbump-shots-'));
const postings = ['data-analyst', 'backend-engineer', 'financial-analyst', 'data-scientist'];
const launch = () => electron.launch({
  args: [root, ...(process.platform === 'linux' ? ['--no-sandbox'] : [])],
  env: { ...process.env, FISTBUMP_USER_DATA: dataDir },
});

(async () => {
  // the first start creates the database, then the sample resume goes in
  let app = await launch();
  await (await app.firstWindow()).waitForLoadState('domcontentloaded');
  await app.close();
  const db = fs.readdirSync(dataDir, { recursive: true }).find(f => String(f).endsWith('fistbump.db'));
  execFileSync('sqlite3', [path.join(dataDir, db)], { input: fs.readFileSync(path.join(root, 'backend/testdata/seed.sql')) });

  app = await launch();
  const win = await app.firstWindow();
  await win.setViewportSize({ width: 1280, height: 800 });
  await win.waitForLoadState('domcontentloaded');
  await win.waitForSelector('#nav-home');
  const shot = async name => {
    await win.waitForTimeout(900);
    await win.screenshot({ path: path.join(out, `${name}.png`) });
    console.log('saved', name);
  };
  const click = async sel => { await win.click(sel); };

  const ids = [];
  for (const name of postings) {
    const text = fs.readFileSync(path.join(root, 'backend/testdata/postings', `${name}.txt`), 'utf8');
    const job = await win.evaluate(raw => window.api.jobs.create({ source: 'pasted', raw_text: raw }), text);
    ids.push(job.id ?? job.data?.id);
  }
  await win.evaluate(id => window.api.applications.create({ job_id: id, status: 'Applied', date_applied: '2026-11-10' }), ids[1]);
  await win.evaluate(id => window.api.applications.create({ job_id: id, status: 'Interviewing', date_applied: '2026-11-03', next_step_date: '2026-11-28' }), ids[2]);
  // a round of suggestions for the first job, written by the built-in rules
  const rev = await win.evaluate(id => window.api.revisions.create({ job_id: id, engine: 'auto', language: 'en' }), ids[0]);
  const revId = rev.id ?? rev.data?.id;
  for (let i = 0; i < 40; i++) {
    const r = await win.evaluate(id => window.api.revisions.get(id), revId);
    if (['done', 'failed'].includes((r.data ?? r).status)) break;
    await win.waitForTimeout(500);
  }
  await win.reload();
  await win.waitForSelector('#nav-home');

  await shot('home');
  await click('#nav-resume'); await shot('resume');
  await click('#tab-saved');
  await win.locator('text=Data Analyst').first().click();
  await shot('jobs');
  await win.locator('.stage-job', { hasText: 'Data Analyst' }).click();
  await shot('tailor');
  await click('#nav-tracker'); await shot('tracker');
  await click('#tab-settings'); await shot('settings');
  await app.close();
  fs.rmSync(dataDir, { recursive: true, force: true });
})().catch(e => { console.error(e); process.exit(1); });

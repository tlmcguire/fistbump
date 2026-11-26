#!/usr/bin/env node
// Captures 1280x800 screenshots of every new theme on the Home view into docs/images/themes/.
// Usage:
//   npm run build:go && node scripts/theme-screenshots.js          # uses live fistbump-core backend
//   node scripts/theme-screenshots.js --mock                       # offline render with sample data
const { chromium } = require('@playwright/test');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { startHarness } = require('../e2e/harness');

const root = path.join(__dirname, '..');
const out = path.join(root, 'docs', 'images', 'themes');
fs.mkdirSync(out, { recursive: true });

const STYLE_THEMES = [
  'bubblegum', 'cyber', 'techno', 'retro', 'vaporwave',
  'dracula', 'matcha', 'midnight-gold', 'ocean-abyss', 'arcade',
];
const SEASONAL_THEMES = [
  'january', 'february', 'march', 'april', 'may', 'june',
  'july', 'august', 'september', 'october', 'november', 'december',
];
const ALL_THEMES = [...STYLE_THEMES, ...SEASONAL_THEMES];

const useMock = process.argv.includes('--mock');

const installMockApi = async (page) => {
  const today = new Date().toISOString().slice(0, 10);
  const now = new Date().toISOString();
  await page.addInitScript(({ todayStr, nowStr }) => {
    const sampleResume = {
      profile: {
        full_name: 'Alex Rivera',
        email: 'alex.rivera@example.com',
        phone: '(555) 234-5678',
        home_location: 'Austin, TX',
        preferred_mode: 'Hybrid',
        target_positions: ['Software Engineer', 'Data Analyst'],
        summary: 'Product-minded software engineer building local-first developer tools and analytics pipelines.',
        updated_at: nowStr,
      },
      experiences: [
        { id: 1, job_title: 'Software Engineer', company_name: 'Northstar Labs', impact: 'Cut p95 API latency by 42% across 3 core services.' },
        { id: 2, job_title: 'Data Analyst', company_name: 'Harbor Metrics', impact: '' },
      ],
      education: [{ id: 1, school: 'State University', degree: 'B.S. Computer Science' }],
    };
    const sampleVersions = [
      { id: 1, version_label: 'Backend Focus', target_position: 'Software Engineer', created_at: nowStr },
      { id: 2, version_label: 'Analytics Focus', target_position: 'Data Analyst', created_at: nowStr },
    ];
    const sampleJobs = [
      { id: 101, company_name: 'Linear', position_title: 'Senior Product Engineer', location: 'Remote · US', work_mode: 'Remote', board: 'linear', external_id: '101' },
      { id: 102, company_name: 'Stripe', position_title: 'Software Engineer, Core Systems', location: 'Austin, TX', work_mode: 'Hybrid', board: 'stripe', external_id: '102' },
      { id: 103, company_name: 'Figma', position_title: 'Data Engineer', location: 'San Francisco, CA', work_mode: 'Hybrid', board: 'figma', external_id: '103' },
    ];
    const sampleApps = [
      { id: 201, job_id: 103, company_name: 'Figma', position_title: 'Data Engineer', status: 'Interviewing', next_step_date: todayStr },
    ];
    const sampleSuggest = {
      boards_searched: 12,
      postings: [
        { company_name: 'Vercel', position_title: 'Frontend Platform Engineer', location: 'Remote', work_mode: 'Remote', pay_min: 155000, pay_max: 185000, board: 'vercel', external_id: '501', fit: { percent: 92 } },
        { company_name: 'Datadog', position_title: 'Software Engineer, Telemetry', location: 'Austin, TX', work_mode: 'Hybrid', pay_min: 145000, pay_max: 175000, board: 'datadog', external_id: '502', fit: { percent: 84 } },
        { company_name: 'Notion', position_title: 'Product Engineer', location: 'New York, NY', work_mode: 'Hybrid', pay_min: 160000, pay_max: 195000, board: 'notion', external_id: '503', fit: { percent: 78 } },
      ],
    };

    localStorage.setItem('fistbump.search-history', JSON.stringify([
      { fields: { 'search-keywords': 'Software Engineer', 'search-location': 'Remote', 'search-work-mode': 'Remote' }, industry: 'Technology', at: nowStr },
      { fields: { 'search-keywords': 'Data Analyst', 'search-location': 'Austin, TX', 'search-work-mode': 'Hybrid' }, industry: '', at: nowStr },
    ]));
    localStorage.removeItem('fistbump.last-listing');

    window.api = {
      platform: 'darwin',
      setZoom: () => {},
      onBackendRestarted: () => {},
      app: {
        info: async () => ({ version: '0.5.0', platform: 'darwin', arch: 'arm64', tauri: '2.5.0' }),
        checkForUpdates: async () => ({ status: 'current', message: 'Up to date.' }),
        onOpenSettings: () => {},
        onUpdateProgress: () => {},
      },
      resume: { get: async () => sampleResume },
      resumes: { list: async () => sampleVersions },
      jobs: {
        list: async () => sampleJobs,
      },
      applications: { list: async () => sampleApps },
      revisions: { list: async () => [] },
      connectors: {
        list: async () => [],
        suggest: async () => sampleSuggest,
        categories: async () => ({ categories: [], companies: [], board_total: 0 }),
      },
      settings: { get: async () => ({}), put: async () => ({}) },
      storage: { report: async () => ({ cache_bytes: 0, db_bytes: 65536, total_bytes: 65536, data_dir: '/tmp' }) },
      ai: { status: async () => ({ mode: 'rules', local: { available: false, installed: false, running: false }, remote: { configured: false } }) },
      models: { list: async () => [] },
      savedSearches: { list: async () => [] },
    };
  }, { todayStr: today, nowStr: now });
};

(async () => {
  const launchOpts = process.env.PLAYWRIGHT_SINGLE_PROCESS
    ? { args: ['--single-process', '--no-zygote', '--no-sandbox'] }
    : {};
  const browser = await chromium.launch(launchOpts);
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });

  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });

  let harness = null;
  let dataDir = null;

  if (useMock) {
    await installMockApi(page);
    const indexUrl = pathToFileURL(path.join(root, 'app/renderer/index.html')).href;
    await page.goto(indexUrl, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('#nav-home');
    await page.waitForTimeout(700);
  } else {
    dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fistbump-theme-shots-'));
    const postings = ['data-analyst', 'backend-engineer', 'financial-analyst'];
    harness = await startHarness(dataDir);
    await harness.close();
    const db = fs.readdirSync(dataDir, { recursive: true }).find((f) => String(f).endsWith('fistbump.db'));
    execFileSync('sqlite3', [path.join(dataDir, db)], {
      input: fs.readFileSync(path.join(root, 'backend/testdata/seed.sql')),
    });
    harness = await startHarness(dataDir);
    await page.goto(harness.url, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('#nav-home');
    const ids = [];
    for (const name of postings) {
      const text = fs.readFileSync(path.join(root, 'backend/testdata/postings', `${name}.txt`), 'utf8');
      const job = await page.evaluate((raw) => window.api.jobs.create({ source: 'pasted', raw_text: raw }), text);
      ids.push(job.id ?? job.data?.id);
    }
    await page.evaluate((id) => window.api.applications.create({
      job_id: id, status: 'Interviewing', date_applied: '2026-11-10', next_step_date: '2026-11-25',
    }), ids[1]);
    await page.evaluate(() => {
      localStorage.setItem('fistbump.search-history', JSON.stringify([
        { fields: { 'search-keywords': 'Software Engineer', 'search-location': 'Remote', 'search-work-mode': 'Remote' }, industry: 'Technology', at: new Date().toISOString() },
        { fields: { 'search-keywords': 'Data Analyst', 'search-location': 'Charleston, SC', 'search-work-mode': 'Hybrid' }, industry: '', at: new Date().toISOString() },
      ]));
    });
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForSelector('#nav-home');
    await page.waitForTimeout(700);
  }

  //Home's matches come from a live search; wait for them so no picture says "Looking for roles"
  await page.waitForFunction(() => !document.body.innerText.includes('Looking for roles that match'), null, { timeout: 30000 }).catch(() => {});
  //settled pictures, the bar where most people keep it, and the Night version of each theme
  await page.evaluate(() => { window.Prefs.set('reduceMotion', 'on'); window.Prefs.set('barPosition', 'bottom'); window.Prefs.set('mode', 'night'); });
  for (const theme of ALL_THEMES) {
    await page.evaluate((t) => window.Prefs.set('theme', t), theme);
    await page.waitForTimeout(220);
    await page.screenshot({ path: path.join(out, `${theme}.png`) });
    console.log('saved', theme);
  }

  //each theme by day too, into docs/images/themes/day/
  fs.mkdirSync(path.join(out, 'day'), { recursive: true });
  await page.evaluate(() => window.Prefs.set('mode', 'day'));
  for (const theme of ALL_THEMES) {
    await page.evaluate((t) => window.Prefs.set('theme', t), theme);
    await page.waitForTimeout(220);
    await page.screenshot({ path: path.join(out, 'day', `${theme}.png`) });
    console.log('saved day', theme);
  }
  await page.evaluate(() => { window.Prefs.set('mode', 'night'); window.Prefs.set('theme', 'classic'); });

  await browser.close();
  if (harness) await harness.close();
  if (dataDir) fs.rmSync(dataDir, { recursive: true, force: true });

  if (errors.length) {
    console.error('Console/page errors detected during theme capture:', errors);
    process.exit(1);
  }
  console.log(`Captured ${ALL_THEMES.length} theme screenshots cleanly with 0 errors.`);
})().catch((e) => {
  console.error(e);
  process.exit(1);
});

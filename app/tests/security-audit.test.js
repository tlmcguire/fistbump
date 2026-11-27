const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');

const ROOT = path.join(__dirname, '..', '..');

test('verify-community-data.js passes on current repo catalogs', () => {
  const out = execFileSync(process.execPath, [path.join(ROOT, 'scripts', 'verify-community-data.js')], {
    cwd: ROOT,
    encoding: 'utf8',
  });
  assert.match(out, /passed security checks/);
});

test('update-ats-boards.js --offline validates public_boards.json', () => {
  const out = execFileSync(process.execPath, [path.join(ROOT, 'scripts', 'update-ats-boards.js'), '--offline'], {
    cwd: ROOT,
    encoding: 'utf8',
  });
  assert.match(out, /ok \(offline\)/);
});

test('update-ats-boards.js enforces strict institutional host allowlist and rejects HTML/cross-host redirects', () => {
  const src = fs.readFileSync(path.join(ROOT, 'scripts', 'update-ats-boards.js'), 'utf8');
  assert.match(src, /ALLOWED_HOSTS\s*=\s*new Set\(/);
  assert.match(src, /datasets-server\.huggingface\.co/);
  assert.match(src, /index\.commoncrawl\.org/);
  assert.match(src, /redirect:\s*'manual'/);
  assert.match(src, /text\/html/);
  assert.doesNotMatch(src, /lemonsignal\.io/);
  assert.doesNotMatch(src, /datahub\.io/);
});

test('update-cost-data.js blocks cross-domain and non-HTTPS redirects', () => {
  const src = fs.readFileSync(path.join(ROOT, 'scripts', 'update-cost-data.js'), 'utf8');
  assert.match(src, /redirect:\s*'manual'/);
  assert.match(src, /baseDomain\(next\.hostname\)\s*!==\s*allowedBase/);
  assert.match(src, /next\.protocol\s*!==\s*'https:'/);
});

test('GitHub Actions workflows enforce PR review, timeouts, and community data security gate', () => {
  const atsWf = fs.readFileSync(path.join(ROOT, '.github', 'workflows', 'update-ats-boards.yml'), 'utf8');
  assert.match(atsWf, /cron:\s*'0 7 1 \* \*'/);
  assert.match(atsWf, /timeout-minutes:\s*15/);
  assert.match(atsWf, /verify-community-data\.js/);
  assert.match(atsWf, /gh pr create/);

  const prWf = fs.readFileSync(path.join(ROOT, '.github', 'workflows', 'verify-community-pr.yml'), 'utf8');
  assert.match(prWf, /community-boards\.json/);
  assert.match(prWf, /community-listings\.json/);
  assert.match(prWf, /public_boards\.json/);
  assert.match(prWf, /persist-credentials:\s*false/);
});

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
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

test('trusted community verifier checks data from an explicit root', () => {
  const dataRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'community-data-audit-'));
  try {
    fs.mkdirSync(path.join(dataRoot, 'data'));
    fs.writeFileSync(
      path.join(dataRoot, 'data', 'community-listings.json'),
      JSON.stringify({
        listings: [{
          id: 'non-https',
          listing_url: 'http://example.test/jobs',
          added: '2026-01-01',
          expires_at: '2026-02-01',
        }],
      }),
    );
    assert.throws(
      () => execFileSync(
        process.execPath,
        [path.join(ROOT, 'scripts', 'verify-community-data.js'), '--root', dataRoot],
        { cwd: ROOT, encoding: 'utf8' },
      ),
      (error) => /non-HTTPS URL/.test(error.stderr),
    );
  } finally {
    fs.rmSync(dataRoot, { recursive: true, force: true });
  }
});

test('community verifier rejects caller-set evergreen flags and extended standard expiry', () => {
  const dataRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'community-data-policy-'));
  try {
    fs.mkdirSync(path.join(dataRoot, 'data'));
    fs.writeFileSync(
      path.join(dataRoot, 'data', 'community-listings.json'),
      JSON.stringify({
        listings: [{
          id: 'extended-standard',
          company_name: 'Example',
          position_title: 'Backend Engineer',
          raw_text: 'A standard full-time backend engineering position.',
          listing_url: 'https://example.test/jobs/1',
          added: '2026-01-01',
          expires_at: '2026-04-01',
          closes_at: '2026-06-30',
          evergreen: true,
        }],
      }),
    );
    assert.throws(
      () => execFileSync(
        process.execPath,
        [path.join(ROOT, 'scripts', 'verify-community-data.js'), '--root', dataRoot],
        { cwd: ROOT, encoding: 'utf8' },
      ),
      (error) => /evergreen flag is not supported/.test(error.stderr)
        && /exceeds the allowed timeframe/.test(error.stderr),
    );
  } finally {
    fs.rmSync(dataRoot, { recursive: true, force: true });
  }
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
  assert.match(prWf, /pull_request_target:/);
  assert.match(prWf, /github\.event\.pull_request\.base\.sha/);
  assert.match(prWf, /github\.event\.pull_request\.head\.sha/);
  assert.match(prWf, /path:\s*trusted/);
  assert.match(prWf, /path:\s*pr/);
  assert.match(prWf, /node trusted\/scripts\/verify-community-data\.js --root pr/);
  assert.match(prWf, /persist-credentials:\s*false/);

  const pruneWf = fs.readFileSync(path.join(ROOT, '.github', 'workflows', 'prune-community-listings.yml'), 'utf8');
  assert.match(pruneWf, /cron:\s*'0 7 \* \* 1'/);
  assert.match(pruneWf, /timeout-minutes:\s*15/);
  assert.match(pruneWf, /prune-community-listings\.js/);
  assert.match(pruneWf, /verify-community-data\.js/);
  assert.match(pruneWf, /gh pr create/);

  assert.ok(fs.existsSync(path.join(ROOT, 'docs', 'security-policy.md')));
  assert.ok(fs.existsSync(path.join(ROOT, 'docs', 'periodic-external-calls.md')));
});

test('prune-community-listings.js enforces strict timeframes, close dates, and long-running protections', () => {
  const pruner = require(path.join(ROOT, 'scripts', 'prune-community-listings.js'));
  const today = '2026-11-26';

  // 1. Dateless listing cannot escape pruning.
  assert.deepEqual(
    pruner.evaluateListing({ id: 'no-date', company_name: 'Acme', position_title: 'Engineer', raw_text: 'Great role.' }, today),
    { keep: false, reason: 'missing_timeframe' }
  );

  // 2. LinkedIn listing with elapsed close date in raw_text is pruned even if recently added.
  assert.deepEqual(
    pruner.evaluateListing({
      id: 'li-closed',
      source: 'LinkedIn',
      added: '2026-11-01',
      raw_text: 'Senior Engineer role. Apply by November 15, 2026 for consideration.',
    }, today),
    { keep: false, reason: 'close_date_elapsed' }
  );

  // 3. Standard LinkedIn listing > 45 days old is pruned.
  assert.deepEqual(
    pruner.evaluateListing({
      id: 'li-stale',
      source: 'LinkedIn',
      added: '2026-09-20',
      raw_text: 'Standard backend role at Example Corp.',
    }, today),
    { keep: false, reason: 'standard_ttl_elapsed' }
  );

  // 4. Long-running / evergreen listing at 75 days old is kept (protected from 45-day cutoff).
  const evergreenKeep = pruner.evaluateListing({
    id: 'evergreen-ok',
    source: 'LinkedIn',
    added: '2026-09-12',
    position_title: 'Software Engineer (General Application)',
    raw_text: 'Applications are accepted on a rolling basis and open until filled.',
  }, today);
  assert.equal(evergreenKeep.keep, true);
  assert.equal(evergreenKeep.updated.evergreen, true);
  assert.equal(evergreenKeep.updated.expires_at, '2027-01-10');

  // 5. Long-running listing > 120 days old is pruned.
  assert.deepEqual(
    pruner.evaluateListing({
      id: 'evergreen-old',
      added: '2026-07-01',
      evergreen: true,
      raw_text: 'Evergreen pipeline role.',
    }, today),
    { keep: false, reason: 'evergreen_ttl_elapsed' }
  );

  // 6. A caller-supplied evergreen flag cannot extend a standard listing.
  assert.deepEqual(
    pruner.evaluateListing({
      id: 'false-evergreen',
      added: '2026-09-01',
      evergreen: true,
      raw_text: 'A standard backend engineering position.',
    }, today),
    { keep: false, reason: 'standard_ttl_elapsed' }
  );

  // 7. A caller-supplied expiry cannot extend the computed standard timeframe.
  const expiryCapped = pruner.evaluateListing({
    id: 'extended-expiry',
    added: '2026-11-01',
    expires_at: '2027-04-30',
    closes_at: '2027-04-30',
    raw_text: 'A standard backend engineering position.',
  }, today);
  assert.equal(expiryCapped.keep, true);
  assert.equal(expiryCapped.updated.expires_at, '2026-12-16');

  // 8. Verified closed ATS posting (404/410) is pruned immediately.
  assert.deepEqual(
    pruner.evaluateListing({ id: 'ats-404', added: '2026-11-20', raw_text: 'Active role' }, today, 'closed'),
    { keep: false, reason: 'ats_posting_closed' }
  );

  // 9. Verified live ATS posting (200) is kept past 45 days, but still pruned after 180-day hard ceiling.
  const liveAts = pruner.evaluateListing({
    id: 'ats-live',
    added: '2026-09-01',
    expires_at: '2027-01-01',
    raw_text: 'Active role',
  }, today, 'open');
  assert.equal(liveAts.keep, true);
  assert.equal(liveAts.updated.expires_at, '2026-10-16');
  assert.deepEqual(
    pruner.evaluateListing({ id: 'ats-hard-cap', added: '2026-05-01', raw_text: 'Old role' }, today, 'open'),
    { keep: false, reason: 'hard_max_age_exceeded' }
  );
});

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
        version: 1,
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

test('trusted community verifier refuses symlinked catalogs and parent directories', t => {
  if (process.platform === 'win32') return t.skip('symlink creation requires Windows developer mode');
  const dataRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'community-data-symlink-'));
  t.after(() => fs.rmSync(dataRoot, { recursive: true, force: true }));
  fs.symlinkSync(path.join(ROOT, 'data'), path.join(dataRoot, 'data'), 'dir');
  assert.throws(
    () => execFileSync(process.execPath, [path.join(ROOT, 'scripts', 'verify-community-data.js'), '--root', dataRoot]),
    error => /symlinks are not allowed/.test(String(error.stderr)),
  );
});

test('community verifier rejects caller-set evergreen flags and extended standard expiry', () => {
  const dataRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'community-data-policy-'));
  try {
    fs.mkdirSync(path.join(dataRoot, 'data'));
    fs.writeFileSync(
      path.join(dataRoot, 'data', 'community-listings.json'),
      JSON.stringify({
        version: 1,
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

test('StoreSignal board ingestion follows the dataset row count instead of a fixed page cap', async () => {
  const updater = require(path.join(ROOT, 'scripts', 'update-ats-boards.js'));
  const originalFetch = global.fetch;
  const offsets = [];
  global.fetch = async (input) => {
    const url = new URL(input);
    if (url.pathname.endsWith('/splits')) {
      return new Response(JSON.stringify({ splits: [{ config: 'default', split: 'train' }] }), {
        headers: { 'content-type': 'application/json' },
      });
    }
    const offset = Number(url.searchParams.get('offset'));
    offsets.push(offset);
    const total = 205;
    const count = Math.min(100, total - offset);
    const rows = Array.from({ length: count }, (_, index) => ({
      row: { ats: 'greenhouse', slug: `board-${offset + index}` },
    }));
    return new Response(JSON.stringify({ num_rows_total: total, rows }), {
      headers: { 'content-type': 'application/json' },
    });
  };
  try {
    const boards = [];
    const added = await updater.pullHuggingFaceDataset('example/boards', (ats, token) => {
      boards.push(`${ats}:${token}`);
      return true;
    });
    assert.equal(added, 205);
    assert.equal(boards.length, 205);
    assert.deepEqual(offsets.sort((a, b) => a - b), [0, 100, 200]);
  } finally {
    global.fetch = originalFetch;
  }
});

test('ATS candidate verification shares its budget and matches official URL hosts exactly', () => {
  const { parseBoardFromURL, selectCandidates } = require(path.join(ROOT, 'scripts', 'update-ats-boards.js'));
  assert.deepEqual(parseBoardFromURL('https://jobs.lever.co/example/123'), { ats: 'lever', token: 'example' });
  for (const url of ['https://fakejobs.lever.co/example', 'https://untrusted.test/jobs.lever.co/example', 'https://jobs.lever.co:444/example']) {
    assert.equal(parseBoardFromURL(url), null);
  }
  const ats = ['greenhouse', 'lever', 'ashby'];
  const candidates = ats.flatMap(source => Array.from({ length: 50 }, (_, i) => ({ ats: source, token: `board-${i}` })));
  const selected = selectCandidates(candidates, '2026-11-26', 12);
  assert.equal(selected.length, 12);
  assert.deepEqual(ats.map(source => selected.filter(entry => entry.ats === source).length), [4, 4, 4]);
  assert.deepEqual(selectCandidates([...candidates].reverse(), '2026-11-26', 12), selected);
});

test('Open-Apply seeds only supported ATS board slugs and reports the newest daily partition', async () => {
  const updater = require(path.join(ROOT, 'scripts', 'update-ats-boards.js'));
  const originalFetch = global.fetch;
  const requests = [];
  global.fetch = async (input) => {
    const url = new URL(input);
    requests.push(url.hostname + url.pathname);
    if (url.hostname === 'huggingface.co') {
      return new Response(JSON.stringify([
        { path: 'data/date=2026-11-24', type: 'directory' },
        { path: 'data/date=2026-11-26', type: 'directory' },
        { path: 'data/not-a-date', type: 'directory' },
      ]), { headers: { 'content-type': 'application/json' } });
    }
    const name = url.pathname.split('/').pop();
    const files = {
      'cc_greenhouse_FINAL.txt': 'stripe\nnew-gh\n',
      'cc_lever_FINAL.txt': 'new-lever\n',
      'cc_ashby_FINAL.txt': 'new-ashby\n',
    };
    return new Response(files[name], { headers: { 'content-type': 'text/plain' } });
  };
  try {
    assert.equal(await updater.latestOpenApplyPartition(), '2026-11-26');
    const added = [];
    const count = await updater.pullOpenApplyBoardSlugs((ats, token) => {
      added.push(`${ats}:${token}`);
      return true;
    });
    assert.equal(count, 4);
    assert.deepEqual(added.sort(), [
      'ashby:new-ashby',
      'greenhouse:new-gh',
      'greenhouse:stripe',
      'lever:new-lever',
    ]);
    assert.equal(requests.filter((request) => request.startsWith('raw.githubusercontent.com')).length, 3);
  } finally {
    global.fetch = originalFetch;
  }
});

test('dead ATS board check distinguishes definitive removal from empty or transient boards', async () => {
  const checker = require(path.join(ROOT, 'scripts', 'check-dead-ats-boards.js'));
  let requestOptions;
  const board = { ats: 'greenhouse', token: 'exampleco', company: 'Example Co', sources: ['curated'] };

  const emptyBoard = await checker.checkBoard(board, async (_url, options) => {
    requestOptions = options;
    return new Response('{"jobs":[]}', { status: 200 });
  });
  assert.equal(emptyBoard.status, 'live');
  assert.equal(requestOptions.redirect, 'error');

  const missingBoard = await checker.checkBoard(board, async () => new Response('{}', { status: 404 }));
  assert.equal(missingBoard.status, 'dead');
  assert.equal(missingBoard.reason, 'HTTP 404');

  const goneBoard = await checker.checkBoard(board, async () => new Response('{}', { status: 410 }));
  assert.equal(goneBoard.status, 'dead');
  assert.equal(goneBoard.reason, 'HTTP 410');

  const unavailableBoard = await checker.checkBoard(board, async () => new Response('{}', { status: 503 }));
  assert.equal(unavailableBoard.status, 'unknown');
  assert.equal(unavailableBoard.reason, 'HTTP 503');

  const boards = checker.loadBoards();
  assert.equal(new Set(boards.map(({ ats, token }) => `${ats}:${token}`)).size, boards.length);
  assert.deepEqual([...new Set(boards.map(({ ats }) => ats))].sort(), ['ashby', 'greenhouse', 'lever']);
  const sharded = Array.from({ length: 7 }, (_, shard) => checker.boardsForShard(boards, shard));
  assert.equal(sharded.flat().length, boards.length);
  assert.equal(new Set(sharded.flat().map(({ ats, token }) => `${ats}:${token}`)).size, boards.length);
});

test('update-ats-boards.js enforces strict institutional host allowlist and rejects HTML/cross-host redirects', () => {
  const src = fs.readFileSync(path.join(ROOT, 'scripts', 'update-ats-boards.js'), 'utf8');
  assert.match(src, /ALLOWED_HOSTS\s*=\s*new Set\(/);
  assert.match(src, /datasets-server\.huggingface\.co/);
  assert.match(src, /huggingface\.co/);
  assert.match(src, /raw\.githubusercontent\.com/);
  assert.match(src, /index\.commoncrawl\.org/);
  assert.match(src, /redirect:\s*'manual'/);
  assert.match(src, /text\/html/);
  assert.match(src, /MAX_HF_DATASET_ROWS/);
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

  const openApplyWf = fs.readFileSync(path.join(ROOT, '.github', 'workflows', 'discover-openapply-boards.yml'), 'utf8');
  assert.match(openApplyWf, /cron:\s*'0 9 \* \* \*'/);
  assert.match(openApplyWf, /contents:\s*read/);
  assert.match(openApplyWf, /update-ats-boards\.js --openapply-only/);
  assert.match(openApplyWf, /gh pr create/);

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

  const deadBoardsWf = fs.readFileSync(path.join(ROOT, '.github', 'workflows', 'check-dead-ats-boards.yml'), 'utf8');
  assert.match(deadBoardsWf, /cron:\s*'0 8 \* \* \*'/);
  assert.match(deadBoardsWf, /timeout-minutes:\s*30/);
  assert.match(deadBoardsWf, /contents:\s*read/);
  assert.match(deadBoardsWf, /check-dead-ats-boards\.js/);

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

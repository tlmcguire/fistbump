#!/usr/bin/env node
// Refreshes backend/internal/connectors/public_boards.json from Hugging Face, Open-Apply,
// and Common Crawl board-discovery sources.
//
// Security hardening:
//   - Strict HTTPS-only host allowlist; cross-host redirects are rejected (protects against
//     domain expiration/takeover or malicious redirects).
//   - Strict Content-Type validation (rejects text/html landing/parked pages) and 5 MiB byte cap.
//   - Token format validation, reserved-slug denylist, and per-run anomaly cap.
//
// Usage:
//   node scripts/update-ats-boards.js                    pull all allowlisted sources
//   node scripts/update-ats-boards.js --skip-cc          skip Common Crawl CDX queries
//   node scripts/update-ats-boards.js --skip-openapply   skip Open-Apply board seeds
//   node scripts/update-ats-boards.js --openapply-only   refresh Open-Apply board seeds only
//   node scripts/update-ats-boards.js --offline          validate existing public_boards.json only
const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { checkBoards } = require('./check-dead-ats-boards.js');

const ROOT = path.join(__dirname, '..');
const OUT_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'public_boards.json');
const GH_JSON_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'greenhouse', 'boards.json');
const GH_GO_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'greenhouse', 'boards.go');
const LEVER_GO_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'lever', 'boards.go');
const ASHBY_GO_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'ashby', 'boards.go');

const OFFLINE = process.argv.includes('--offline');
const SKIP_CC = process.argv.includes('--skip-cc');
const SKIP_OPENAPPLY = process.argv.includes('--skip-openapply');
const OPENAPPLY_ONLY = process.argv.includes('--openapply-only');
const TOKEN_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;
const MAX_RESPONSE_BYTES = 5 * 1024 * 1024;
const MAX_REDIRECTS = 3;
// Bound live validation separately from source enumeration.
const MAX_NEW_BOARDS_PER_RUN = 180;
const MAX_HF_DATASET_ROWS = 25000;
const HF_PAGE_SIZE = 100;
const HF_PAGE_CONCURRENCY = 4;

// Strict host allowlist for automated GitHub Actions fetches. Small/personal third-party domains
// are intentionally excluded so a future domain sale or takeover cannot hijack the workflow.
const ALLOWED_HOSTS = new Set([
  'datasets-server.huggingface.co',
  'huggingface.co',
  'index.commoncrawl.org',
  'raw.githubusercontent.com',
]);

const RESERVED_TOKENS = new Set([
  'embed', 'jobs', 'apply', 'api', 'v1', 'v0', 'assets', 'static', 'favicon', 'robots',
  'sitemap', 'login', 'signup', 'auth', 'cdn', 'public', 'app', 'www', 'boards', 'careers',
  'job_app', 'job_board', 'postings', 'posting-api', 'job-board', 'privacy', 'terms',
  'example', 'company', 'name', 'new', 'events', 'abcd', 'gdpr',
]);

// Filters out obvious sandbox/demo/test boards while preserving real companies like sandboxaq or testgorilla.
const SANDBOX_RE = /(?:^|[-_])(sandbox\d*|demo-org|embed-demo|examplecorp|testboard)(?:$|[-_])|sandbox\d*$/i;

function assertSafeAllowlistedUrl(raw) {
  const u = new URL(raw);
  if (u.protocol !== 'https:') throw new Error(`Refusing non-HTTPS URL: ${raw}`);
  if (u.username || u.password) throw new Error(`Refusing URL with credentials: ${raw}`);
  const host = u.hostname.toLowerCase();
  if (!ALLOWED_HOSTS.has(host)) {
    throw new Error(`Refusing host outside allowlist: ${host}`);
  }
  return u.toString();
}

async function safeFetch(rawUrl, { expectJson = false, timeoutMs = 20000 } = {}) {
  let current = assertSafeAllowlistedUrl(rawUrl);
  for (let hop = 0; hop <= MAX_REDIRECTS; hop++) {
    const res = await fetch(current, {
      headers: {
        'User-Agent': 'Fistbump/0.5 (public ATS board updater)',
        Accept: expectJson ? 'application/json' : 'application/json, text/plain',
      },
      redirect: 'manual',
      signal: AbortSignal.timeout(timeoutMs),
    });
    if (res.status >= 300 && res.status < 400) {
      const loc = res.headers.get('location');
      await res.body?.cancel();
      if (!loc) throw new Error(`Redirect without Location header from ${current}`);
      const next = assertSafeAllowlistedUrl(new URL(loc, current).toString());
      if (new URL(next).origin !== new URL(current).origin) throw new Error(`Cross-origin redirect from ${current}`);
      current = next;
      continue;
    }
    if (!res.ok) throw new Error(`HTTP ${res.status} from ${current}`);
    const ct = (res.headers.get('content-type') || '').toLowerCase();
    if (ct.includes('text/html')) {
      throw new Error(`Unexpected HTML response (${ct}) from ${current} — possible parked/hijacked endpoint`);
    }
    if (expectJson && !ct.includes('json')) {
      throw new Error(`Expected JSON content-type, got ${ct || 'none'} from ${current}`);
    }
    const chunks = [];
    let length = 0;
    const reader = res.body?.getReader();
    if (!reader) return '';
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        length += value.byteLength;
        if (length > MAX_RESPONSE_BYTES) {
          await reader.cancel();
          throw new Error(`Response exceeded ${MAX_RESPONSE_BYTES} bytes from ${current}`);
        }
        chunks.push(value);
      }
    } finally {
      reader.releaseLock();
    }
    return Buffer.concat(chunks, length).toString('utf8');
  }
  throw new Error(`Too many redirects fetching ${rawUrl}`);
}

function loadCuratedTokens() {
  const curated = { greenhouse: new Set(), lever: new Set(), ashby: new Set() };
  try {
    const gh = JSON.parse(fs.readFileSync(GH_JSON_PATH, 'utf8'));
    for (const c of gh.categories || []) {
      for (const b of c.boards || []) curated.greenhouse.add(b.token.toLowerCase());
    }
  } catch {}
  try {
    const src = fs.readFileSync(GH_GO_PATH, 'utf8');
    for (const m of src.matchAll(/Token:\s*"([^"]+)"/g)) curated.greenhouse.add(m[1].toLowerCase());
  } catch {}
  for (const [ats, file] of [['lever', LEVER_GO_PATH], ['ashby', ASHBY_GO_PATH]]) {
    try {
      const src = fs.readFileSync(file, 'utf8');
      for (const m of src.matchAll(/\{"([^"]+)",\s*"([^"]+)"\}/g)) curated[ats].add(m[1].toLowerCase());
    } catch {}
  }
  return curated;
}

function titleFromSlug(slug) {
  return slug.replace(/[-_]+/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

function sanitizeCompany(raw, fallbackSlug) {
  const cleaned = String(raw || '')
    .replace(/[\u0000-\u001f\u007f\u200b-\u200f\u202a-\u202e\u2060-\u206f\ufeff]/g, '')
    .trim()
    .slice(0, 80);
  return cleaned || titleFromSlug(fallbackSlug);
}

const ATS_BOARD_HOSTS = new Map([
  ['job-boards.greenhouse.io', 'greenhouse'],
  ['boards.greenhouse.io', 'greenhouse'],
  ['jobs.lever.co', 'lever'],
  ['jobs.ashbyhq.com', 'ashby'],
]);

function parseBoardFromURL(raw) {
  if (!raw || typeof raw !== 'string') return null;
  try {
    const url = new URL(raw);
    if (url.protocol !== 'https:' || url.username || url.password || url.port) return null;
    const ats = ATS_BOARD_HOSTS.get(url.hostname);
    const token = url.pathname.split('/')[1]?.toLowerCase();
    return ats && TOKEN_RE.test(token || '') ? { ats, token } : null;
  } catch {
    return null;
  }
}

async function pullHuggingFaceDataset(dataset, add) {
  const splits = JSON.parse(await safeFetch(
    `https://datasets-server.huggingface.co/splits?dataset=${encodeURIComponent(dataset)}`,
    { expectJson: true }
  ));
  const first = (splits.splits || [])[0];
  if (!first) return 0;
  const { config, split } = first;
  const rowsURL = (offset) => `https://datasets-server.huggingface.co/rows?dataset=${encodeURIComponent(dataset)}&config=${encodeURIComponent(config)}&split=${encodeURIComponent(split)}&offset=${offset}&length=${HF_PAGE_SIZE}`;
  const firstPage = JSON.parse(await safeFetch(rowsURL(0), { expectJson: true }));
  const totalRows = firstPage.num_rows_total;
  if (!Number.isSafeInteger(totalRows) || totalRows < 0) {
    throw new Error(`Dataset ${dataset} did not provide a valid num_rows_total`);
  }
  if (totalRows > MAX_HF_DATASET_ROWS) {
    throw new Error(`Dataset ${dataset} has ${totalRows} rows; refusing to scan more than ${MAX_HF_DATASET_ROWS}`);
  }

  const pages = [firstPage.rows || []];
  for (let offset = HF_PAGE_SIZE; offset < totalRows; offset += HF_PAGE_SIZE * HF_PAGE_CONCURRENCY) {
    const offsets = Array.from(
      { length: Math.min(HF_PAGE_CONCURRENCY, Math.ceil((totalRows - offset) / HF_PAGE_SIZE)) },
      (_, index) => offset + index * HF_PAGE_SIZE
    );
    const batch = await Promise.all(offsets.map(async (pageOffset) => {
      const data = JSON.parse(await safeFetch(rowsURL(pageOffset), { expectJson: true }));
      if (data.num_rows_total !== totalRows) {
        throw new Error(`Dataset ${dataset} changed during pagination`);
      }
      const expectedRows = Math.min(HF_PAGE_SIZE, totalRows - pageOffset);
      if (!Array.isArray(data.rows) || data.rows.length !== expectedRows) {
        throw new Error(`Dataset ${dataset} returned an incomplete page at offset ${pageOffset}`);
      }
      return data.rows;
    }));
    pages.push(...batch);
  }

  if (!Array.isArray(firstPage.rows) || firstPage.rows.length !== Math.min(HF_PAGE_SIZE, totalRows)) {
    throw new Error(`Dataset ${dataset} returned an incomplete first page`);
  }

  let added = 0;
  for (const page of pages) {
    for (const item of page) {
      const r = item.row || {};
      if (r.open_jobs !== undefined && r.open_jobs !== null && Number(r.open_jobs) <= 0) continue;
      const company = String(r.company || r.company_name || r.employer || r.name || '').trim();
      let ats = String(r.ats || r.source || r.platform || r.hiringSystem || r.ats_platform || '').trim().toLowerCase();
      let token = String(r.board || r.slug || r.board_slug || r.token || r.board_token || r.company_slug || '').trim().toLowerCase();
      if (!token && typeof r.company_id === 'string' && r.company_id.includes(':')) {
        const [a, t] = r.company_id.split(':');
        ats = ats || a.trim().toLowerCase();
        token = t.trim().toLowerCase();
      }
      if (!ats || !token) {
        const parsed = parseBoardFromURL(r.url || r.board_url || r.apply_url || r.careers_url || r.job_url || '');
        if (parsed) { ats = parsed.ats; token = parsed.token; }
      }
      if (ats && token && add(ats, token, company)) added++;
    }
  }
  return added;
}

async function latestOpenApplyPartition() {
  const entries = JSON.parse(await safeFetch(
    'https://huggingface.co/api/datasets/edwarddgao/open-apply-jobs/tree/main/data?recursive=false&limit=1000',
    { expectJson: true }
  ));
  const dates = (entries || [])
    .map((entry) => String(entry.path || '').match(/^data\/date=(\d{4}-\d{2}-\d{2})$/)?.[1])
    .filter(Boolean)
    .filter((date) => {
      const parsed = new Date(`${date}T00:00:00Z`);
      return !Number.isNaN(parsed.valueOf()) && parsed.toISOString().slice(0, 10) === date;
    })
    .sort();
  if (!dates.length) throw new Error('Open-Apply did not expose a dated daily partition');
  return dates[dates.length - 1];
}

async function pullOpenApplyBoardSlugs(add) {
  try {
    console.log(`Open-Apply latest daily snapshot: ${await latestOpenApplyPartition()}`);
  } catch (err) {
    console.warn(`Open-Apply snapshot metadata unavailable: ${err.message}`);
  }
  const files = [
    ['greenhouse', 'cc_greenhouse_FINAL.txt'],
    ['lever', 'cc_lever_FINAL.txt'],
    ['ashby', 'cc_ashby_FINAL.txt'],
  ];
  const results = await Promise.all(files.map(async ([ats, file]) =>
    [ats, await safeFetch(`https://raw.githubusercontent.com/edwarddgao/openapply/main/slugs/${file}`)]));
  let added = 0;
  for (const [ats, text] of results) {
    for (const line of text.split(/\r?\n/)) {
      const token = line.trim().toLowerCase();
      if (token && add(ats, token, '')) added++;
    }
  }
  return added;
}

async function pullCommonCrawl(add) {
  const colls = JSON.parse(await safeFetch('https://index.commoncrawl.org/collinfo.json', { expectJson: true }));
  const latest = (colls || [])[0];
  if (!latest || !latest['cdx-api']) return 0;
  const cdx = assertSafeAllowlistedUrl(latest['cdx-api']);
  if (new URL(cdx).origin !== 'https://index.commoncrawl.org') throw new Error('Unexpected Common Crawl index origin');
  const queries = [
    ['greenhouse', 'boards.greenhouse.io/*'],
    ['greenhouse', 'job-boards.greenhouse.io/*'],
    ['lever', 'jobs.lever.co/*'],
    ['ashby', 'jobs.ashbyhq.com/*'],
  ];
  let added = 0;
  for (const [ats, pattern] of queries) {
    try {
      const url = `${cdx}?url=${encodeURIComponent(pattern)}&output=json&fl=url&filter=status:200&collapse=urlkey&limit=1500`;
      const text = await safeFetch(url, { timeoutMs: 30000 });
      for (const line of text.split('\n')) {
        if (!line.trim()) continue;
        try {
          const rec = JSON.parse(line);
          const parsed = parseBoardFromURL(rec.url);
          if (parsed && parsed.ats === ats && add(ats, parsed.token, '')) added++;
        } catch {}
      }
    } catch (err) {
      console.warn(`Common Crawl query skipped for ${pattern}: ${err.message}`);
    }
  }
  return added;
}

async function main() {
  const catalog = JSON.parse(fs.readFileSync(OUT_PATH, 'utf8'));
  if (OFFLINE) {
    const counts = ['greenhouse', 'lever', 'ashby'].map((k) => `${k}=${(catalog.boards?.[k] || []).length}`).join(', ');
    console.log(`ok (offline): ${counts}, updated_at=${catalog.updated_at}`);
    return;
  }

  const curated = loadCuratedTokens();
  const seen = { greenhouse: new Map(), lever: new Map(), ashby: new Map() };
  for (const ats of ['greenhouse', 'lever', 'ashby']) {
    for (const b of catalog.boards?.[ats] || []) {
      const tok = String(b.token || '').trim().toLowerCase();
      if (TOKEN_RE.test(tok) && !RESERVED_TOKENS.has(tok) && !curated[ats].has(tok)) {
        seen[ats].set(tok, sanitizeCompany(b.company, tok));
      }
    }
  }

  const candidates = new Map();
  const queueCandidate = (id, rawAts, rawToken, rawCompany) => {
    const ats = rawAts.toLowerCase().trim();
    if (!Object.hasOwn(seen, ats)) return false;
    const token = rawToken.toLowerCase().trim();
    if (!TOKEN_RE.test(token) || RESERVED_TOKENS.has(token) || SANDBOX_RE.test(token) || curated[ats].has(token)) return false;
    const company = sanitizeCompany(rawCompany, token);
    if (seen[ats].has(token)) {
      if (rawCompany && seen[ats].get(token) === titleFromSlug(token)) seen[ats].set(token, company);
      return false;
    }
    const key = `${ats}:${token}`;
    if (candidates.has(key)) return false;
    candidates.set(key, { id, ats, token, company });
    return true;
  };

  const today = new Date().toISOString().slice(0, 10);
  const runners = [];
  if (!OPENAPPLY_ONLY) {
    runners.push(['hf_storesignal_ats_boards', add => pullHuggingFaceDataset('storesignalhq/ats-job-boards', add)]);
    if (!SKIP_CC) runners.push(['common_crawl_cdx', pullCommonCrawl]);
  }
  if (!SKIP_OPENAPPLY) runners.push(['openapply_board_seeds', pullOpenApplyBoardSlugs]);

  let anySucceeded = false;
  for (const [id, fn] of runners) {
    try {
      const n = await fn((...board) => queueCandidate(id, ...board));
      anySucceeded = true;
      console.log(`source ${id}: ${n} new candidates`);
    } catch (err) {
      console.warn(`source ${id} skipped: ${err.message}`);
    }
  }

  if (!anySucceeded) throw new Error('All ATS board sources failed');
  const selected = selectCandidates(candidates.values(), today, MAX_NEW_BOARDS_PER_RUN);
  const counts = new Map();
  const checked = await checkBoards(selected);
  for (const entry of checked) {
    if (entry.status !== 'live') continue;
    seen[entry.ats].set(entry.token, entry.company);
    counts.set(entry.id, (counts.get(entry.id) || 0) + 1);
  }
  for (const [id, count] of counts) recordSource(catalog, id, count, today);
  console.log(`ATS candidates: ${candidates.size} discovered, ${checked.length} checked, ${[...counts.values()].reduce((a, b) => a + b, 0)} verified and added`);
  const originalBoards = JSON.stringify(catalog.boards);
  catalog.boards = {};
  for (const ats of ['greenhouse', 'lever', 'ashby']) {
    catalog.boards[ats] = [...seen[ats].entries()]
      .sort((a, b) => a[0].localeCompare(b[0]))
      .map(([token, company]) => ({ token, company }));
  }
  if (JSON.stringify(catalog.boards) !== originalBoards) {
    catalog.updated_at = today;
    catalog.refresh_days = 30;
  }
  fs.writeFileSync(OUT_PATH, JSON.stringify(catalog, null, 2) + '\n', 'utf8');
  const ghN = catalog.boards.greenhouse.length;
  const lvN = catalog.boards.lever.length;
  const asN = catalog.boards.ashby.length;
  console.log(`Saved public_boards.json: greenhouse=${ghN}, lever=${lvN}, ashby=${asN} (total=${ghN + lvN + asN})`);
}

// Rotate the review batch daily and share the request budget across ATSs.
function selectCandidates(candidates, day, limit) {
  const groups = { greenhouse: [], lever: [], ashby: [] };
  for (const entry of candidates) {
    const rank = createHash('sha256').update(`${day}:${entry.ats}:${entry.token}`).digest('hex');
    groups[entry.ats].push({ entry, rank });
  }
  const sorted = Object.values(groups).map(group => group.sort((a, b) => a.rank.localeCompare(b.rank)));
  const result = [];
  const widest = Math.max(...sorted.map(group => group.length));
  for (let round = 0; round < widest && result.length < limit; round++) {
    for (const group of sorted) {
      if (group[round]) result.push(group[round].entry);
      if (result.length === limit) break;
    }
  }
  return result;
}

function recordSource(catalog, id, boardsAdded, today) {
  catalog.sources ||= [];
  let source = catalog.sources.find((entry) => entry.id === id);
  if (!source) {
    const names = {
      hf_storesignal_ats_boards: 'Hugging Face: storesignalhq/ats-job-boards',
      common_crawl_cdx: 'Common Crawl CDX Index (Greenhouse, Lever, Ashby)',
      openapply_board_seeds: 'Open-Apply board seeds (Common Crawl-derived)',
    };
    source = { id, name: names[id] || id };
    catalog.sources.push(source);
  }
  source.updated_at = today;
  source.boards_added = boardsAdded;
}

if (require.main === module) {
  main().catch((err) => {
    console.error(`ATS board update failed: ${err.message}`);
    process.exitCode = 1;
  });
}

module.exports = {
  latestOpenApplyPartition,
  parseBoardFromURL,
  pullHuggingFaceDataset,
  pullOpenApplyBoardSlugs,
  safeFetch,
  selectCandidates,
};

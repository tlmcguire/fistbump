#!/usr/bin/env node
// Refreshes backend/internal/connectors/public_boards.json on a 30-day cadence by pulling
// Greenhouse, Lever, and Ashby board slugs ONLY from a strict allowlist of institutional/
// infrastructure hosts (Hugging Face Datasets Server & Common Crawl CDX).
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
//   node scripts/update-ats-boards.js --offline          validate existing public_boards.json only
const fs = require('node:fs');
const path = require('node:path');

const ROOT = path.join(__dirname, '..');
const OUT_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'public_boards.json');
const GH_JSON_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'greenhouse', 'boards.json');
const GH_GO_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'greenhouse', 'boards.go');
const LEVER_GO_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'lever', 'boards.go');
const ASHBY_GO_PATH = path.join(ROOT, 'backend', 'internal', 'connectors', 'ashby', 'boards.go');

const OFFLINE = process.argv.includes('--offline');
const SKIP_CC = process.argv.includes('--skip-cc');
const TOKEN_RE = /^[a-z0-9_-]{1,64}$/;
const MAX_RESPONSE_BYTES = 5 * 1024 * 1024;
const MAX_REDIRECTS = 3;
const MAX_NEW_BOARDS_PER_RUN = 5000;

// Strict host allowlist for automated GitHub Actions fetches. Small/personal third-party domains
// are intentionally excluded so a future domain sale or takeover cannot hijack the workflow.
const ALLOWED_HOSTS = new Set([
  'datasets-server.huggingface.co',
  'index.commoncrawl.org',
]);

const RESERVED_TOKENS = new Set([
  'embed', 'jobs', 'apply', 'api', 'v1', 'v0', 'assets', 'static', 'favicon', 'robots',
  'sitemap', 'login', 'signup', 'auth', 'cdn', 'public', 'app', 'www', 'boards', 'careers',
  'job_app', 'job_board', 'postings', 'posting-api', 'job-board', 'privacy', 'terms',
]);

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
      if (!loc) throw new Error(`Redirect without Location header from ${current}`);
      current = assertSafeAllowlistedUrl(new URL(loc, current).toString());
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
    const buf = Buffer.from(await res.arrayBuffer());
    if (buf.length > MAX_RESPONSE_BYTES) {
      throw new Error(`Response exceeded ${MAX_RESPONSE_BYTES} bytes from ${current}`);
    }
    return buf.toString('utf8');
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

function parseBoardFromURL(raw) {
  if (!raw || typeof raw !== 'string') return null;
  let m;
  if ((m = raw.match(/(?:job-boards|boards)\.greenhouse\.io\/([a-z0-9_-]{1,64})(?:[/?#]|$)/i))) {
    return { ats: 'greenhouse', token: m[1].toLowerCase() };
  }
  if ((m = raw.match(/jobs\.lever\.co\/([a-z0-9_-]{1,64})(?:[/?#]|$)/i))) {
    return { ats: 'lever', token: m[1].toLowerCase() };
  }
  if ((m = raw.match(/jobs\.ashbyhq\.com\/([a-z0-9_-]{1,64})(?:[/?#]|$)/i))) {
    return { ats: 'ashby', token: m[1].toLowerCase() };
  }
  return null;
}

async function pullHuggingFaceDataset(dataset, add, maxPages = 10) {
  let added = 0;
  const splits = JSON.parse(await safeFetch(
    `https://datasets-server.huggingface.co/splits?dataset=${encodeURIComponent(dataset)}`,
    { expectJson: true }
  ));
  const first = (splits.splits || [])[0];
  if (!first) return 0;
  const { config, split } = first;
  for (let page = 0; page < maxPages; page++) {
    const offset = page * 100;
    const data = JSON.parse(await safeFetch(
      `https://datasets-server.huggingface.co/rows?dataset=${encodeURIComponent(dataset)}&config=${encodeURIComponent(config)}&split=${encodeURIComponent(split)}&offset=${offset}&length=100`,
      { expectJson: true }
    ));
    const rows = data.rows || [];
    if (!rows.length) break;
    for (const item of rows) {
      const r = item.row || {};
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
    if (rows.length < 100) break;
  }
  return added;
}

async function pullCommonCrawl(add) {
  const colls = JSON.parse(await safeFetch('https://index.commoncrawl.org/collinfo.json', { expectJson: true }));
  const latest = (colls || [])[0];
  if (!latest || !latest['cdx-api']) return 0;
  const cdx = assertSafeAllowlistedUrl(latest['cdx-api']);
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

(async () => {
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

  let newThisRun = 0;
  const add = (rawAts, rawToken, rawCompany) => {
    if (newThisRun >= MAX_NEW_BOARDS_PER_RUN) return false;
    const ats = rawAts.toLowerCase().trim();
    if (!seen[ats]) return false;
    const tok = rawToken.toLowerCase().trim();
    if (!TOKEN_RE.test(tok) || RESERVED_TOKENS.has(tok) || curated[ats].has(tok)) return false;
    const comp = sanitizeCompany(rawCompany, tok);
    if (seen[ats].has(tok)) {
      if (rawCompany && seen[ats].get(tok) === titleFromSlug(tok)) seen[ats].set(tok, comp);
      return false;
    }
    seen[ats].set(tok, comp);
    newThisRun++;
    return true;
  };

  const today = new Date().toISOString().slice(0, 10);
  const runners = [
    ['hf_open_apply_jobs', () => pullHuggingFaceDataset('edwarddgao/open-apply-jobs', add, 10)],
    ['hf_storesignal_ats_boards', () => pullHuggingFaceDataset('storesignalhq/ats-job-boards', add, 10)],
    ...(SKIP_CC ? [] : [['common_crawl_cdx', () => pullCommonCrawl(add)]]),
  ];

  let anySucceeded = false;
  for (const [id, fn] of runners) {
    const src = (catalog.sources || []).find((s) => s.id === id);
    try {
      const n = await fn();
      anySucceeded = true;
      if (src) {
        src.updated_at = today;
        src.boards_added = n;
      }
      console.log(`source ${id}: +${n} new boards`);
    } catch (err) {
      console.warn(`source ${id} skipped: ${err.message}`);
    }
  }

  catalog.boards = {};
  for (const ats of ['greenhouse', 'lever', 'ashby']) {
    catalog.boards[ats] = [...seen[ats].entries()]
      .sort((a, b) => a[0].localeCompare(b[0]))
      .map(([token, company]) => ({ token, company }));
  }
  if (anySucceeded) {
    catalog.updated_at = today;
    catalog.refresh_days = 30;
  }
  fs.writeFileSync(OUT_PATH, JSON.stringify(catalog, null, 2) + '\n', 'utf8');
  const ghN = catalog.boards.greenhouse.length;
  const lvN = catalog.boards.lever.length;
  const asN = catalog.boards.ashby.length;
  console.log(`Saved public_boards.json: greenhouse=${ghN}, lever=${lvN}, ashby=${asN} (total=${ghN + lvN + asN})`);
})();

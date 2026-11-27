#!/usr/bin/env node
// Security & schema audit for community-submitted boards, listings, and public ATS datasets.
// Runs in CI on PRs touching data/community-*.json or backend/internal/connectors/public_boards.json.
const fs = require('node:fs');
const path = require('node:path');
const {
  addDays,
  daysBetween,
  extractCloseDate,
  isLongRunning,
  parseIsoDate,
} = require('./prune-community-listings.js');

const REPO_ROOT = path.join(__dirname, '..');
const args = process.argv.slice(2);
let ROOT = REPO_ROOT;
for (let i = 0; i < args.length; i++) {
  if (args[i] !== '--root' || !args[i + 1] || args[i + 1].startsWith('--')) {
    throw new Error(`Invalid argument: ${args[i]}`);
  }
  ROOT = path.resolve(args[++i]);
}
const TOKEN_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;
const ATS_SET = new Set(['greenhouse', 'lever', 'ashby']);
const INVISIBLE_RE = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f\u200b-\u200f\u202a-\u202e\u2060-\u206f\ufeff]|[\u{E0000}-\u{E007F}]/u;
const SHORTENERS = new Set([
  'bit.ly', 'tinyurl.com', 't.co', 'ow.ly', 'goo.gl', 'is.gd', 'buff.ly',
  'rebrand.ly', 'cutt.ly', 'rb.gy', 'shorturl.at', 'tiny.cc', 'lnkd.in',
]);
const RESERVED = new Set([
  'embed', 'jobs', 'apply', 'api', 'v1', 'v0', 'assets', 'static', 'favicon', 'robots',
  'sitemap', 'login', 'signup', 'auth', 'cdn', 'public', 'app', 'www', 'boards', 'careers',
]);

const errors = [];

function editDistance(a, b) {
  if (Math.abs(a.length - b.length) > 1) return 2;
  const prev = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 1; i <= a.length; i++) {
    let diag = prev[0];
    prev[0] = i;
    for (let j = 1; j <= b.length; j++) {
      const tmp = prev[j];
      prev[j] = Math.min(prev[j] + 1, prev[j - 1] + 1, diag + (a[i - 1] === b[j - 1] ? 0 : 1));
      diag = tmp;
    }
  }
  return prev[b.length];
}

function checkHttpsUrl(raw, label) {
  if (!raw) return;
  let u;
  try {
    u = new URL(raw);
  } catch {
    errors.push(`${label}: invalid URL ${JSON.stringify(raw)}`);
    return;
  }
  if (u.protocol !== 'https:') errors.push(`${label}: non-HTTPS URL ${raw}`);
  if (u.username || u.password) errors.push(`${label}: URL contains credentials`);
  const host = u.hostname.toLowerCase().replace(/\.$/, '');
  if (!host.includes('.') || host.includes('xn--')) errors.push(`${label}: suspicious/punycode host ${host}`);
  if (/^(\d+\.){3}\d+$/.test(host) || host.startsWith('[') || host === 'localhost' || host.endsWith('.local') || host.endsWith('.internal')) {
    errors.push(`${label}: IP or local host not allowed (${host})`);
  }
  if (SHORTENERS.has(host.replace(/^www\./, ''))) errors.push(`${label}: URL shortener not allowed (${host})`);
}

function loadCuratedTokens(root) {
  const curated = { greenhouse: new Set(), lever: new Set(), ashby: new Set() };
  try {
    const gh = JSON.parse(fs.readFileSync(path.join(root, 'backend', 'internal', 'connectors', 'greenhouse', 'boards.json'), 'utf8'));
    for (const c of gh.categories || []) for (const b of c.boards || []) curated.greenhouse.add(b.token.toLowerCase());
  } catch {}
  for (const [ats, rel] of [
    ['lever', 'backend/internal/connectors/lever/boards.go'],
    ['ashby', 'backend/internal/connectors/ashby/boards.go'],
  ]) {
    try {
      const src = fs.readFileSync(path.join(root, rel), 'utf8');
      for (const m of src.matchAll(/\{"([^"]+)",\s*"([^"]+)"\}/g)) curated[ats].add(m[1].toLowerCase());
    } catch {}
  }
  return curated;
}

// PR checkouts are untrusted. Never follow a symlink or read a non-regular file.
function readCatalog(relativePath, field) {
  const root = fs.realpathSync(ROOT);
  let file = root;
  for (const segment of relativePath.split('/')) {
    file = path.join(file, segment);
    const stat = fs.lstatSync(file, { throwIfNoEntry: false });
    if (!stat) return null;
    if (stat.isSymbolicLink()) throw new Error(`${relativePath}: symlinks are not allowed`);
  }
  const stat = fs.statSync(file);
  if (!stat.isFile() || stat.size > 4 * 1024 * 1024) {
    throw new Error(`${relativePath}: expected a regular JSON file up to 4 MiB`);
  }
  const data = JSON.parse(fs.readFileSync(file, 'utf8'));
  if (data?.version !== 1 || !field(data)) throw new Error(`${relativePath}: invalid catalog schema`);
  return data;
}

const curated = loadCuratedTokens(REPO_ROOT);

// 1. Audit data/community-boards.json
const boardsCatalog = readCatalog('data/community-boards.json', value => Array.isArray(value.boards));
if (boardsCatalog) {
  const data = boardsCatalog;
  const seen = new Set();
  for (const b of data.boards || []) {
    const ats = String(b.ats || '').toLowerCase();
    const tok = String(b.token || '').toLowerCase();
    if (!ATS_SET.has(ats)) errors.push(`community-boards: unsupported ATS ${JSON.stringify(b.ats)}`);
    if (!TOKEN_RE.test(tok) || RESERVED.has(tok)) errors.push(`community-boards: invalid/reserved token ${JSON.stringify(b.token)}`);
    if (INVISIBLE_RE.test(String(b.company || ''))) errors.push(`community-boards/${tok}: invisible chars in company name`);
    const key = `${ats}:${tok}`;
    if (seen.has(key)) errors.push(`community-boards: duplicate ${key}`);
    seen.add(key);
    if (tok.length >= 5 && curated[ats]) {
      for (const known of curated[ats]) {
        if (known !== tok && known.length >= 5 && editDistance(tok, known) === 1) {
          errors.push(`community-boards: possible typosquat ${key} (edit distance 1 from curated ${ats}:${known})`);
          break;
        }
      }
    }
  }
}

// 2. Audit data/community-listings.json
const listingsCatalog = readCatalog('data/community-listings.json', value => Array.isArray(value.listings));
if (listingsCatalog) {
  const data = listingsCatalog;
  const today = new Date().toISOString().slice(0, 10);
  for (const l of data.listings || []) {
    const label = `community-listings/${l.id || l.company_name || 'unknown'}`;
    for (const field of ['source', 'company_name', 'position_title', 'location', 'raw_text']) {
      if (INVISIBLE_RE.test(String(l[field] || ''))) {
        errors.push(`${label}: invisible/bidi/tag characters found in ${field}`);
      }
    }
    checkHttpsUrl(l.listing_url, `${label}.listing_url`);
    checkHttpsUrl(l.company_url, `${label}.company_url`);
    const added = parseIsoDate(l.added) ? String(l.added).trim() : '';
    const expires = parseIsoDate(l.expires_at) ? String(l.expires_at).trim() : '';
    if (added && added > addDays(today, 2)) errors.push(`${label}: added date is in the future`);
    if (!added) {
      errors.push(`${label}: missing or invalid YYYY-MM-DD added date`);
    }
    if (!expires) {
      errors.push(`${label}: missing or invalid YYYY-MM-DD expires_at timeframe`);
    }
    const explicitClose = parseIsoDate(l.closes_at) ? String(l.closes_at).trim() : '';
    if (l.closes_at) {
      if (!explicitClose) {
        errors.push(`${label}: invalid YYYY-MM-DD closes_at date (${l.closes_at})`);
      }
    }
    const closeDate = [explicitClose, extractCloseDate(l.raw_text)].filter(Boolean).sort()[0] || '';
    if (closeDate && closeDate < today) {
      errors.push(`${label}: closes_at (${closeDate}) has already elapsed`);
    }

    if (l.evergreen === true && !isLongRunning(l)) {
      errors.push(`${label}: evergreen flag is not supported by the listing content`);
    }

    if (added && expires) {
      const evergreen = isLongRunning(l);
      const ttlExpiry = addDays(added, evergreen ? 120 : 45);
      const policyExpiry = closeDate && closeDate < ttlExpiry ? closeDate : ttlExpiry;
      const spanDays = daysBetween(added, expires);
      if (spanDays < 0 || (policyExpiry && expires > policyExpiry)) {
        errors.push(`${label}: expires_at (${expires}) exceeds the allowed timeframe from added (${added})`);
      }
    }
  }
}

// 3. Audit backend/internal/connectors/public_boards.json
const publicCatalog = readCatalog('backend/internal/connectors/public_boards.json', value => value.boards && typeof value.boards === 'object' && !Array.isArray(value.boards) && Object.entries(value.boards).every(([ats, boards]) => ATS_SET.has(ats) && Array.isArray(boards)));
if (publicCatalog) {
  const data = publicCatalog;
  for (const ats of ['greenhouse', 'lever', 'ashby']) {
    const seen = new Set();
    for (const b of data.boards?.[ats] || []) {
      const tok = String(b.token || '').toLowerCase();
      if (!TOKEN_RE.test(tok) || RESERVED.has(tok)) errors.push(`public_boards/${ats}: invalid token ${JSON.stringify(b.token)}`);
      if (INVISIBLE_RE.test(String(b.company || ''))) errors.push(`public_boards/${ats}/${tok}: invisible chars in company`);
      if (seen.has(tok)) errors.push(`public_boards/${ats}: duplicate token ${tok}`);
      seen.add(tok);
    }
  }
}

if (errors.length) {
  console.error('Security audit failed:\n' + errors.map((e) => `  - ${e}`).join('\n'));
  process.exit(1);
}
console.log('ok: community boards, listings, and public ATS board catalogs passed security checks.');

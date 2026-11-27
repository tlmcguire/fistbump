#!/usr/bin/env node
// Prunes outdated, expired, or dateless listings from data/community-listings.json.
//
// Pruning rules (no listing can escape without a timeframe):
//   1. Mandatory timeframe: listings missing a valid YYYY-MM-DD `added` date (and no valid future
//      `closes_at`/`expires_at`) are pruned immediately.
//   2. Explicit close date: listings whose `closes_at` field OR extracted deadline in `raw_text`
//      has elapsed (< today) are pruned immediately.
//   3. Safe live ATS check (online mode only): for allowlisted ATS hosts (Greenhouse, Lever, Ashby),
//      fetches with redirect:"manual" + HTTPS/host-allowlist validation. 404/410 -> pruned;
//      200 OK -> protected from standard 45-day pruning (up to the 180-day hard ceiling).
//   4. Age-based TTL with long-running protection:
//      - Standard listings (LinkedIn, Indeed, Workday, etc.): 45 days from `added`.
//      - Long-running / evergreen / rolling listings: 120 days from `added`.
//      - A submitted `expires_at` may shorten, but never extend, the calculated timeframe.
//      - Hard ceiling for ALL listings: 180 days from `added`.
//
// Usage:
//   node scripts/prune-community-listings.js [--dry-run] [--offline]
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const ROOT = path.join(__dirname, '..');
const LISTINGS_FILE = path.join(ROOT, 'data', 'community-listings.json');

const STANDARD_TTL_DAYS = 45;
const EVERGREEN_TTL_DAYS = 120;
const HARD_MAX_AGE_DAYS = 180;
const ISO_DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

// Only allowlisted, institutional ATS hosts are ever probed by GitHub Actions.
const ALLOWED_ATS_HOSTS = new Set([
  'boards.greenhouse.io',
  'job-boards.greenhouse.io',
  'boards.eu.greenhouse.io',
  'job-boards.eu.greenhouse.io',
  'jobs.lever.co',
  'jobs.eu.lever.co',
  'jobs.ashbyhq.com',
]);

const EVERGREEN_RE = /\b(evergreen|rolling\s+basis|rolling\s+applications?|accepted\s+on\s+a\s+rolling|continuous\s+recruitment|ongoing\s+recruitment|open\s+until\s+filled|always\s+hiring|general\s+application|talent\s+pool|pipeline\s+role)\b/i;

const MONTHS = {
  jan: 1, january: 1, feb: 2, february: 2, mar: 3, march: 3, apr: 4, april: 4,
  may: 5, jun: 6, june: 6, jul: 7, july: 7, aug: 8, august: 8,
  sep: 9, sept: 9, september: 9, oct: 10, october: 10, nov: 11, november: 11, dec: 12, december: 12,
};

function parseIsoDate(s) {
  if (!ISO_DATE_RE.test(String(s || '').trim())) return null;
  const [y, m, d] = String(s).trim().split('-').map(Number);
  const dt = new Date(Date.UTC(y, m - 1, d));
  if (dt.getUTCFullYear() !== y || dt.getUTCMonth() !== m - 1 || dt.getUTCDate() !== d) return null;
  return dt;
}

function formatIsoDate(dt) {
  return dt.toISOString().slice(0, 10);
}

function addDays(isoDate, days) {
  const dt = parseIsoDate(isoDate);
  if (!dt) return '';
  dt.setUTCDate(dt.getUTCDate() + days);
  return formatIsoDate(dt);
}

function daysBetween(fromIso, toIso) {
  const a = parseIsoDate(fromIso);
  const b = parseIsoDate(toIso);
  if (!a || !b) return NaN;
  return Math.floor((b.getTime() - a.getTime()) / 86400000);
}

// Extracts an explicit close/deadline date (YYYY-MM-DD) from listing text if present.
function extractCloseDate(text) {
  const src = String(text || '');
  const cue = '(?:application\\s+deadline|apply\\s+by|closes?(?:\\s+on)?|closing\\s+date|deadline|accepting\\s+applications\\s+(?:through|until)|applications?\\s+close(?:\\s+on)?)';
  const isoMatch = src.match(new RegExp(`${cue}[^\\n]{0,24}?(\\d{4}-\\d{2}-\\d{2})`, 'i'));
  if (isoMatch && parseIsoDate(isoMatch[1])) return isoMatch[1];

  const slashMatch = src.match(new RegExp(`${cue}[^\\n]{0,24}?(\\d{1,2})\\/(\\d{1,2})\\/(\\d{4})`, 'i'));
  if (slashMatch) {
    const iso = `${slashMatch[3]}-${String(slashMatch[1]).padStart(2, '0')}-${String(slashMatch[2]).padStart(2, '0')}`;
    if (parseIsoDate(iso)) return iso;
  }

  const wordMatch = src.match(new RegExp(`${cue}[^\\n]{0,24}?([A-Za-z]{3,9})\\s+(\\d{1,2})(?:st|nd|rd|th)?,?\\s+(\\d{4})`, 'i'));
  if (wordMatch) {
    const m = MONTHS[wordMatch[1].toLowerCase()];
    if (m) {
      const iso = `${wordMatch[3]}-${String(m).padStart(2, '0')}-${String(wordMatch[2]).padStart(2, '0')}`;
      if (parseIsoDate(iso)) return iso;
    }
  }
  return '';
}

function isLongRunning(listing) {
  const text = `${listing?.position_title || ''}\n${listing?.raw_text || ''}`;
  return EVERGREEN_RE.test(text);
}

function isAllowlistedAtsUrl(rawUrl) {
  if (!rawUrl) return false;
  try {
    const u = new URL(rawUrl);
    return u.protocol === 'https:' && ALLOWED_ATS_HOSTS.has(u.hostname.toLowerCase());
  } catch {
    return false;
  }
}

// Safely probes an allowlisted ATS listing URL with manual redirect inspection (no cross-host jumps).
async function checkAtsListingLive(rawUrl) {
  if (!isAllowlistedAtsUrl(rawUrl)) return 'skip';
  let current = rawUrl;
  for (let hop = 0; hop <= 2; hop++) {
    try {
      const res = await fetch(current, {
        method: 'GET',
        redirect: 'manual',
        headers: { 'User-Agent': 'Fistbump-Listing-Pruner/1.0' },
        signal: AbortSignal.timeout(10000),
      });
      if (res.status >= 300 && res.status < 400) {
        const loc = res.headers.get('location');
        if (!loc) return 'unknown';
        const next = new URL(loc, current).href;
        if (!isAllowlistedAtsUrl(next)) return 'unknown';
        current = next;
        continue;
      }
      if (res.status === 404 || res.status === 410) return 'closed';
      if (res.status >= 200 && res.status < 300) return 'open';
      return 'unknown';
    } catch {
      return 'unknown';
    }
  }
  return 'unknown';
}

// Pure decision function so unit tests can verify every branch deterministically.
function evaluateListing(listing, todayIso, atsStatus = 'skip') {
  const added = parseIsoDate(listing?.added) ? String(listing.added).trim() : '';
  const explicitClose = parseIsoDate(listing?.closes_at)
    ? String(listing.closes_at).trim()
    : extractCloseDate(listing?.raw_text);
  const explicitExpires = parseIsoDate(listing?.expires_at) ? String(listing.expires_at).trim() : '';

  // 1. No listing can escape pruning by omitting a valid timeframe.
  if (!added) {
    return { keep: false, reason: 'missing_timeframe' };
  }

  const ageDays = daysBetween(added, todayIso);
  if (Number.isNaN(ageDays) || ageDays < -2) {
    return { keep: false, reason: 'invalid_date' };
  }

  // 2. Explicit close/deadline date elapsed.
  if (explicitClose && daysBetween(explicitClose, todayIso) > 0) {
    return { keep: false, reason: 'close_date_elapsed' };
  }

  // 3. Verified closed on an allowlisted ATS.
  if (atsStatus === 'closed') {
    return { keep: false, reason: 'ats_posting_closed' };
  }

  // 4. Hard maximum ceiling (180 days) applies to EVERY listing, even evergreen or live-checked.
  if (ageDays > HARD_MAX_AGE_DAYS) {
    return { keep: false, reason: 'hard_max_age_exceeded' };
  }

  const evergreen = isLongRunning(listing);
  const defaultTtl = evergreen ? EVERGREEN_TTL_DAYS : STANDARD_TTL_DAYS;
  const ttlExpiry = addDays(added, defaultTtl);
  const computedExpiry = explicitClose && explicitClose < ttlExpiry ? explicitClose : ttlExpiry;
  const effectiveExpiry = explicitExpires && explicitExpires < computedExpiry
    ? explicitExpires
    : computedExpiry;
  const updated = {
    ...listing,
    expires_at: effectiveExpiry,
  };
  if (explicitClose) updated.closes_at = explicitClose;
  else delete updated.closes_at;
  if (evergreen) updated.evergreen = true;
  else delete updated.evergreen;

  // If the ATS URL was verified actively open (200 OK), allow it past standard TTL up to hard ceiling.
  if (atsStatus === 'open') {
    return {
      keep: true,
      updated,
    };
  }

  // 5. Standard / evergreen timeframe expiration check.
  if (effectiveExpiry && daysBetween(effectiveExpiry, todayIso) > 0) {
    return { keep: false, reason: evergreen ? 'evergreen_ttl_elapsed' : 'standard_ttl_elapsed' };
  }

  return {
    keep: true,
    updated,
  };
}

async function main() {
  const args = new Set(process.argv.slice(2));
  const dryRun = args.has('--dry-run');
  const offline = args.has('--offline');
  const todayIso = new Date().toISOString().slice(0, 10);

  if (!fs.existsSync(LISTINGS_FILE)) {
    console.log('ok: data/community-listings.json not found; nothing to prune.');
    return;
  }

  const data = JSON.parse(fs.readFileSync(LISTINGS_FILE, 'utf8'));
  const listings = Array.isArray(data.listings) ? data.listings : [];
  const kept = [];
  const pruned = [];

  for (const l of listings) {
    const atsStatus = offline ? 'skip' : await checkAtsListingLive(l.listing_url);
    const verdict = evaluateListing(l, todayIso, atsStatus);
    if (verdict.keep) {
      kept.push(verdict.updated);
    } else {
      pruned.push({ id: l.id || 'unknown', company: l.company_name || 'unknown', reason: verdict.reason });
    }
  }

  const changed = pruned.length > 0 || JSON.stringify(kept) !== JSON.stringify(listings);
  if (!dryRun && changed) {
    const out = {
      version: data.version || 1,
      updated: todayIso,
      listings: kept,
    };
    fs.writeFileSync(LISTINGS_FILE, `${JSON.stringify(out, null, 2)}\n`, 'utf8');
  }

  console.log(`ok${dryRun ? ' (dry-run)' : ''}: kept=${kept.length}, pruned=${pruned.length}`);
  for (const p of pruned) {
    console.log(`  pruned ${p.id} (${p.company}): ${p.reason}`);
  }
}

if (require.main === module) {
  main().catch((err) => {
    console.error(err);
    process.exit(1);
  });
}

module.exports = {
  STANDARD_TTL_DAYS,
  EVERGREEN_TTL_DAYS,
  HARD_MAX_AGE_DAYS,
  ALLOWED_ATS_HOSTS,
  parseIsoDate,
  addDays,
  daysBetween,
  extractCloseDate,
  isLongRunning,
  isAllowlistedAtsUrl,
  evaluateListing,
};

#!/usr/bin/env node
// Refreshes backend/internal/cost/sources.json on a 30-day cadence by verifying each cited
// public data source URL and updating the verification timestamp.
//
// Security hardening:
//   - Enforces HTTPS-only and validates every redirect hop stays on the same base domain
//     (so a sold/expired domain redirecting to an unrelated malicious site fails verification).
const fs = require('node:fs');
const path = require('node:path');

const filePath = path.join(__dirname, '..', 'backend', 'internal', 'cost', 'sources.json');
const catalog = JSON.parse(fs.readFileSync(filePath, 'utf8'));
const today = new Date().toISOString().slice(0, 10);

function baseDomain(hostname) {
  const parts = hostname.toLowerCase().replace(/^www\./, '').split('.');
  return parts.length >= 2 ? parts.slice(-2).join('.') : hostname.toLowerCase();
}

async function verify(rawUrl) {
  let current;
  let allowedBase;
  try {
    const u = new URL(rawUrl);
    if (u.protocol !== 'https:') return false;
    current = u.toString();
    allowedBase = baseDomain(u.hostname);
  } catch {
    return false;
  }

  for (const method of ['HEAD', 'GET']) {
    let url = current;
    let ok = false;
    for (let hop = 0; hop <= 3; hop++) {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), 15000);
      try {
        const res = await fetch(url, { method, redirect: 'manual', signal: controller.signal });
        if (res.status >= 300 && res.status < 400) {
          const loc = res.headers.get('location');
          if (!loc) break;
          const next = new URL(loc, url);
          if (next.protocol !== 'https:' || baseDomain(next.hostname) !== allowedBase) {
            console.warn(`Security warning: blocked cross-domain/non-HTTPS redirect from ${url} -> ${next.toString()}`);
            return false;
          }
          url = next.toString();
          continue;
        }
        if (res.ok || (method === 'HEAD' && (res.status === 405 || res.status === 403))) {
          ok = true;
        }
        break;
      } catch {
        break;
      } finally {
        clearTimeout(timer);
      }
    }
    if (ok) return true;
  }
  return false;
}

(async () => {
  let verified = 0;
  for (const src of catalog.sources) {
    const ok = await verify(src.url);
    if (ok) {
      src.updated_at = today;
      verified++;
    } else {
      console.warn(`Warning: could not verify ${src.name} (${src.url}); keeping previous timestamp ${src.updated_at}`);
    }
  }
  if (verified > 0) {
    catalog.updated_at = today;
    catalog.refresh_days = 30;
    fs.writeFileSync(filePath, JSON.stringify(catalog, null, 2) + '\n', 'utf8');
    console.log(`Updated ${verified}/${catalog.sources.length} cost data sources (${today}).`);
  } else {
    console.error('No sources could be verified.');
    process.exitCode = 1;
  }
})();

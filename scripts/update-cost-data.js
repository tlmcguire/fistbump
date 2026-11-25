#!/usr/bin/env node
// Refreshes backend/internal/cost/sources.json on a 30-day cadence by verifying each cited
// public data source URL and updating the verification timestamp.
const fs = require('node:fs');
const path = require('node:path');

const filePath = path.join(__dirname, '..', 'backend', 'internal', 'cost', 'sources.json');
const catalog = JSON.parse(fs.readFileSync(filePath, 'utf8'));
const today = new Date().toISOString().slice(0, 10);

async function verify(url) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 15000);
  try {
    const res = await fetch(url, {method: 'HEAD', redirect: 'follow', signal: controller.signal});
    if (res.ok || res.status === 405 || res.status === 403) return true;
    const getRes = await fetch(url, {method: 'GET', redirect: 'follow', signal: controller.signal});
    return getRes.ok;
  } catch {
    return false;
  } finally {
    clearTimeout(timer);
  }
}

(async () => {
  let verified = 0;
  for (const src of catalog.sources) {
    const ok = await verify(src.url);
    if (ok) {
      src.updated_at = today;
      verified++;
    } else {
      console.warn(`Warning: could not reach ${src.name} (${src.url}); keeping previous timestamp ${src.updated_at}`);
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

#!/usr/bin/env node
// Checks backend/internal/connectors/greenhouse/boards.json.
//   node scripts/verify-greenhouse-boards.js            structure checks and a live check of every token
//   node scripts/verify-greenhouse-boards.js --offline  structure checks only
const fs = require('fs');
const path = require('path');

const file = path.join(__dirname, '..', 'backend', 'internal', 'connectors', 'greenhouse', 'boards.json');
const API = 'https://boards-api.greenhouse.io/v1/boards';
const offline = process.argv.includes('--offline');

function structureErrors(data) {
  const errs = [];
  if (data.version !== 1) errs.push(`unsupported version ${data.version}`);
  const ids = new Set();
  for (const c of data.categories || []) {
    if (!c.id || !c.name) errs.push(`category missing id or name: ${JSON.stringify(c)}`);
    if (ids.has(c.id)) errs.push(`duplicate category id ${c.id}`);
    ids.add(c.id);
    if (c.parent && c.parent === c.id) errs.push(`${c.id}: parent is itself`);
    const tokens = new Set();
    for (const b of c.boards || []) {
      if (!b.token || !/^[a-z0-9_-]+$/i.test(b.token)) errs.push(`${c.id}: invalid token ${JSON.stringify(b.token)}`);
      if (!b.company) errs.push(`${c.id}/${b.token}: missing company`);
      if (tokens.has(b.token)) errs.push(`${c.id}: duplicate token ${b.token}`);
      tokens.add(b.token);
    }
    if (!(c.boards || []).length) errs.push(`${c.id}: no boards`);
  }
  for (const c of data.categories || []) {
    if (c.parent && !ids.has(c.parent)) errs.push(`${c.id}: unknown parent ${c.parent}`);
    if (c.parent && (data.categories.find((p) => p.id === c.parent) || {}).parent) errs.push(`${c.id}: parent ${c.parent} is itself a subset`);
  }
  return errs;
}

async function liveErrors(data) {
  const tokens = [...new Set(data.categories.flatMap((c) => c.boards.map((b) => b.token)))];
  const errs = [];
  let next = 0;
  async function worker() {
    while (next < tokens.length) {
      const token = tokens[next++];
      try {
        const res = await fetch(`${API}/${token}/jobs`, { signal: AbortSignal.timeout(20000) });
        if (!res.ok) { errs.push(`${token}: HTTP ${res.status}`); continue; }
        const body = await res.json();
        if (!body.jobs || body.jobs.length === 0) errs.push(`${token}: no open postings`);
      } catch (e) {
        errs.push(`${token}: ${e.message}`);
      }
    }
  }
  await Promise.all(Array.from({ length: 8 }, worker));
  return errs.sort();
}

(async () => {
  const data = JSON.parse(fs.readFileSync(file, 'utf8'));
  const errs = structureErrors(data);
  if (!offline && !errs.length) errs.push(...(await liveErrors(data)));
  if (errs.length) {
    console.error(errs.join('\n'));
    process.exit(1);
  }
  const n = new Set(data.categories.flatMap((c) => c.boards.map((b) => b.token))).size;
  console.log(`ok: ${data.categories.length} categories, ${n} unique boards${offline ? ' (offline)' : ''}`);
})();

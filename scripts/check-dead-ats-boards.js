#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const ROOT = path.join(__dirname, '..');
const TOKEN_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;
const ATS_TYPES = ['greenhouse', 'lever', 'ashby'];
const CONCURRENCY = 8;
// Rotate a deterministic slice each day to spread requests across a weekly full scan.
const SHARD_COUNT = 7;
const TIMEOUT_MS = 10000;

function addBoard(boards, ats, token, company, source) {
  const normalizedToken = String(token || '').toLowerCase();
  if (!TOKEN_RE.test(normalizedToken)) {
    throw new Error(`Invalid ${ats} board token in ${source}: ${JSON.stringify(token)}`);
  }
  const key = `${ats}:${normalizedToken}`;
  const current = boards.get(key) || {
    ats,
    token: normalizedToken,
    company: String(company || normalizedToken),
    sources: [],
  };
  if (!current.sources.includes(source)) current.sources.push(source);
  boards.set(key, current);
}

function loadBoards(root = ROOT) {
  const boards = new Map();
  const greenhouseFile = path.join(root, 'backend/internal/connectors/greenhouse/boards.json');
  const greenhouse = JSON.parse(fs.readFileSync(greenhouseFile, 'utf8'));
  for (const category of greenhouse.categories || []) {
    for (const board of category.boards || []) {
      addBoard(boards, 'greenhouse', board.token, board.company, `curated:${category.id}`);
    }
  }

  for (const [ats, relativePath] of [
    ['greenhouse', 'backend/internal/connectors/greenhouse/boards.go'],
    ['lever', 'backend/internal/connectors/lever/boards.go'],
    ['ashby', 'backend/internal/connectors/ashby/boards.go'],
  ]) {
    const source = fs.readFileSync(path.join(root, relativePath), 'utf8');
    const pattern = ats === 'greenhouse'
      ? /Token:\s*"([^"]+)"\s*,\s*Company:\s*"([^"]+)"/g
      : /\{"([^"]+)",\s*"([^"]+)"\}/g;
    for (const match of source.matchAll(pattern)) {
      addBoard(boards, ats, match[1], match[2], `curated:${ats}`);
    }
  }

  const publicFile = path.join(root, 'backend/internal/connectors/public_boards.json');
  const publicData = JSON.parse(fs.readFileSync(publicFile, 'utf8'));
  for (const ats of ATS_TYPES) {
    for (const board of publicData.boards?.[ats] || []) {
      addBoard(boards, ats, board.token, board.company, 'public');
    }
  }

  const communityFile = path.join(root, 'data/community-boards.json');
  const communityData = JSON.parse(fs.readFileSync(communityFile, 'utf8'));
  for (const board of communityData.boards || []) {
    if (ATS_TYPES.includes(board.ats)) {
      addBoard(boards, board.ats, board.token, board.company, 'community');
    }
  }

  return [...boards.values()]
    .map((board) => ({ ...board, sources: board.sources.sort() }))
    .sort((a, b) => a.ats.localeCompare(b.ats) || a.token.localeCompare(b.token));
}

function boardUrl({ ats, token }) {
  const encoded = encodeURIComponent(token);
  if (ats === 'greenhouse') return `https://boards-api.greenhouse.io/v1/boards/${encoded}/jobs`;
  if (ats === 'lever') return `https://api.lever.co/v0/postings/${encoded}?mode=json&limit=1`;
  if (ats === 'ashby') return `https://api.ashbyhq.com/posting-api/job-board/${encoded}`;
  throw new Error(`Unsupported ATS: ${ats}`);
}

async function checkBoard(board, fetchImpl = globalThis.fetch) {
  try {
    const response = await fetchImpl(boardUrl(board), {
      headers: { 'User-Agent': 'Fistbump-ATS-Liveness/1.0', Accept: 'application/json' },
      redirect: 'error',
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
    const result = response.status === 404 || response.status === 410
      ? { status: 'dead', reason: `HTTP ${response.status}` }
      : response.status >= 200 && response.status < 300
        ? { status: 'live', reason: `HTTP ${response.status}` }
        : { status: 'unknown', reason: `HTTP ${response.status}` };
    if (response.body) await response.body.cancel();
    return { ...board, ...result };
  } catch (error) {
    return { ...board, status: 'unknown', reason: error.message };
  }
}

async function checkBoards(boards, fetchImpl = globalThis.fetch) {
  const results = new Array(boards.length);
  let next = 0;
  async function worker() {
    while (true) {
      const index = next++;
      if (index >= boards.length) return;
      results[index] = await checkBoard(boards[index], fetchImpl);
    }
  }
  await Promise.all(Array.from({ length: Math.min(CONCURRENCY, boards.length) }, worker));
  return results;
}

function shardForBoard(board) {
  let hash = 2166136261;
  for (const char of `${board.ats}:${board.token}`) {
    hash = Math.imul(hash ^ char.charCodeAt(0), 16777619) >>> 0;
  }
  return hash % SHARD_COUNT;
}

function boardsForShard(boards, shard) {
  if (!Number.isInteger(shard) || shard < 0 || shard >= SHARD_COUNT) {
    throw new Error(`Shard must be an integer from 0 to ${SHARD_COUNT - 1}`);
  }
  return boards.filter((board) => shardForBoard(board) === shard);
}

function markdownCell(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\|/g, '\\|')
    .replace(/[\r\n]/g, ' ');
}

function writeJobSummary(results, { shard, total }) {
  const dead = results.filter((result) => result.status === 'dead');
  const unknown = results.filter((result) => result.status === 'unknown');
  const lines = [
    '## ATS board liveness check',
    '',
    `Shard ${shard + 1}/${SHARD_COUNT}: checked ${results.length} of ${total}; dead (HTTP 404/410): ${dead.length}; unknown: ${unknown.length}.`,
    '',
    'Boards with no current postings are retained when their ATS endpoint still responds successfully.',
  ];
  if (dead.length) {
    lines.push('', '| ATS | Token | Company | Sources | Response |', '|---|---|---|---|---|');
    for (const board of dead) {
      lines.push(`| ${board.ats} | ${board.token} | ${markdownCell(board.company)} | ${board.sources.join(', ')} | ${board.reason} |`);
    }
  }
  if (unknown.length) {
    lines.push('', 'Unknown responses are not treated as dead and were left unchanged.');
  }
  if (process.env.GITHUB_STEP_SUMMARY) {
    fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${lines.join('\n')}\n`, 'utf8');
  }
  return { dead, unknown };
}

async function main() {
  const args = process.argv.slice(2);
  if (args.some((arg) => arg !== '--offline')) throw new Error(`Unsupported argument: ${args.find((arg) => arg !== '--offline')}`);

  const allBoards = loadBoards();
  const shard = new Date().getUTCDay();
  const boards = boardsForShard(allBoards, shard);
  if (args.includes('--offline')) {
    console.log(`ok (offline): ${allBoards.length} unique ATS boards loaded; shard ${shard + 1}/${SHARD_COUNT} contains ${boards.length}`);
    return;
  }

  const results = await checkBoards(boards);
  const { dead, unknown } = writeJobSummary(results, { shard, total: allBoards.length });
  console.log(`checked ${results.length}/${allBoards.length} ATS boards (shard ${shard + 1}/${SHARD_COUNT}): dead=${dead.length}, unknown=${unknown.length}`);
  for (const board of dead) {
    console.error(`${board.ats}:${board.token} (${board.company}) — ${board.reason}`);
  }
  if (unknown.length) {
    console.warn(`${unknown.length} boards could not be classified; no action was taken for them.`);
  }
  if (dead.length) process.exitCode = 1;
}

if (require.main === module) {
  main().catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
}

module.exports = {
  addBoard,
  boardUrl,
  boardsForShard,
  checkBoard,
  checkBoards,
  loadBoards,
  shardForBoard,
  writeJobSummary,
};

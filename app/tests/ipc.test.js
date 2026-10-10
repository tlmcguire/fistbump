// Run with: npm test (node --test). No Electron needed: these check pure routing tables.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const vm = require('vm');

// ipc-handlers requires ./secrets which requires electron. Stub it for the pure parts.
const Module = require('module');
const origLoad = Module._load;
Module._load = function (request, ...rest) {
  if (request === 'electron') return { app: {}, safeStorage: {} };
  return origLoad.call(this, request, ...rest);
};
const ipc = require('../main/ipc-handlers');
Module._load = origLoad;

function preloadChannels() {
  const src = fs.readFileSync(path.join(__dirname, '..', 'preload', 'preload.js'), 'utf8');
  const literal = src.match(/const CHANNELS = (\{[\s\S]*?\n\});/)[1];
  const channels = vm.runInNewContext(`(${literal})`);
  return Object.entries(channels).flatMap(([g, ms]) => ms.map((m) => `${g}:${m}`));
}

test('preload channels match main handlers exactly', () => {
  assert.deepEqual(preloadChannels().sort(), ipc.allChannels().sort());
});

test('route builders produce method, path and body', () => {
  assert.deepEqual(ipc.routes['jobs:analyze'](7), ['POST', '/v1/jobs/7/analyze']);
  assert.deepEqual(ipc.routes['revisions:edit'](1, 2, 'x'), ['POST', '/v1/revisions/1/suggestions/2/edit', { edited_text: 'x' }]);
  assert.deepEqual(ipc.routes['jobs:list']({ q: 'go dev', limit: 5, source: '' }), ['GET', '/v1/jobs?q=go+dev&limit=5']);
  assert.deepEqual(ipc.routes['tailored:create'](3), ['POST', '/v1/tailored-resumes', { revision_id: 3 }]);
});

test('ids are validated so the renderer cannot inject path segments', () => {
  assert.throws(() => ipc.routes['jobs:get']('1/../settings'));
  assert.throws(() => ipc.routes['jobs:get'](-1));
  assert.throws(() => ipc.routes['jobs:get'](1.5));
});

test('free-form path segments are encoded', () => {
  assert.equal(ipc.routes['connectors:removeBoard']('a/b?c')[1], '/v1/connectors/greenhouse/boards/a%2Fb%3Fc');
  assert.equal(ipc.routes['models:downloadStatus']('x y')[1], '/v1/models/downloads/x%20y');
});

test('no route can read or write secrets through generic settings', () => {
  const channels = ipc.allChannels();
  for (const c of channels) assert.ok(!/getKey|readKey|getToken/i.test(c), c);
});

test('toResult maps backend errors', () => {
  assert.deepEqual(ipc.toResult({ status: 200, json: { a: 1 } }), { ok: true, data: { a: 1 } });
  const bad = ipc.toResult({ status: 422, json: { error: { code: 'validation_failed', message: 'nope' } } });
  assert.equal(bad.ok, false);
  assert.equal(bad.error.code, 'validation_failed');
});

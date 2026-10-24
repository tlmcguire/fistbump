// Checks that every IPC channel the preload script calls has a handler, and that search can be canceled.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const { CHANNELS, register } = require('../main/ipc-handlers');

const preload = fs.readFileSync(path.join(__dirname, '..', 'preload', 'preload.js'), 'utf8');

function fakeIpc() {
  const handlers = new Map();
  return { handlers, handle: (channel, fn) => handlers.set(channel, fn) };
}

test('every channel the preload invokes is registered and listed', () => {
  const used = [...preload.matchAll(/invoke\('([^']+)'/g)].map((m) => m[1]);
  assert.ok(used.length > 0);
  const ipc = fakeIpc();
  register(ipc, { call: async () => ({}) });
  for (const channel of used) {
    assert.ok(ipc.handlers.has(channel), `${channel} has no handler`);
    assert.ok(CHANNELS.includes(channel), `${channel} is missing from CHANNELS`);
  }
  for (const channel of CHANNELS) assert.ok(ipc.handlers.has(channel), `${channel} is listed but not registered`);
});

test('a new search cancels the one before it, and cancel stops it', async () => {
  const signals = [];
  const backend = {
    call: (method, url, body, signal) => {
      signals.push(signal);
      return new Promise((resolve) => signal.addEventListener('abort', () => resolve({ error: { code: 'canceled' } })));
    },
  };
  const ipc = fakeIpc();
  register(ipc, backend);
  const search = ipc.handlers.get('connectors:search');

  const first = search({}, { keywords: ['a'] });
  const second = search({}, { keywords: ['b'] });
  assert.strictEqual((await first).error.code, 'canceled');
  assert.strictEqual(signals[0].aborted, true);
  assert.strictEqual(signals[1].aborted, false);

  ipc.handlers.get('connectors:cancel-search')();
  assert.strictEqual((await second).error.code, 'canceled');
});

test('search rejects a body that is not an object', async () => {
  const ipc = fakeIpc();
  register(ipc, { call: async () => ({}) });
  const result = await ipc.handlers.get('connectors:search')({}, [1]);
  assert.strictEqual(result.error.code, 'validation_failed');
});

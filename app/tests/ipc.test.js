// Checks that every IPC channel the preload script calls has a handler, and that search can be canceled.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const { CHANNELS, register, MAX_IMPORT_BYTES } = require('../main/ipc-handlers');

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

// A backend that records each call and answers {ok: true}.
function recorder() {
  const calls = [];
  return { calls, call: async (method, url, body) => { calls.push({ method, url, body }); return { ok: true }; } };
}

function importSetup({ picked, size = 3, bytes = Buffer.from('Ada') } = {}) {
  const backend = recorder();
  const ipc = fakeIpc();
  const deps = {
    dialog: { showOpenDialog: async () => picked || { canceled: false, filePaths: ['/Users/ada/Resume.pdf'] } },
    BrowserWindow: { fromWebContents: () => null },
    fs: { stat: async () => ({ size }), readFile: async () => bytes },
  };
  register(ipc, backend, deps);
  return { backend, handler: (channel) => ipc.handlers.get(channel) };
}

test('importFile sends the picked file as base64 with its name, never its path', async () => {
  const { backend, handler } = importSetup();
  await handler('resume:importFile')({ sender: {} });
  assert.deepStrictEqual(backend.calls, [{ method: 'POST', url: '/v1/resume/import', body: { filename: 'Resume.pdf', data_base64: 'QWRh' } }]);
});

test('importFile reports a canceled picker and refuses files it should not read', async () => {
  let { backend, handler } = importSetup({ picked: { canceled: true, filePaths: [] } });
  assert.deepStrictEqual(await handler('resume:importFile')({ sender: {} }), { canceled: true });

  ({ backend, handler } = importSetup({ size: MAX_IMPORT_BYTES + 1 }));
  assert.match((await handler('resume:importFile')({ sender: {} })).error.message, /larger than 5 MB/);

  ({ backend, handler } = importSetup({ picked: { canceled: false, filePaths: ['/tmp/cv.docx'] } }));
  assert.match((await handler('resume:importFile')({ sender: {} })).error.message, /cv\.docx is not a PDF/);
  assert.strictEqual(backend.calls.length, 0);
});

test('importData checks the name and size, then sends the dropped bytes', async () => {
  const { backend, handler } = importSetup();
  const drop = handler('resume:importData');
  assert.strictEqual((await drop({}, 'cv.png', new Uint8Array(3))).error.code, 'validation_failed');
  assert.strictEqual((await drop({}, 'cv.pdf', new Uint8Array(0))).error.code, 'validation_failed');
  assert.strictEqual((await drop({}, 'cv.pdf', new ArrayBuffer(MAX_IMPORT_BYTES + 1))).error.code, 'validation_failed');
  assert.strictEqual((await drop({}, 'cv.pdf', 'not bytes')).error.code, 'validation_failed');
  assert.strictEqual(backend.calls.length, 0);

  await drop({}, '../../cv.MD', new TextEncoder().encode('Ada').buffer);
  assert.deepStrictEqual(backend.calls[0].body, { filename: 'cv.MD', data_base64: 'QWRh' });
});

test('importText and importApply check their arguments', async () => {
  const { backend, handler } = importSetup();
  assert.strictEqual((await handler('resume:importText')({}, '   ')).error.code, 'validation_failed');
  assert.strictEqual((await handler('resume:importApply')({}, { replace: true })).error.code, 'validation_failed');
  await handler('resume:importText')({}, 'Ada Lovelace');
  await handler('resume:importApply')({}, { draft: {}, replace: false });
  assert.deepStrictEqual(backend.calls.map((c) => c.url), ['/v1/resume/import', '/v1/resume/import/apply']);
  assert.deepStrictEqual(backend.calls[0].body, { text: 'Ada Lovelace' });
});

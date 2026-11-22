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

//phase 4 test
test('import and job routes check their arguments', async () => {
  const backend = recorder();
  const ipc = fakeIpc();
  register(ipc, backend);
  const importJobs = ipc.handlers.get('connectors:import');

  assert.strictEqual((await importJobs({}, [])).error.code, 'validation_failed');
  assert.strictEqual((await importJobs({}, [{ board: 1 }])).error.code, 'validation_failed');
  assert.strictEqual((await ipc.handlers.get('jobs:get')({}, '1')).error.code, 'validation_failed');
  assert.strictEqual(backend.calls.length, 0);

  await importJobs({}, [{ board: 'backblaze', external_id: '5287532008' }]);
  assert.deepStrictEqual(backend.calls, [{
    method: 'POST',
    url: '/v1/connectors/greenhouse/import',
    body: { postings: [{ board: 'backblaze', external_id: '5287532008' }] },
  }]);
});

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

test('storage cleanup only passes known targets to the backend', async () => {
  const calls = [];
  const ipc = fakeIpc();
  register(ipc, { call: async (...args) => { calls.push(args); return { ok: true }; } });
  const cleanup = ipc.handlers.get('storage:cleanup');
  assert.strictEqual((await cleanup({}, ['everything'])).error.code, 'validation_failed');
  assert.strictEqual((await cleanup({}, [])).error.code, 'validation_failed');
  assert.strictEqual((await cleanup({}, 'cache')).error.code, 'validation_failed');
  await cleanup({}, ['cache']);
  assert.deepStrictEqual(calls, [['POST', '/v1/storage/cleanup', { targets: ['cache'] }]]);
});

test('app actions come from the main process, and say so when missing', async () => {
  const ipc = fakeIpc();
  register(ipc, { call: async () => ({}) }, { info: () => ({ version: '0.1.0' }) });
  assert.deepStrictEqual(await ipc.handlers.get('app:info')(), { version: '0.1.0' });
  assert.strictEqual((await ipc.handlers.get('app:check-updates')()).error.code, 'validation_failed');
});

//phase 5 tests
test('revision, suggestion and tailored routes check their arguments', async () => {
  const backend = recorder();
  const ipc = fakeIpc();
  register(ipc, backend);

  assert.strictEqual((await ipc.handlers.get('revisions:create')({}, {})).error.code, 'validation_failed');
  assert.strictEqual((await ipc.handlers.get('suggestions:edit')({}, 1, 2, '  ')).error.code, 'validation_failed');
  assert.strictEqual((await ipc.handlers.get('tailored:create')({}, 'x')).error.code, 'validation_failed');
  assert.strictEqual(backend.calls.length, 0);

  await ipc.handlers.get('suggestions:accept')({}, 4, 11);
  assert.deepStrictEqual(backend.calls, [{ method: 'POST', url: '/v1/revisions/4/suggestions/11/accept', body: undefined }]);
});

test('word diff rebuilds both texts', () => {
  global.window = {};
  global.document = {};
  require('../renderer/js/worddiff.js');
  const words = window.WordDiff.diff('excel python sql', 'sql excel python');
  const text = side => words.filter(w => w.op === 'eq' || w.op === side).map(w => w.text).join(' ');
  assert.strictEqual(text('del'), 'excel python sql');
  assert.strictEqual(text('add'), 'sql excel python');
});
//phase 6 tests: export
function exportSetup({ raw, picked } = {}) {
  const writes = [];
  const shown = [];
  const dialogs = [];
  const requests = [];
  const backend = {
    call: async () => ({}),
    requestRaw: async (method, url, body) => {
      requests.push({ method, url, body });
      return raw || {
        status: 200,
        headers: new Headers({ 'content-disposition': `attachment; filename="Ada Lovelace - Acme.pdf"; filename*=UTF-8''Ada%20Lovelace%20-%20Acme.pdf` }),
        bytes: new TextEncoder().encode('%PDF').buffer,
      };
    },
  };
  const ipc = fakeIpc();
  register(ipc, backend, {
    dialog: { showSaveDialog: async (...args) => { dialogs.push(args.at(-1)); return picked || { canceled: false, filePath: '/Users/ada/Documents/out.pdf' }; } },
    BrowserWindow: { fromWebContents: () => null },
    fs: { writeFile: async (file, data) => writes.push({ file, data: Buffer.from(data).toString() }) },
    shell: { showItemInFolder: (file) => shown.push(file) },
    documentsDir: () => '/Users/ada/Documents',
  });
  return { writes, shown, dialogs, requests, handler: (channel) => ipc.handlers.get(channel) };
}

test('export asks where to save, starting in Documents with the suggested name, and writes there', async () => {
  const { writes, dialogs, requests, handler } = exportSetup();
  const result = await handler('tailored:export')({ sender: {} }, 7, 'pdf');
  assert.deepStrictEqual(result, { saved: true, path: '/Users/ada/Documents/out.pdf' });
  assert.deepStrictEqual(requests, [{ method: 'POST', url: '/v1/tailored-resumes/7/export', body: { format: 'pdf' } }]);
  assert.strictEqual(dialogs[0].defaultPath, path.join('/Users/ada/Documents', 'Ada Lovelace - Acme.pdf'));
  assert.deepStrictEqual(dialogs[0].filters, [{ name: 'PDF', extensions: ['pdf'] }]);
  assert.deepStrictEqual(writes, [{ file: '/Users/ada/Documents/out.pdf', data: '%PDF' }]);
});

test('export reports a canceled dialog and writes nothing', async () => {
  const { writes, handler } = exportSetup({ picked: { canceled: true } });
  assert.deepStrictEqual(await handler('resume:export')({ sender: {} }, 'md'), { saved: false });
  assert.strictEqual(writes.length, 0);
});

test('export passes back a PDF failure and refuses bad arguments without calling the backend', async () => {
  const failure = { error: { code: 'internal', message: 'The PDF could not be made', details: { fallback: 'md' } } };
  const { writes, dialogs, requests, handler } = exportSetup({ raw: failure });
  assert.deepStrictEqual(await handler('resume:export')({ sender: {} }, 'pdf'), failure);
  assert.strictEqual(dialogs.length + writes.length, 0);

  assert.strictEqual((await handler('resume:export')({ sender: {} }, 'docx')).error.code, 'validation_failed');
  assert.strictEqual((await handler('tailored:export')({ sender: {} }, '7', 'pdf')).error.code, 'validation_failed');
  assert.strictEqual(requests.length, 1);
});

test('a suggested name can never choose the folder', async () => {
  const raw = { status: 200, headers: new Headers({ 'content-disposition': 'attachment; filename="../../evil.pdf"' }), bytes: new ArrayBuffer(1) };
  const { dialogs, handler } = exportSetup({ raw });
  await handler('resume:export')({ sender: {} }, 'pdf');
  assert.strictEqual(dialogs[0].defaultPath, path.join('/Users/ada/Documents', 'evil.pdf'));
});

test('show in folder only reveals the file the last export wrote', async () => {
  const { shown, handler } = exportSetup();
  const show = handler('shell:showItem');
  assert.strictEqual((await show({}, '/Users/ada/Documents/out.pdf')).error.code, 'validation_failed');
  await handler('resume:export')({ sender: {} }, 'pdf');
  assert.strictEqual((await show({}, '/etc/passwd')).error.code, 'validation_failed');
  assert.deepStrictEqual(await show({}, '/Users/ada/Documents/out.pdf'), { ok: true });
  assert.deepStrictEqual(shown, ['/Users/ada/Documents/out.pdf']);
});

test('requestRaw gives bytes on success and the error shape for a JSON answer', async () => {
  const { Backend } = require('../main/backend');
  const backend = new Backend({ binDir: '', dataDir: '' });
  Object.assign(backend, { port: 1, state: 'ready', token: 't' });
  const realFetch = global.fetch;
  try {
    global.fetch = async () => new Response(new Uint8Array([1, 2, 3]), { status: 200, headers: { 'content-type': 'application/pdf' } });
    const ok = await backend.requestRaw('POST', '/v1/resume/export', { format: 'pdf' });
    assert.strictEqual(ok.status, 200);
    assert.deepStrictEqual([...new Uint8Array(ok.bytes)], [1, 2, 3]);

    const body = { error: { code: 'internal', message: 'The PDF could not be made', details: { fallback: 'md' } } };
    global.fetch = async () => new Response(JSON.stringify(body), { status: 500, headers: { 'content-type': 'application/json' } });
    assert.deepStrictEqual(await backend.requestRaw('POST', '/v1/resume/export', { format: 'pdf' }), body);

    global.fetch = async () => { throw new Error('refused'); };
    assert.strictEqual((await backend.requestRaw('POST', '/v1/resume/export', {})).error.code, 'unreachable');
  } finally {
    global.fetch = realFetch;
  }
});

//phase 7 tests: applications and job lifecycle
test('application and job lifecycle routes check their arguments and build the right URLs', async () => {
  const backend = recorder();
  const ipc = fakeIpc();
  register(ipc, backend);
  const h = (channel) => ipc.handlers.get(channel);

  assert.strictEqual((await h('applications:create')({}, {})).error.code, 'validation_failed');
  assert.strictEqual((await h('applications:update')({}, 1, 'x')).error.code, 'validation_failed');
  assert.strictEqual((await h('applications:bulkStatus')({}, [], 'Applied')).error.code, 'validation_failed');
  assert.strictEqual((await h('applications:bulkStatus')({}, [1, '2'], 'Applied')).error.code, 'validation_failed');
  assert.strictEqual((await h('applications:bulkArchive')({}, [1], 'yes')).error.code, 'validation_failed');
  assert.strictEqual((await h('jobs:trash')({}, 0)).error.code, 'validation_failed');
  assert.strictEqual(backend.calls.length, 0);

  await h('applications:list')({}, { status: 'Applied', q: 'a&b', archived: 'only', other: 'x' });
  await h('applications:list')({});
  await h('applications:bulkStatus')({}, [1, 2], 'Rejected');
  await h('applications:bulkArchive')({}, [3], false);
  await h('applications:update')({}, 4, { notes: null });
  await h('jobs:archive')({}, 5, { archive_application: true });
  await h('jobs:archive')({}, 5);
  await h('jobs:restore')({}, 5);
  await h('jobs:list')({}, 'acme', { trashed: 'only' });
  await h('jobs:list')({}, '');
  assert.deepStrictEqual(backend.calls, [
    { method: 'GET', url: '/v1/applications?status=Applied&q=a%26b&archived=only', body: undefined },
    { method: 'GET', url: '/v1/applications', body: undefined },
    { method: 'POST', url: '/v1/applications/bulk-status', body: { ids: [1, 2], status: 'Rejected' } },
    { method: 'POST', url: '/v1/applications/bulk-archive', body: { ids: [3], archived: false } },
    { method: 'PUT', url: '/v1/applications/4', body: { notes: null } },
    { method: 'POST', url: '/v1/jobs/5/archive', body: { archive_application: true } },
    { method: 'POST', url: '/v1/jobs/5/archive', body: { archive_application: false } },
    { method: 'POST', url: '/v1/jobs/5/restore', body: undefined },
    { method: 'GET', url: '/v1/jobs?q=acme&trashed=only', body: undefined },
    { method: 'GET', url: '/v1/jobs', body: undefined },
  ]);
});

test('a saved tailored PDF is recorded on the application that uses it', async () => {
  const calls = [];
  const backend = {
    call: async (method, url, body) => {
      calls.push({ method, url, body });
      if (method === 'GET') return [{ id: 9, tailored_resume_id: 7 }, { id: 10, tailored_resume_id: null }];
      return { id: 9 };
    },
    requestRaw: async () => ({ status: 200, headers: new Headers(), bytes: new ArrayBuffer(1) }),
  };
  const ipc = fakeIpc();
  const shown = [];
  register(ipc, backend, {
    dialog: { showSaveDialog: async () => ({ canceled: false, filePath: '/docs/out.pdf' }) },
    fs: { writeFile: async () => {} },
    shell: { showItemInFolder: (file) => shown.push(file) },
  });
  const result = await ipc.handlers.get('tailored:export')({ sender: {} }, 7, 'pdf');
  assert.deepStrictEqual(result, { saved: true, path: '/docs/out.pdf', application_id: 9 });
  assert.deepStrictEqual(calls.at(-1), { method: 'PUT', url: '/v1/applications/9', body: { exported_pdf_path: '/docs/out.pdf' } });

  // Markdown is not recorded as the exported PDF.
  calls.length = 0;
  await ipc.handlers.get('tailored:export')({ sender: {} }, 7, 'md');
  assert.strictEqual(calls.length, 0);
});

test('show in folder also reveals a PDF an application recorded', async () => {
  const backend = { call: async () => [{ id: 1, exported_pdf_path: '/docs/Ada - Acme.pdf' }] };
  const ipc = fakeIpc();
  const shown = [];
  register(ipc, backend, { shell: { showItemInFolder: (file) => shown.push(file) } });
  const show = ipc.handlers.get('shell:showItem');
  assert.deepStrictEqual(await show({}, '/docs/Ada - Acme.pdf'), { ok: true });
  assert.strictEqual((await show({}, '/etc')).error.code, 'validation_failed');
  assert.deepStrictEqual(shown, ['/docs/Ada - Acme.pdf']);
});

test('the tracker puts steps due today or overdue first, using the local date', () => {
  global.window = {};
  require('../renderer/views/tracker/tracker.js');
  const { order, localToday } = window.Tracker;
  const today = '2026-11-22';
  const list = [
    { id: 1, next_step_date: null },
    { id: 2, next_step_date: '2026-11-30' },
    { id: 3, next_step_date: '2026-11-22' },
    { id: 4, next_step_date: '2026-11-20' },
  ];
  assert.deepStrictEqual(order(list, today).map((a) => a.id), [4, 3, 1, 2]);

  // localToday reads the computer's own clock, not UTC.
  const d = new Date();
  assert.strictEqual(localToday(), `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`);
});

test('PDF preview gives the PDF as base64 without opening a save dialog', async () => {
  const calls = [];
  const backend = {
    call: async () => ({}),
    requestRaw: async (method, url, body) => { calls.push([method, url, body]); return { bytes: Buffer.from('%PDF-1.4 test'), headers: new Map() }; },
  };
  const ipc = fakeIpc();
  register(ipc, backend, { dialog: { showSaveDialog: () => { throw new Error('no dialog'); } } });
  const mine = await ipc.handlers.get('resume:previewPdf')({});
  assert.equal(Buffer.from(mine.data_base64, 'base64').toString(), '%PDF-1.4 test');
  const tailored = await ipc.handlers.get('tailored:previewPdf')({}, 7);
  assert.ok(tailored.data_base64);
  assert.deepEqual(calls[1], ['POST', '/v1/tailored-resumes/7/export', { format: 'pdf' }]);
  assert.ok((await ipc.handlers.get('tailored:previewPdf')({}, '7/../x')).error);
});

test('AI settings check their arguments, and the remote key is stored by main and never sent back', async () => {
  const calls = [];
  const backend = { call: async (method, url, body) => { calls.push([method, url, body]); return method === 'DELETE' ? { ok: true } : { remote: { configured: true } }; } };
  const stored = [];
  const secrets = { set: (k) => { stored.push(k); return true; }, clear: () => stored.push('cleared') };
  const ipc = fakeIpc();
  register(ipc, backend, { secrets });

  assert.ok((await ipc.handlers.get('ai:setMode')({}, 'cloud')).error);
  await ipc.handlers.get('ai:setMode')({}, 'rules');
  assert.deepEqual(calls.at(-1), ['PUT', '/v1/ai/mode', { mode: 'rules' }]);
  assert.ok((await ipc.handlers.get('ai:test')({}, 'rules')).error);

  assert.ok((await ipc.handlers.get('ai:setRemote')({}, { base_url: '  ' })).error);
  const saved = await ipc.handlers.get('ai:setRemote')({}, { base_url: ' https://api.example.com/v1 ', model: 'm', api_key: ' sk-1 ' });
  assert.deepEqual(calls.at(-1), ['PUT', '/v1/ai/remote', { base_url: 'https://api.example.com/v1', model: 'm', api_key: 'sk-1' }]);
  assert.equal(saved.key_remembered, true);
  assert.deepEqual(stored, ['sk-1']);
  assert.ok(!JSON.stringify(saved).includes('sk-1'));

  await ipc.handlers.get('ai:clearRemote')({});
  assert.deepEqual(stored, ['sk-1', 'cleared']);
});

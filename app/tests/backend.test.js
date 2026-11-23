// The backend restarts after a crash: a new port and a new token, and only a few times in a minute.
// A fake child process stands in for the Go binary, so the test counts exits without spawning anything.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const http = require('node:http');
const os = require('node:os');
const path = require('node:path');
const { EventEmitter } = require('node:events');
const { Backend } = require('../main/backend');

// A child process that only knows how to print a port and to exit.
class FakeChild extends EventEmitter {
  constructor(port) {
    super();
    this.port = port;
    this.stdout = new EventEmitter();
    this.stderr = new EventEmitter();
    this.stdin = { end: () => { this.ended = true; } };
    this.killed = false;
    this.ended = false;
  }

  kill() {
    this.killed = true;
  }

  // The backend sees the same exit event a real process gives.
  crash(code = 1) {
    this.emit('exit', code, null);
  }
}

// A health server on a random port, so the health check really passes.
function healthServer() {
  const server = http.createServer((req, res) => {
    res.writeHead(200, { 'content-type': 'application/json' });
    res.end(JSON.stringify({ ok: true, schema_version: 1 }));
  });
  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => resolve({ server, port: server.address().port }));
  });
}

// The backend needs its binary to exist before it spawns it.
function binaryDir() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'fistbump-backend-'));
  const name = process.platform === 'win32' ? 'fistbump-core.exe' : 'fistbump-core';
  fs.writeFileSync(path.join(dir, name), '');
  return dir;
}

// A spawn that hands out one child per call, each with its own port.
function fakeSpawn(ports) {
  const children = [];
  const spawn = () => {
    const child = new FakeChild(ports[Math.min(children.length, ports.length - 1)]);
    children.push(child);
    setImmediate(() => child.stdout.emit('data', `PORT=${child.port}\n`));
    return child;
  };
  return { spawn, children };
}

const once = (emitter, event) => new Promise((resolve) => emitter.once(event, (...args) => resolve(args)));

// Waits for one state, skipping the ones that come before it.
const waitForState = (backend, wanted) => new Promise((resolve) => {
  const onState = (state, extra) => {
    if (state !== wanted) return;
    backend.off('state', onState);
    resolve(extra);
  };
  backend.on('state', onState);
});

// Crashes the running child and waits until the backend is ready again.
async function crashAndWait(backend, child) {
  const restarted = once(backend, 'restarted');
  child.crash(1);
  await restarted;
}

test('a crashed backend comes back with a new port and a new token', async (t) => {
  const first = await healthServer();
  const second = await healthServer();
  t.after(() => { first.server.close(); second.server.close(); });
  const { spawn, children } = fakeSpawn([first.port, second.port]);
  const backend = new Backend({ binDir: binaryDir(), dataDir: os.tmpdir(), spawn });

  await backend.start();
  assert.strictEqual(backend.state, 'ready');
  const port = backend.port;
  const token = backend.token;
  assert.strictEqual(port, first.port);

  await crashAndWait(backend, children[0]);

  assert.strictEqual(backend.state, 'ready');
  assert.notStrictEqual(backend.port, port, 'the restart uses a new port');
  assert.notStrictEqual(backend.token, token, 'the restart uses a new token');
  assert.strictEqual(backend.port, second.port);
  assert.strictEqual(children.length, 2, 'the backend was started again');
});

test('the restarted event is emitted once per restart, after the backend is ready', async (t) => {
  const health = await healthServer();
  t.after(() => health.server.close());
  const { spawn, children } = fakeSpawn([health.port]);
  const backend = new Backend({ binDir: binaryDir(), dataDir: os.tmpdir(), spawn });

  const states = [];
  const restarts = [];
  backend.on('state', (state) => states.push(state));
  backend.on('restarted', () => restarts.push(backend.state));

  await backend.start();
  assert.deepStrictEqual(restarts, [], 'the first start is not a restart');
  assert.deepStrictEqual(states, ['starting', 'ready']);

  await crashAndWait(backend, children[0]);
  assert.deepStrictEqual(restarts, ['ready'], 'the restart is reported when the backend is ready');
  assert.deepStrictEqual(states, ['starting', 'ready', 'crashed', 'starting', 'ready']);
});

test('more than three restarts in a minute stop the restarts', async (t) => {
  const health = await healthServer();
  t.after(() => health.server.close());
  const { spawn, children } = fakeSpawn([health.port]);
  const backend = new Backend({ binDir: binaryDir(), dataDir: os.tmpdir(), spawn });

  await backend.start();

  // Three restarts are allowed.
  for (let i = 0; i < 3; i++) {
    await crashAndWait(backend, children[children.length - 1]);
  }
  assert.strictEqual(children.length, 4, 'the backend was started three times again');
  assert.strictEqual(backend.state, 'ready');

  // The fourth crash in the same minute is too many.
  const failed = waitForState(backend, 'failed');
  children[children.length - 1].crash(1);
  const extra = await failed;
  assert.strictEqual(extra.reason, 'too many crashes');
  assert.strictEqual(backend.state, 'failed');

  // Nothing is started after that.
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.strictEqual(children.length, 4, 'the backend was not started again');
});

test('the backend does not restart while the app is quitting', async (t) => {
  const health = await healthServer();
  t.after(() => health.server.close());
  const { spawn, children } = fakeSpawn([health.port]);
  const backend = new Backend({ binDir: binaryDir(), dataDir: os.tmpdir(), spawn });

  await backend.start();
  backend.quitting = true;

  children[0].crash(1);
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.strictEqual(backend.state, 'stopped');
  assert.strictEqual(children.length, 1, 'the backend was not started again');
});

test('the last lines of the backend error log stay available', async (t) => {
  const health = await healthServer();
  t.after(() => health.server.close());
  const { spawn, children } = fakeSpawn([health.port]);
  const backend = new Backend({ binDir: binaryDir(), dataDir: os.tmpdir(), spawn });

  await backend.start();
  for (let i = 1; i <= 50; i++) children[0].stderr.emit('data', `line ${i}\n`);

  const lines = backend.stderrLines(40);
  assert.strictEqual(lines.length, 40);
  assert.strictEqual(lines[0], 'line 11');
  assert.strictEqual(lines[39], 'line 50');
});

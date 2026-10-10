// Spawns the Go backend, reads its port, authenticates every request, and restarts it after a crash.
const { spawn } = require('child_process');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const { EventEmitter } = require('events');

const STARTUP_TIMEOUT_MS = 10_000;
const SHUTDOWN_TIMEOUT_MS = 5_000;
const MAX_RESTARTS = 3;
const RESTART_WINDOW_MS = 60_000;

class Backend extends EventEmitter {
  constructor({ binDir, dataDir }) {
    super();
    this.binDir = binDir;
    this.dataDir = dataDir;
    this.state = 'stopped'; // starting | ready | crashed | failed | stopping | stopped
    this.child = null;
    this.port = null;
    this.token = null;
    this.stderrTail = [];
    this.restarts = [];
    this.quitting = false;
  }

  binaryPath() {
    return path.join(this.binDir, process.platform === 'win32' ? 'fistbump-core.exe' : 'fistbump-core');
  }

  setState(s, extra) {
    this.state = s;
    this.emit('state', s, extra);
  }

  // start() resolves once the port line was read and /v1/health passed.
  async start() {
    const bin = this.binaryPath();
    if (!fs.existsSync(bin)) {
      throw new Error(`Backend binary not found at ${bin}. Run "npm run build:go".`);
    }
    this.setState('starting');
    this.token = crypto.randomBytes(32).toString('hex');
    this.stderrTail = [];
    const args = ['--data-dir', this.dataDir];
    const llama = path.join(this.binDir, process.platform === 'win32' ? 'llama-server.exe' : 'llama-server');
    if (fs.existsSync(llama)) args.push('--llama-server', llama);
    // Token goes in the environment, not argv, so it is absent from process listings.
    const child = spawn(bin, args, { env: { ...process.env, FISTBUMP_TOKEN: this.token }, stdio: ['pipe', 'pipe', 'pipe'] });
    this.child = child;
    child.stderr.on('data', (d) => {
      for (const line of d.toString().split('\n')) if (line) this.stderrTail.push(line);
      if (this.stderrTail.length > 200) this.stderrTail.splice(0, this.stderrTail.length - 200);
    });
    child.on('exit', (code, signal) => this.onExit(child, code, signal));

    try {
      this.port = await this.readPort(child);
      await this.health();
    } catch (err) {
      this.kill(child);
      this.setState('failed');
      throw err;
    }
    this.setState('ready');
    this.emit('ready');
  }

  readPort(child) {
    return new Promise((resolve, reject) => {
      let buf = '';
      const timer = setTimeout(() => reject(new Error('Backend did not report a port in time.\n' + this.tail())), STARTUP_TIMEOUT_MS);
      child.stdout.on('data', (d) => {
        buf += d.toString();
        const m = buf.match(/^PORT=(\d+)$/m);
        if (m) {
          clearTimeout(timer);
          resolve(Number(m[1]));
        }
      });
      child.once('exit', (code) => {
        clearTimeout(timer);
        reject(new Error(`Backend exited early (code ${code}).\n${this.tail()}`));
      });
    });
  }

  tail() {
    return this.stderrTail.slice(-20).join('\n');
  }

  async health() {
    const res = await this.request('GET', '/v1/health');
    if (res.status !== 200) throw new Error(`Backend health check failed (${res.status}).`);
    return res.json;
  }

  onExit(child, code, signal) {
    if (child !== this.child) return;
    this.child = null;
    if (this.state === 'stopping' || this.quitting) {
      this.setState('stopped');
      return;
    }
    if (this.state === 'starting') return; // start() reports it
    this.setState('crashed', { code, signal });
    const now = Date.now();
    this.restarts = this.restarts.filter((t) => now - t < RESTART_WINDOW_MS);
    if (this.restarts.length >= MAX_RESTARTS) {
      this.setState('failed', { reason: 'too many crashes' });
      return;
    }
    this.restarts.push(now);
    // New port and token on every restart. Listeners re-send secrets on 'ready'.
    this.start().catch((err) => this.emit('error-state', err));
  }

  // request returns { status, headers, json, buffer }. It throws if the backend is not reachable.
  async request(method, urlPath, body) {
    if (!this.port || (this.state !== 'ready' && this.state !== 'starting')) {
      throw new Error('The backend is not running. It may be restarting; try again in a moment.');
    }
    const init = { method, headers: { Authorization: `Bearer ${this.token}` } };
    if (body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    const res = await fetch(`http://127.0.0.1:${this.port}${urlPath}`, init);
    const buffer = Buffer.from(await res.arrayBuffer());
    let json = null;
    if ((res.headers.get('content-type') || '').includes('application/json') && buffer.length) {
      try {
        json = JSON.parse(buffer.toString('utf8'));
      } catch {
        json = null;
      }
    }
    return { status: res.status, headers: res.headers, json, buffer };
  }

  kill(child) {
    try {
      child.kill('SIGKILL');
    } catch {
      /* already gone */
    }
  }

  // stop() closes stdin, which is the quit signal, and kills the process if it does not exit in time.
  async stop() {
    this.quitting = true;
    const child = this.child;
    if (!child) return;
    this.setState('stopping');
    await new Promise((resolve) => {
      const timer = setTimeout(() => {
        this.kill(child);
        resolve();
      }, SHUTDOWN_TIMEOUT_MS);
      child.once('exit', () => {
        clearTimeout(timer);
        resolve();
      });
      try {
        child.stdin.end();
      } catch {
        this.kill(child);
      }
    });
  }
}

module.exports = { Backend };

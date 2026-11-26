// Lightweight local HTTP harness for Playwright E2E tests and README screenshots.
// Spawns the real `fistbump-core` Go backend with a temporary data directory, serves
// `app/renderer/`, and exposes `/__rpc` so a thin browser shim implements `window.api`.
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawn } = require('node:child_process');
const hostArch = require('../scripts/host-arch');

const ROOT = path.join(__dirname, '..');
const RENDERER = path.join(ROOT, 'app', 'renderer');
const PKG = JSON.parse(fs.readFileSync(path.join(ROOT, 'package.json'), 'utf8'));
const HOST = 'localhost';

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.woff2': 'font/woff2',
  '.json': 'application/json',
};

function startBackend(dataDir) {
  return new Promise((resolve, reject) => {
    const binName = process.platform === 'win32' ? 'fistbump-core.exe' : 'fistbump-core';
    const bin = path.join(ROOT, 'bin', `${process.platform}-${hostArch()}`, binName);
    if (!fs.existsSync(bin)) {
      reject(new Error(`Backend binary not found at ${bin}. Run "npm run build:go".`));
      return;
    }
    const token = crypto.randomBytes(32).toString('hex');
    const child = spawn(bin, ['--data-dir', dataDir], {
      env: { ...process.env, FISTBUMP_TOKEN: token },
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    let buf = '';
    const timer = setTimeout(() => reject(new Error('Backend did not report a port in time.')), 10_000);
    child.stdout.on('data', (chunk) => {
      buf += chunk.toString();
      const m = buf.match(/^PORT=(\d+)$/m);
      if (m) {
        clearTimeout(timer);
        resolve({ child, port: Number(m[1]), token });
      }
    });
    child.once('exit', (code) => {
      clearTimeout(timer);
      reject(new Error(`Backend exited early (code ${code}).`));
    });
  });
}

async function stopBackend(child) {
  if (!child || child.exitCode !== null) return;
  await new Promise((resolve) => {
    const timer = setTimeout(() => { try { child.kill(); } catch {} resolve(); }, 5000);
    child.once('exit', () => { clearTimeout(timer); resolve(); });
    try { child.stdin.end(); } catch { resolve(); }
  });
}

function routeRequest(channel, args) {
  const [a0, a1, a2] = args;
  const qs = (obj, allowed) => {
    const p = new URLSearchParams();
    if (obj && typeof obj === 'object') {
      for (const k of allowed) if (typeof obj[k] === 'string' && obj[k]) p.set(k, obj[k]);
    }
    const s = p.toString();
    return s ? `?${s}` : '';
  };
  const map = {
    'connectors:list': () => ['GET', '/v1/connectors'],
    'connectors:categories': () => ['GET', '/v1/connectors/greenhouse/categories'],
    'resume:get': () => ['GET', '/v1/resume'],
    'resume:putProfile': () => ['PUT', '/v1/resume/profile', a0],
    'resume:preview': () => ['GET', '/v1/resume/preview'],
    'experiences:create': () => ['POST', '/v1/resume/experiences', a0],
    'experiences:update': () => ['PUT', `/v1/resume/experiences/${a0}`, a1],
    'experiences:delete': () => ['DELETE', `/v1/resume/experiences/${a0}`],
    'connectors:link': () => ['GET', `/v1/connectors/greenhouse/link?url=${encodeURIComponent(a0 || '')}`],
    'resumes:list': () => ['GET', '/v1/resumes'],
    'settings:get': () => ['GET', '/v1/settings'],
    'settings:put': () => ['PUT', '/v1/settings', a0],
    'storage:report': () => ['GET', '/v1/storage'],
    'storage:cleanup': () => ['POST', '/v1/storage/cleanup', a0],
    'ai:status': () => ['GET', '/v1/ai/status'],
    'models:list': () => ['GET', '/v1/models'],
    'jobs:create': () => ['POST', '/v1/jobs', a0],
    'jobs:list': () => ['GET', `/v1/jobs${qs({ ...(a1 || {}), q: a0 || '' }, ['q', 'archived', 'trashed'])}`],
    'jobs:get': () => ['GET', `/v1/jobs/${a0}`],
    'jobs:analyze': () => ['POST', `/v1/jobs/${a0}/analyze`],
    'applications:update': () => ['PUT', `/v1/applications/${a0}`, a1],
    'revisions:create': () => ['POST', '/v1/revisions', a0],
    'revisions:list': () => ['GET', a0 ? `/v1/revisions?job_id=${a0}` : '/v1/revisions'],
    'revisions:get': () => ['GET', `/v1/revisions/${a0}`],
    'revisions:cancel': () => ['POST', `/v1/revisions/${a0}/cancel`],
    'revisions:delete': () => ['DELETE', `/v1/revisions/${a0}`],
    'suggestions:accept': () => ['POST', `/v1/revisions/${a0}/suggestions/${a1}/accept`],
    'suggestions:reject': () => ['POST', `/v1/revisions/${a0}/suggestions/${a1}/reject`],
    'suggestions:edit': () => ['POST', `/v1/revisions/${a0}/suggestions/${a1}/edit`, { edited_text: a2 }],
    'tailored:create': () => ['POST', '/v1/tailored-resumes', { revision_id: a0 }],
    'tailored:list': () => ['GET', a0 ? `/v1/tailored-resumes?job_id=${a0}` : '/v1/tailored-resumes'],
    'applications:list': () => ['GET', `/v1/applications${qs(a0, ['status', 'q', 'archived'])}`],
    'applications:create': () => ['POST', '/v1/applications', a0],
    'cost:analyze': () => ['POST', '/v1/cost', a0],
  };
  return map[channel]?.() || null;
}

const CLIENT_SHIM = `
(() => {
  if (window.api) return;
  const call = async (channel, ...args) => {
    const res = await fetch('/__rpc', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ channel, args }),
    });
    return res.json();
  };
  const group = (prefix, map) => Object.fromEntries(
    Object.entries(map).map(([k, ch]) => [k, (...a) => call(ch || prefix + ':' + k, ...a)])
  );
  window.api = {
    platform: '${process.platform}',
    setZoom: () => {},
    onBackendRestarted: () => {},
    error: group('error', { details: 0, copyDetails: 0, quit: 0 }),
    connectors: group('connectors', { link: 0, posting: 0, search: 0, suggest: 0, list: 0, categories: 0, logo: 0, cancelSearch: 'connectors:cancel-search', import: 0 }),
    resume: group('resume', { get: 0, putProfile: 0, preview: 0, createExperience: 'experiences:create', updateExperience: 'experiences:update', deleteExperience: 'experiences:delete', createEducation: 'education:create', updateEducation: 'education:update', deleteEducation: 'education:delete', importFile: 0, importData: 0, importText: 0, importApply: 0, export: 0, previewPdf: 0 }),
    resumes: group('resumes', { list: 0, get: 0, create: 0, update: 0, delete: 0 }),
    revisions: group('revisions', { create: 0, list: 0, get: 0, cancel: 0, delete: 0 }),
    suggestions: group('suggestions', { accept: 0, reject: 0, edit: 0 }),
    tailored: group('tailored', { create: 0, createBlank: 0, list: 0, get: 0, update: 0, diff: 0, delete: 0, export: 0, previewPdf: 0 }),
    ai: group('ai', { status: 0, setMode: 0, setRemote: 0, clearRemote: 0, test: 0 }),
    shell: group('shell', { showItem: 0 }),
    settings: group('settings', { get: 0, put: 0 }),
    storage: group('storage', { report: 0, cleanup: 0 }),
    app: { ...group('app', { info: 0, checkForUpdates: 'app:check-updates', showDataFolder: 'app:show-data-folder', openReleases: 'app:open-releases', downloadUpdate: 'app:download-update', cancelUpdate: 'app:cancel-update', openUpdate: 'app:open-update', installUpdate: 'app:install-update' }), onUpdateProgress: () => {}, onOpenSettings: () => {} },
    jobs: group('jobs', { parse: 0, create: 0, list: 0, get: 0, update: 0, delete: 0, analyze: 0, analyzeText: 0, archive: 0, unarchive: 0, trash: 0, restore: 0 }),
    cost: group('cost', { analyze: 0 }),
    models: group('models', { list: 0, get: 0, download: 0, downloadStatus: 0, cancelDownload: 0, delete: 0 }),
    applications: group('applications', { list: 0, create: 0, get: 0, update: 0, delete: 0, bulkStatus: 0, archive: 0, unarchive: 0, bulkArchive: 0 }),
  };
})();
`;

async function startHarness(dataDir) {
  const { child, port, token } = await startBackend(dataDir);
  const authHeader = ['Bearer', token].join(' ');
  const apiBase = `http://${HOST}:${port}`;

  const server = http.createServer(async (req, res) => {
    if (req.method === 'POST' && req.url === '/__rpc') {
      let body = '';
      for await (const chunk of req) body += chunk;
      const { channel, args = [] } = JSON.parse(body || '{}');
      if (channel === 'app:info') {
        res.writeHead(200, { 'content-type': 'application/json' });
        res.end(JSON.stringify({ version: PKG.version, tauri: '2', platform: process.platform, arch: hostArch(), dataDir }));
        return;
      }
      if (channel === 'app:check-updates') {
        res.writeHead(200, { 'content-type': 'application/json' });
        res.end(JSON.stringify({ status: 'current', current: PKG.version, latest: PKG.version, message: '', downloadable: false, size: 0 }));
        return;
      }
      if (channel === 'resume:previewPdf' || channel === 'tailored:previewPdf') {
        const p = channel === 'resume:previewPdf' ? '/v1/resume/export' : `/v1/tailored-resumes/${args[0]}/export`;
        const style = channel === 'resume:previewPdf' ? args[0] : args[1];
        const payload = { format: 'pdf', ...(style ? { style } : {}) };
        const r = await fetch(`${apiBase}${p}`, {
          method: 'POST',
          headers: { Authorization: authHeader, 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        const buf = Buffer.from(await r.arrayBuffer());
        res.writeHead(200, { 'content-type': 'application/json' });
        res.end(JSON.stringify({ data_base64: buf.toString('base64') }));
        return;
      }
      const route = routeRequest(channel, args);
      if (!route) {
        res.writeHead(200, { 'content-type': 'application/json' });
        res.end(JSON.stringify({ error: { code: 'not_found', message: `Unsupported test channel: ${channel}`, details: null } }));
        return;
      }
      const [method, urlPath, payload] = route;
      const r = await fetch(`${apiBase}${urlPath}`, {
        method,
        headers: { Authorization: authHeader, ...(payload !== undefined ? { 'Content-Type': 'application/json' } : {}) },
        ...(payload !== undefined ? { body: JSON.stringify(payload) } : {}),
      });
      const text = r.status === 204 ? '{"ok":true}' : await r.text();
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(text);
      return;
    }

    if (req.url === '/__shim.js') {
      res.writeHead(200, { 'content-type': 'text/javascript; charset=utf-8' });
      res.end(CLIENT_SHIM);
      return;
    }

    const reqPath = (req.url || '/').split('?')[0];
    const rel = reqPath === '/' ? 'index.html' : reqPath.replace(/^\/+/, '');
    const full = path.join(RENDERER, rel);
    if (!full.startsWith(RENDERER) || !fs.existsSync(full) || fs.statSync(full).isDirectory()) {
      res.writeHead(404);
      res.end('Not found');
      return;
    }
    if (rel === 'index.html') {
      const html = fs.readFileSync(full, 'utf8').replace(
        '<script src="js/tauri-bridge.js" defer></script>',
        '<script src="/__shim.js" defer></script>\n        <script src="js/tauri-bridge.js" defer></script>'
      );
      res.writeHead(200, { 'content-type': MIME['.html'] });
      res.end(html);
      return;
    }
    res.writeHead(200, { 'content-type': MIME[path.extname(full)] || 'application/octet-stream' });
    fs.createReadStream(full).pipe(res);
  });

  await new Promise((resolve) => server.listen(0, HOST, resolve));
  const url = `http://${HOST}:${server.address().port}/`;
  return {
    url,
    dataDir,
    async close() {
      await new Promise((resolve) => server.close(resolve));
      await stopBackend(child);
    },
  };
}

module.exports = { startHarness };

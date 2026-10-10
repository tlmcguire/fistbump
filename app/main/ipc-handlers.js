// Maps renderer IPC channels to backend HTTP routes. The renderer never sees the port or token.
const fs = require('fs');
const path = require('path');
const secrets = require('./secrets');

const REMOTE_KEY = 'remote_ai_key';
const HF_TOKEN = 'hf_token';

function id(n) {
  if (!Number.isInteger(n) || n <= 0) throw new Error('invalid id');
  return n;
}
const seg = (s) => encodeURIComponent(String(s));
function qs(o) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o || {})) if (v !== undefined && v !== null && v !== '') p.set(k, String(v));
  const s = p.toString();
  return s ? `?${s}` : '';
}

// channel -> (...args) => [method, path, body?]. Channels with side effects in main live in `special`.
const routes = {
  'resume:get': () => ['GET', '/v1/resume'],
  'resume:saveProfile': (p) => ['PUT', '/v1/resume/profile', p],
  'resume:addExperience': (e) => ['POST', '/v1/resume/experiences', e],
  'resume:updateExperience': (i, e) => ['PUT', `/v1/resume/experiences/${id(i)}`, e],
  'resume:deleteExperience': (i) => ['DELETE', `/v1/resume/experiences/${id(i)}`],
  'resume:addEducation': (e) => ['POST', '/v1/resume/education', e],
  'resume:updateEducation': (i, e) => ['PUT', `/v1/resume/education/${id(i)}`, e],
  'resume:deleteEducation': (i) => ['DELETE', `/v1/resume/education/${id(i)}`],
  'resume:importText': (text, engine) => ['POST', '/v1/resume/import', { text, engine: engine || 'rules' }],
  'resume:applyImport': (body) => ['POST', '/v1/resume/import/apply', body],
  'resume:summary': () => ['POST', '/v1/resume/summary', {}],
  'resume:preview': () => ['GET', '/v1/resume/preview'],

  'resumes:list': () => ['GET', '/v1/resumes'],
  'resumes:create': (r) => ['POST', '/v1/resumes', r],
  'resumes:get': (i) => ['GET', `/v1/resumes/${id(i)}`],
  'resumes:update': (i, r) => ['PUT', `/v1/resumes/${id(i)}`, r],
  'resumes:remove': (i) => ['DELETE', `/v1/resumes/${id(i)}`],

  'jobs:parse': (raw) => ['POST', '/v1/jobs/parse', { raw_text: raw }],
  'jobs:create': (j) => ['POST', '/v1/jobs', j],
  'jobs:list': (f) => ['GET', `/v1/jobs${qs(f)}`],
  'jobs:get': (i) => ['GET', `/v1/jobs/${id(i)}`],
  'jobs:update': (i, j) => ['PUT', `/v1/jobs/${id(i)}`, j],
  'jobs:remove': (i) => ['DELETE', `/v1/jobs/${id(i)}`],
  'jobs:analyze': (i) => ['POST', `/v1/jobs/${id(i)}/analyze`],
  'jobs:archive': (i, opts) => ['POST', `/v1/jobs/${id(i)}/archive`, { archive_application: !!opts?.archiveApplication }],
  'jobs:unarchive': (i) => ['POST', `/v1/jobs/${id(i)}/unarchive`],

  'revisions:create': (r) => ['POST', '/v1/revisions', r],
  'revisions:list': (jobId) => ['GET', `/v1/revisions${qs({ job_id: jobId })}`],
  'revisions:get': (i) => ['GET', `/v1/revisions/${id(i)}`],
  'revisions:cancel': (i) => ['POST', `/v1/revisions/${id(i)}/cancel`],
  'revisions:remove': (i) => ['DELETE', `/v1/revisions/${id(i)}`],
  'revisions:accept': (i, s) => ['POST', `/v1/revisions/${id(i)}/suggestions/${id(s)}/accept`],
  'revisions:reject': (i, s) => ['POST', `/v1/revisions/${id(i)}/suggestions/${id(s)}/reject`],
  'revisions:edit': (i, s, text) => ['POST', `/v1/revisions/${id(i)}/suggestions/${id(s)}/edit`, { edited_text: text }],

  'tailored:create': (revisionId) => ['POST', '/v1/tailored-resumes', { revision_id: revisionId }],
  'tailored:list': (jobId) => ['GET', `/v1/tailored-resumes${qs({ job_id: jobId })}`],
  'tailored:get': (i) => ['GET', `/v1/tailored-resumes/${id(i)}`],
  'tailored:diff': (i) => ['GET', `/v1/tailored-resumes/${id(i)}/diff`],
  'tailored:remove': (i) => ['DELETE', `/v1/tailored-resumes/${id(i)}`],

  'applications:list': (f) => ['GET', `/v1/applications${qs(f)}`],
  'applications:create': (a) => ['POST', '/v1/applications', a],
  'applications:get': (i) => ['GET', `/v1/applications/${id(i)}`],
  'applications:update': (i, a) => ['PUT', `/v1/applications/${id(i)}`, a],
  'applications:remove': (i) => ['DELETE', `/v1/applications/${id(i)}`],
  'applications:bulkStatus': (ids, status) => ['POST', '/v1/applications/bulk-status', { ids: (ids || []).map(id), status }],

  'connectors:list': () => ['GET', '/v1/connectors'],
  'connectors:setEnabled': (cid, enabled) => ['PUT', `/v1/connectors/${seg(cid)}`, { enabled: !!enabled }],
  'connectors:fetch': (cid, query) => ['POST', `/v1/connectors/${seg(cid)}/fetch`, { query: query || {} }],
  'connectors:import': (cid, query, ids) => ['POST', `/v1/connectors/${seg(cid)}/import`, { query: query || {}, external_ids: ids }],
  'connectors:categories': () => ['GET', '/v1/connectors/greenhouse/categories'],
  'connectors:setCategory': (c, enabled) => ['PUT', `/v1/connectors/greenhouse/categories/${seg(c)}`, { enabled: !!enabled }],
  'connectors:addBoard': (b) => ['POST', '/v1/connectors/greenhouse/boards', { board: b }],
  'connectors:removeBoard': (b) => ['DELETE', `/v1/connectors/greenhouse/boards/${seg(b)}`],
  'connectors:posting': (extId) => ['GET', `/v1/connectors/greenhouse/postings/${seg(extId)}`],

  'ai:status': () => ['GET', '/v1/ai/status'],
  'ai:setMode': (mode) => ['PUT', '/v1/ai/mode', { mode }],
  'ai:test': (engine) => ['POST', '/v1/ai/test', { engine }],

  'models:list': () => ['GET', '/v1/models'],
  'models:download': (m) => ['POST', `/v1/models/${seg(m)}/download`],
  'models:downloadStatus': (d) => ['GET', `/v1/models/downloads/${seg(d)}`],
  'models:cancelDownload': (d) => ['DELETE', `/v1/models/downloads/${seg(d)}`],
  'models:remove': (m) => ['DELETE', `/v1/models/${seg(m)}`],

  'settings:get': () => ['GET', '/v1/settings'],
  'settings:set': (s) => ['PUT', '/v1/settings', s],

  'storage:get': () => ['GET', '/v1/storage'],
  'storage:cleanup': (r) => ['POST', '/v1/storage/cleanup', r],
};

// Channels handled in main because they touch secrets, dialogs or the filesystem.
const specialChannels = [
  'ai:setRemote', 'ai:clearRemote', 'models:setToken', 'models:clearToken', 'tailored:export',
  'resume:importFile', 'resume:importData', 'resume:export', 'app:readTextFile', 'app:saveCsv', 'app:info', 'app:getTheme', 'app:setTheme',
  'app:openDataFolder', 'app:paths', 'app:getCustomCss', 'app:setCustomCss', 'app:openCustomCss',
];

const allChannels = () => [...Object.keys(routes), ...specialChannels];

function toResult(res) {
  if (res.status >= 400) {
    return { ok: false, error: res.json?.error || { code: 'internal', message: `Request failed (${res.status})` }, status: res.status };
  }
  return { ok: true, data: res.json };
}

const fail = (message, code = 'unreachable') => ({ ok: false, error: { code, message } });

// Re-sends secrets after every backend (re)start. The Go process keeps them in memory only.
async function deliverSecrets(backend) {
  const key = secrets.get(REMOTE_KEY);
  if (key) {
    const st = await backend.request('GET', '/v1/ai/status');
    const r = st.json?.remote;
    if (r?.base_url && r?.model) await backend.request('PUT', '/v1/ai/remote', { base_url: r.base_url, model: r.model, api_key: key });
  }
  const hf = secrets.get(HF_TOKEN);
  if (hf) await backend.request('PUT', '/v1/models/token', { token: hf });
}

function register({ ipcMain, backend, dialog, getWindow, app, prefs, shell, dataDir, customCss }) {
  const guard = (event) => {
    let u;
    try {
      u = new URL(event.senderFrame?.url || '');
    } catch {
      throw new Error('blocked: unexpected sender');
    }
    // Ignore the hash and query: the view router lives in the hash.
    if (u.protocol !== 'file:' || !u.pathname.endsWith('/renderer/index.html')) throw new Error('blocked: unexpected sender');
  };
  const handle = (channel, fn) =>
    ipcMain.handle(channel, async (event, ...args) => {
      try {
        guard(event);
        return await fn(...args);
      } catch (err) {
        return fail(err.message || String(err));
      }
    });

  for (const [channel, build] of Object.entries(routes)) {
    handle(channel, async (...args) => {
      const [method, urlPath, body] = build(...args);
      return toResult(await backend.request(method, urlPath, body));
    });
  }

  handle('ai:setRemote', async ({ base_url, model, api_key } = {}) => {
    let key = api_key;
    if (key === undefined) key = secrets.get(REMOTE_KEY) || '';
    const res = await backend.request('PUT', '/v1/ai/remote', { base_url, model, ...(key ? { api_key: key } : {}) });
    const out = toResult(res);
    if (out.ok && api_key !== undefined) {
      if (api_key) out.data.key_persisted = secrets.set(REMOTE_KEY, api_key).persisted;
      else secrets.remove(REMOTE_KEY);
    }
    return out;
  });
  handle('ai:clearRemote', async () => {
    secrets.remove(REMOTE_KEY);
    return toResult(await backend.request('DELETE', '/v1/ai/remote'));
  });
  handle('models:setToken', async (token) => {
    const out = toResult(await backend.request('PUT', '/v1/models/token', { token }));
    if (out.ok) secrets.set(HF_TOKEN, token);
    return out;
  });
  handle('models:clearToken', async () => {
    secrets.remove(HF_TOKEN);
    return toResult(await backend.request('DELETE', '/v1/models/token'));
  });

  // Go returns bytes. Main shows the save dialog and writes the file, so Go never receives a path.
  async function saveExport(urlPath, format) {
    if (format !== 'pdf' && format !== 'md') throw new Error('format must be pdf or md');
    const res = await backend.request('POST', urlPath, { format });
    if (res.status !== 200) return { result: toResult(res) };
    const disp = res.headers.get('content-disposition') || '';
    const name = (disp.match(/filename="([^"]+)"/) || [])[1] || `resume.${format}`;
    const pick = await dialog.showSaveDialog(getWindow(), {
      defaultPath: path.join(app.getPath('documents'), name),
      filters: [{ name: format === 'pdf' ? 'PDF' : 'Markdown', extensions: [format] }],
    });
    if (pick.canceled || !pick.filePath) return { result: { ok: true, data: { saved: false } } };
    fs.writeFileSync(pick.filePath, res.buffer);
    return { result: { ok: true, data: { saved: true, path: pick.filePath } }, file: pick.filePath };
  }

  handle('tailored:export', async ({ id: tid, format, applicationId }) => {
    const { result, file } = await saveExport(`/v1/tailored-resumes/${id(tid)}/export`, format);
    if (file && applicationId && format === 'pdf') {
      await backend.request('PUT', `/v1/applications/${id(applicationId)}`, { exported_pdf_path: file });
    }
    return result;
  });

  handle('resume:export', async (format) => (await saveExport('/v1/resume/export', format)).result);

  // Reads a resume the user picks (PDF, text or Markdown) and asks Go for a draft. Nothing is saved yet.
  handle('resume:importFile', async (engine) => {
    const pick = await dialog.showOpenDialog(getWindow(), {
      properties: ['openFile'],
      filters: [{ name: 'Resume (PDF, text, Markdown)', extensions: ['pdf', 'txt', 'md', 'markdown'] }],
    });
    if (pick.canceled || !pick.filePaths[0]) return { ok: true, data: null };
    const file = pick.filePaths[0];
    if (fs.statSync(file).size > 5_000_000) return fail('That file is larger than 5 MB.', 'too_large');
    const body = { filename: path.basename(file), data_base64: fs.readFileSync(file).toString('base64'), engine: engine || 'rules' };
    const out = toResult(await backend.request('POST', '/v1/resume/import', body));
    if (out.ok) out.data.filename = path.basename(file);
    return out;
  });

  handle('app:readTextFile', async () => {
    const pick = await dialog.showOpenDialog(getWindow(), {
      properties: ['openFile'],
      filters: [{ name: 'Text or Markdown', extensions: ['txt', 'md', 'markdown'] }],
    });
    if (pick.canceled || !pick.filePaths[0]) return { ok: true, data: null };
    const file = pick.filePaths[0];
    if (fs.statSync(file).size > 1_000_000) return fail('That file is larger than 1 MB.', 'too_large');
    return { ok: true, data: { name: path.basename(file), text: fs.readFileSync(file, 'utf8') } };
  });

  // A resume dropped on the window. The renderer sends the bytes, never a path, so main and Go only
  // ever read files the user picked or dropped.
  handle('resume:importData', async ({ name, data, engine } = {}) => {
    const bytes = Buffer.from(data || []);
    if (!bytes.length) return fail('That file is empty.', 'invalid');
    if (bytes.length > 5_000_000) return fail('That file is larger than 5 MB.', 'too_large');
    const filename = path.basename(String(name || 'resume'));
    const out = toResult(await backend.request('POST', '/v1/resume/import', { filename, data_base64: bytes.toString('base64'), engine: engine || 'rules' }));
    if (out.ok) out.data.filename = filename;
    return out;
  });

  handle('app:openDataFolder', async () => {
    const err = await shell.openPath(dataDir);
    return err ? fail(err, 'open_failed') : { ok: true, data: null };
  });
  handle('app:paths', async () => ({ ok: true, data: { data_dir: dataDir, settings_dir: app.getPath('userData') } }));

  handle('app:getCustomCss', async () => ({
    ok: true, data: { css: customCss.read() || customCss.TEMPLATE, enabled: !!prefs.get('customCssEnabled'), path: customCss.file() },
  }));
  // { css?, enabled? }: saving text writes the file (the watcher applies it); toggling re-applies now.
  handle('app:setCustomCss', async ({ css, enabled } = {}) => {
    if (css !== undefined) customCss.write(css);
    if (enabled !== undefined) prefs.set('customCssEnabled', !!enabled);
    await customCss.apply(getWindow());
    return { ok: true, data: { enabled: !!prefs.get('customCssEnabled') } };
  });
  handle('app:openCustomCss', async () => {
    const err = await shell.openPath(customCss.ensureFile());
    return err ? fail(err, 'open_failed') : { ok: true, data: null };
  });

  handle('app:saveCsv', async ({ name, csv }) => {
    if (typeof csv !== 'string' || csv.length > 5_000_000) return fail('Nothing to save.', 'invalid');
    const pick = await dialog.showSaveDialog(getWindow(), {
      defaultPath: path.join(app.getPath('documents'), String(name || 'applications.csv').replace(/[\\/]/g, '-')),
      filters: [{ name: 'CSV', extensions: ['csv'] }],
    });
    if (pick.canceled || !pick.filePath) return { ok: true, data: { saved: false } };
    fs.writeFileSync(pick.filePath, '\ufeff' + csv); // BOM so Excel reads UTF-8
    return { ok: true, data: { saved: true, path: pick.filePath } };
  });

  // Theme: system, light or dark. Main applies it with nativeTheme so CSS, scrollbars and dialogs agree.
  handle('app:getTheme', async () => ({ ok: true, data: { theme: prefs.get('theme') } }));
  handle('app:setTheme', async (theme) => {
    if (!['system', 'light', 'dark'].includes(theme)) return fail('theme must be system, light or dark', 'invalid');
    prefs.set('theme', theme);
    return { ok: true, data: { theme } };
  });

  handle('app:info', async () => ({
    ok: true,
    data: { version: app.getVersion(), platform: process.platform, key_storage_encrypted: secrets.canPersist() },
  }));
}

module.exports = { routes, specialChannels, allChannels, register, deliverSecrets, toResult };

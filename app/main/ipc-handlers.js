// Maps renderer IPC channels to backend HTTP routes. The renderer never sees the port or token.
const CHANNELS = ['connectors:link', 'connectors:posting', 'connectors:search', 'connectors:list', 'connectors:categories', 'connectors:cancel-search', 'connectors:logo',
   'resume:get', 'resume:putProfile', 'resume:preview',
   'resume:importFile', 'resume:importData', 'resume:importText', 'resume:importApply',
   'experiences:create', 'experiences:update', 'experiences:delete',
   'education:create', 'education:update', 'education:delete',
   'resumes:list', 'resumes:get', 'resumes:create', 'resumes:update', 'resumes:delete',
   'settings:get', 'settings:put',
   'storage:report', 'storage:cleanup', 'app:info', 'app:check-updates', 'app:show-data-folder', 'app:open-releases',
   'jobs:parse', 'jobs:create', 'jobs:list', 'jobs:get', 'jobs:update', 'jobs:delete', 'jobs:analyze', 'connectors:import',
   'revisions:create', 'revisions:list', 'revisions:get', 'revisions:cancel', 'revisions:delete',
   'suggestions:accept', 'suggestions:reject', 'suggestions:edit',
   'tailored:create', 'tailored:list', 'tailored:get', 'tailored:diff', 'tailored:delete',
   'resume:export', 'tailored:export', 'shell:showItem',
   'applications:list', 'applications:create', 'applications:get', 'applications:update', 'applications:delete',
   'applications:bulkStatus', 'applications:archive', 'applications:unarchive', 'applications:bulkArchive',
   'jobs:archive', 'jobs:unarchive', 'jobs:trash', 'jobs:restore',];

const path = require('node:path');

// Cleanup targets the settings page may ask for. The backend validates them again.
const CLEANUP_TARGETS = ['cache', 'partial_downloads'];

// The backend refuses larger files too; checking here first avoids reading or sending them.
const MAX_IMPORT_BYTES = 5 * 1024 * 1024;
const IMPORT_EXTENSIONS = ['pdf', 'txt', 'md'];

const bad = (message) => ({ error: { code: 'validation_failed', message, details: null } });
const seg = (s) => encodeURIComponent(String(s));

// Ids become path segments, so only positive integers pass. Anything else could reach another route.
const isId = (id) => Number.isSafeInteger(id) && id > 0;
const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);
const isIdList = (ids) => Array.isArray(ids) && ids.length > 0 && ids.length <= 1000 && ids.every(isId);

// A query string from the allowed keys only. Empty values are left out.
function query(values, allowed) {
  const params = new URLSearchParams();
  if (isObject(values)) {
    for (const key of allowed) {
      const value = values[key];
      if (typeof value === 'string' && value !== '') params.set(key, value);
    }
  }
  const text = params.toString();
  return text ? `?${text}` : '';
}

//helper function for when a user decides to accept or reject suggested changes
const decide = (verb) => (rid, sid) =>
  isId(rid) && isId(sid) ? ['POST', `/v1/revisions/${rid}/suggestions/${sid}/${verb}`] : bad('Invalid suggestion');

// Channels that map straight to one route
// Each builder checks its arguments, so the renderer can only reach a route it was given.
const ROUTES = {
  'resume:get': () => ['GET', '/v1/resume'],
  'resume:putProfile': (profile) => isObject(profile) ? ['PUT', '/v1/resume/profile', profile] : bad('Invalid profile'),
  'resume:preview': () => ['GET', '/v1/resume/preview'],
  'resume:importText': (text) => typeof text === 'string' && text.trim() ? ['POST', '/v1/resume/import', { text }] : bad('Paste the text of your resume.'),
  'resume:importApply': (body) => isObject(body) && isObject(body.draft) ? ['POST', '/v1/resume/import/apply', body] : bad('Invalid import'),

  'experiences:create': (entry) => isObject(entry) ? ['POST', '/v1/resume/experiences', entry] : bad('Invalid experience'),
  'experiences:update': (id, entry) => isId(id) && isObject(entry) ? ['PUT', `/v1/resume/experiences/${id}`, entry] : bad('Invalid experience'),
  'experiences:delete': (id) => isId(id) ? ['DELETE', `/v1/resume/experiences/${id}`] : bad('Invalid experience id'),

  'education:create': (entry) => isObject(entry) ? ['POST', '/v1/resume/education', entry] : bad('Invalid education entry'),
  'education:update': (id, entry) => isId(id) && isObject(entry) ? ['PUT', `/v1/resume/education/${id}`, entry] : bad('Invalid education entry'),
  'education:delete': (id) => isId(id) ? ['DELETE', `/v1/resume/education/${id}`] : bad('Invalid education id'),

  'resumes:list': () => ['GET', '/v1/resumes'],
  'resumes:get': (id) => isId(id) ? ['GET', `/v1/resumes/${id}`] : bad('Invalid resume id'),
  'resumes:create': (body) => isObject(body) ? ['POST', '/v1/resumes', body] : bad('Invalid resume'),
  'resumes:update': (id, body) => isId(id) && isObject(body) ? ['PUT', `/v1/resumes/${id}`, body] : bad('Invalid resume'),
  'resumes:delete': (id) => isId(id) ? ['DELETE', `/v1/resumes/${id}`] : bad('Invalid resume id'),

  //new routes for handling revisions, suggestions, and the management of tailored resumes
  'revisions:create': (body) => isObject(body) && isId(body.job_id) ? ['POST', '/v1/revisions', body] : bad('Choose a job first.'),
  'revisions:list': (jobId) => ['GET', isId(jobId) ? `/v1/revisions?job_id=${jobId}` : '/v1/revisions'],
  'revisions:get': (id) => isId(id) ? ['GET', `/v1/revisions/${id}`] : bad('Invalid revision id'),
  'revisions:cancel': (id) => isId(id) ? ['POST', `/v1/revisions/${id}/cancel`] : bad('Invalid revision id'),
  'revisions:delete': (id) => isId(id) ? ['DELETE', `/v1/revisions/${id}`] : bad('Invalid revision id'),
  'suggestions:accept': decide('accept'),
  'suggestions:reject': decide('reject'),
  'suggestions:edit': (rid, sid, text) => isId(rid) && isId(sid) && typeof text === 'string' && text.trim()
    ? ['POST', `/v1/revisions/${rid}/suggestions/${sid}/edit`, { edited_text: text }] : bad('Write the edited text.'),
  'tailored:create': (revisionId) => isId(revisionId) ? ['POST', '/v1/tailored-resumes', { revision_id: revisionId }] : bad('Invalid revision id'),
  'tailored:list': (jobId) => ['GET', isId(jobId) ? `/v1/tailored-resumes?job_id=${jobId}` : '/v1/tailored-resumes'],
  'tailored:get': (id) => isId(id) ? ['GET', `/v1/tailored-resumes/${id}`] : bad('Invalid id'),
  'tailored:diff': (id) => isId(id) ? ['GET', `/v1/tailored-resumes/${id}/diff`] : bad('Invalid id'),
  'tailored:delete': (id) => isId(id) ? ['DELETE', `/v1/tailored-resumes/${id}`] : bad('Invalid id'), 



  
  'settings:get': () => ['GET', '/v1/settings'],
  'settings:put': (values) => isObject(values) ? ['PUT', '/v1/settings', values] : bad('Invalid settings'),

  //new in phase 4 for job management
  'jobs:parse':   (raw_text) => typeof raw_text === 'string' && raw_text.trim() ? ['POST', '/v1/jobs/parse', { raw_text }] : bad('Paste the job posting.'),
  'jobs:create':  (body) => isObject(body) ? ['POST', '/v1/jobs', body] : bad('Invalid job'),
  // jobs:list takes the search text and, optionally, { archived: 'include'|'only', trashed: 'only' }.
  'jobs:list':    (q, opts) => ['GET', `/v1/jobs${query({ ...(isObject(opts) ? opts : {}), q: q ? String(q) : '' }, ['q', 'archived', 'trashed'])}`],
  'jobs:get':     (id) => isId(id) ? ['GET', `/v1/jobs/${id}`] : bad('Invalid job id'),
  'jobs:update':  (id, body) => isId(id) && isObject(body) ? ['PUT', `/v1/jobs/${id}`, body] : bad('Invalid job'),
  'jobs:delete':  (id) => isId(id) ? ['DELETE', `/v1/jobs/${id}`] : bad('Invalid job id'),
  'jobs:analyze': (id) => isId(id) ? ['POST', `/v1/jobs/${id}/analyze`] : bad('Invalid job id'),

  //phase 7: job archive and trash
  'jobs:archive':   (id, opts) => isId(id)
    ? ['POST', `/v1/jobs/${id}/archive`, { archive_application: Boolean(isObject(opts) && opts.archive_application) }] : bad('Invalid job id'),
  'jobs:unarchive': (id) => isId(id) ? ['POST', `/v1/jobs/${id}/unarchive`] : bad('Invalid job id'),
  'jobs:trash':     (id) => isId(id) ? ['POST', `/v1/jobs/${id}/trash`] : bad('Invalid job id'),
  'jobs:restore':   (id) => isId(id) ? ['POST', `/v1/jobs/${id}/restore`] : bad('Invalid job id'),

  //phase 7: the application tracker
  'applications:list':   (filters) => ['GET', `/v1/applications${query(filters, ['status', 'q', 'archived'])}`],
  'applications:create': (body) => isObject(body) && isId(body.job_id) ? ['POST', '/v1/applications', body] : bad('Choose a job first.'),
  'applications:get':    (id) => isId(id) ? ['GET', `/v1/applications/${id}`] : bad('Invalid application id'),
  'applications:update': (id, patch) => isId(id) && isObject(patch) ? ['PUT', `/v1/applications/${id}`, patch] : bad('Invalid application'),
  'applications:delete': (id) => isId(id) ? ['DELETE', `/v1/applications/${id}`] : bad('Invalid application id'),
  'applications:bulkStatus': (ids, status) => isIdList(ids) && typeof status === 'string'
    ? ['POST', '/v1/applications/bulk-status', { ids, status }] : bad('Choose 1 to 1000 applications and a status.'),
  'applications:archive':   (id) => isId(id) ? ['POST', `/v1/applications/${id}/archive`] : bad('Invalid application id'),
  'applications:unarchive': (id) => isId(id) ? ['POST', `/v1/applications/${id}/unarchive`] : bad('Invalid application id'),
  'applications:bulkArchive': (ids, archived) => isIdList(ids) && typeof archived === 'boolean'
    ? ['POST', '/v1/applications/bulk-archive', { ids, archived }] : bad('Choose 1 to 1000 applications.'),
  'connectors:import': (postings) => Array.isArray(postings) && postings.length &&
    postings.every(p => isObject(p) && typeof p.board === 'string' && typeof p.external_id === 'string')
    ? ['POST', '/v1/connectors/greenhouse/import', { postings }] : bad('Invalid posting'),
};

const extensionOf = (name) => (/\.([^./\\]+)$/.exec(name) || [])[1]?.toLowerCase() || '';
const baseName = (name) => String(name).split(/[\\/]/).pop();

// The quick checks before a file is sent. The backend decides the type from the bytes.
function checkImport(name, size) {
  if (!IMPORT_EXTENSIONS.includes(extensionOf(name))) return bad(`${baseName(name)} is not a PDF, .txt or .md file.`);
  if (size === 0) return bad('The file is empty or could not be read.');
  if (size > MAX_IMPORT_BYTES) return bad('The file is larger than 5 MB. Export it again without images, or paste the text instead.');
  return null;
}

// The export formats, with the save dialog filter for each.
const EXPORT_FORMATS = {
  pdf: { name: 'PDF', extensions: ['pdf'] },
  md: { name: 'Markdown', extensions: ['md'] },
};

// The file name the backend suggests in Content-Disposition: the UTF-8 filename* when present,
// else the plain filename. Only the last path part is kept, so a name can never pick a folder.
function suggestedName(header, fallback) {
  const h = String(header || '');
  let name = '';
  const extended = /filename\*\s*=\s*UTF-8''([^;]+)/i.exec(h);
  if (extended) {
    try {
      name = decodeURIComponent(extended[1].trim());
    } catch {
      name = '';
    }
  }
  if (!name) {
    const plain = /filename\s*=\s*"([^"]*)"/i.exec(h) || /filename\s*=\s*([^;]+)/i.exec(h);
    if (plain) name = plain[1].trim();
  }
  name = baseName(name).replace(/[\u0000-\u001f]/g, '').trim();
  return name && name !== '.' && name !== '..' ? name : fallback;
}

const importBytes = (backend, filename, bytes) =>
  backend.call('POST', '/v1/resume/import', { filename, data_base64: Buffer.from(bytes).toString('base64') });

// deps holds Electron's dialog and BrowserWindow, fs.promises, and the app-level actions that need
// Electron (version details, the update check, opening folders and pages); tests pass fakes.
function register(ipcMain, backend, deps = {}) {
  const fsp = deps.fs || require('node:fs').promises;
  // Only one search runs at a time: starting a new one, or pressing Cancel, stops the old one.
  let activeSearch = null;

  ipcMain.handle('connectors:link', (_event, url) => {
    if (typeof url !== 'string' || !url.trim()) return bad('Enter a Greenhouse job link.');
    return backend.call('GET', `/v1/connectors/greenhouse/link?url=${encodeURIComponent(url.trim())}`);
  });

  ipcMain.handle('connectors:posting', (_event, board, jobId) => {
    if (typeof board !== 'string' || typeof jobId !== 'string' || !board || !jobId) {
      return bad('Missing board or job id.');
    }
    return backend.call('GET', `/v1/connectors/greenhouse/boards/${seg(board)}/jobs/${seg(jobId)}`);
  });

  //new phase 2
  ipcMain.handle('connectors:search', (_event, query) => {
    if (!query || typeof query !== 'object' || Array.isArray(query)) return bad('Invalid search');
    activeSearch?.abort();
    const controller = new AbortController();
    activeSearch = controller;
    return backend.call('POST', '/v1/connectors/greenhouse/fetch', {query}, controller.signal).finally(() => {
      if (activeSearch === controller) activeSearch = null;
    });
  });

  ipcMain.handle('connectors:cancel-search', () => {
    activeSearch?.abort();
    activeSearch = null;
    return { ok: true };
  });

  // The company's logo as a data URL, read by the backend from the company's own site or its Greenhouse board.
  ipcMain.handle('connectors:logo', (_event, url, board) => {
    const site = typeof url === 'string' ? url : '';
    const token = typeof board === 'string' ? board : '';
    if (!site && !token) return bad('Enter a company website link or a board');
    const query = new URLSearchParams();
    if (site) query.set('url', site);
    if (token) query.set('board', token);
    return backend.call('GET', `/v1/connectors/greenhouse/logo?${query}`);
  });

  ipcMain.handle('connectors:categories', () => backend.call('GET', '/v1/connectors/greenhouse/categories'));

  ipcMain.handle('connectors:list', () => backend.call('GET', '/v1/connectors'));

  // Shows the file picker, then reads and sends the file. The renderer never sends a path.
  ipcMain.handle('resume:importFile', async (event) => {
    const win = deps.BrowserWindow?.fromWebContents(event.sender);
    const picked = await deps.dialog.showOpenDialog(win, {
      title: 'Import a resume',
      properties: ['openFile'],
      filters: [{ name: 'Resumes', extensions: IMPORT_EXTENSIONS }],
    });
    if (picked.canceled || !picked.filePaths.length) return { canceled: true };
    const file = picked.filePaths[0];
    try {
      const refused = checkImport(file, (await fsp.stat(file)).size);
      if (refused) return refused;
      return importBytes(backend, baseName(file), await fsp.readFile(file));
    } catch {
      return bad('The file is empty or could not be read.');
    }
  });

  // A dropped file: the renderer read its bytes with File.arrayBuffer().
  ipcMain.handle('resume:importData', (_event, filename, bytes) => {
    if (typeof filename !== 'string' || !(bytes instanceof ArrayBuffer || ArrayBuffer.isView(bytes))) {
      return bad('The file is empty or could not be read.');
    }
    return checkImport(filename, bytes.byteLength) || importBytes(backend, baseName(filename), bytes instanceof ArrayBuffer ? new Uint8Array(bytes) : bytes);
  });

  //Loop over routes to add an ipcMain.handle to each channel
  for (const [channel, build] of Object.entries(ROUTES)) {
    ipcMain.handle(channel, (_event, ...args) => {
      const route = build(...args);
      if (!Array.isArray(route)) return route;
      const [method, urlPath, body] = route;
      return backend.call(method, urlPath, body);
    });
  }
  // Export: the backend makes the file, the user picks where it goes, and main writes it there.
  // The renderer never sends a path. lastSaved is the one file shell:showItem may reveal.
  let lastSaved = null;
  async function saveExport(event, urlPath, format) {
    const kind = EXPORT_FORMATS[format];
    if (!kind) return bad('Format must be pdf or md');
    // A 500 with details.fallback "md" comes back as is; the renderer offers Markdown.
    const res = await backend.requestRaw('POST', urlPath, { format });
    if (res.error) return res;
    const name = suggestedName(res.headers?.get?.('content-disposition'), `Resume.${format}`);
    const documents = deps.documentsDir?.();
    const options = {
      title: `Export ${kind.name}`,
      defaultPath: documents ? path.join(documents, name) : name,
      filters: [{ name: kind.name, extensions: kind.extensions }],
    };
    const win = deps.BrowserWindow?.fromWebContents(event.sender);
    const picked = await (win ? deps.dialog.showSaveDialog(win, options) : deps.dialog.showSaveDialog(options));
    if (picked.canceled || !picked.filePath) return { saved: false };
    try {
      await fsp.writeFile(picked.filePath, Buffer.from(res.bytes));
    } catch (err) {
      return { error: { code: 'internal', message: `Could not save the file: ${err.message}`, details: null } };
    }
    lastSaved = picked.filePath;
    return { saved: true, path: picked.filePath };
  }

  // After a tailored resume's PDF is saved, the application that uses it records where it went.
  // The file is already saved, so a failure here does not fail the export.
  async function recordExport(tailoredId, filePath) {
    const list = await backend.call('GET', '/v1/applications?archived=include');
    const app = Array.isArray(list) && list.find((a) => a.tailored_resume_id === tailoredId);
    if (!app) return null;
    const updated = await backend.call('PUT', `/v1/applications/${app.id}`, { exported_pdf_path: filePath });
    return updated?.error ? null : app.id;
  }

  ipcMain.handle('resume:export', (event, format) => saveExport(event, '/v1/resume/export', format));
  ipcMain.handle('tailored:export', async (event, id, format) => {
    if (!isId(id)) return bad('Invalid id');
    const result = await saveExport(event, `/v1/tailored-resumes/${id}/export`, format);
    if (result.saved && format === 'pdf') {
      const applicationId = await recordExport(id, result.path).catch(() => null);
      if (applicationId) result.application_id = applicationId;
    }
    return result;
  });

  // Reveals a file Fistbump saved: the last export, or a PDF an application recorded. Main checks the
  // path itself, so the renderer cannot open an arbitrary folder.
  ipcMain.handle('shell:showItem', async (_event, filePath) => {
    if (typeof filePath !== 'string' || !filePath) return bad('Only a file Fistbump saved can be shown.');
    let known = filePath === lastSaved;
    if (!known) {
      const list = await backend.call('GET', '/v1/applications?archived=include').catch(() => null);
      known = Array.isArray(list) && list.some((a) => a.exported_pdf_path === filePath);
    }
    if (!known) return bad('Only a file Fistbump saved can be shown.');
    if (!deps.shell?.showItemInFolder) return bad('Not available');
    deps.shell.showItemInFolder(filePath);
    return { ok: true };
  });

  // Settings: storage use and manual cleanup.
  ipcMain.handle('storage:report', () => backend.call('GET', '/v1/storage'));
  ipcMain.handle('storage:cleanup', (_event, targets) => {
    if (!Array.isArray(targets) || !targets.length || !targets.every((t) => CLEANUP_TARGETS.includes(t))) {
      return bad('Unknown cleanup target');
    }
    return backend.call('POST', '/v1/storage/cleanup', { targets });
  });

  // Settings: about the app, updates, and the data folder.
  const missing = () => bad('Not available');
  ipcMain.handle('app:info', () => (deps.info ? deps.info() : missing()));
  ipcMain.handle('app:check-updates', () => (deps.checkForUpdates ? deps.checkForUpdates() : missing()));
  ipcMain.handle('app:show-data-folder', () => (deps.showDataFolder ? deps.showDataFolder() : missing()));
  ipcMain.handle('app:open-releases', (_event, url) => (deps.openReleases ? deps.openReleases(url) : missing()));
}

module.exports = { CHANNELS, register, MAX_IMPORT_BYTES };

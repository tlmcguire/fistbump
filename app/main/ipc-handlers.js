// Maps renderer IPC channels to backend HTTP routes. The renderer never sees the port or token.
const CHANNELS = ['connectors:link', 'connectors:posting', 'connectors:search', 'connectors:list', 'connectors:categories', 'connectors:cancel-search', 'connectors:logo',
   'resume:get', 'resume:putProfile', 'resume:preview',
   'experiences:create', 'experiences:update', 'experiences:delete',
   'education:create', 'education:update', 'education:delete',
   'resumes:list', 'resumes:get', 'resumes:create', 'resumes:update', 'resumes:delete',
   'settings:get', 'settings:put'];

const bad = (message) => ({ error: { code: 'validation_failed', message, details: null } });
const seg = (s) => encodeURIComponent(String(s));

function register(ipcMain, backend) {
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
}

module.exports = { CHANNELS, register };
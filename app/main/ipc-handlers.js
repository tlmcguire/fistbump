// Maps renderer IPC channels to backend HTTP routes. The renderer never sees the port or token.
const CHANNELS = ['connectors:link', 'connectors:posting', 'connectors:search', 'connectors:list', 'connectors:categories', 'connectors:cancel-search', 'connectors:logo',
  'storage:report', 'storage:cleanup', 'app:info', 'app:check-updates', 'app:show-data-folder', 'app:open-releases'];

// Cleanup targets the settings page may ask for. The backend validates them again.
const CLEANUP_TARGETS = ['cache', 'partial_downloads'];

const bad = (message) => ({ error: { code: 'validation_failed', message, details: null } });
const seg = (s) => encodeURIComponent(String(s));

// register connects every channel. appApi holds the app-level actions that need Electron (version
// details, the update check, opening folders and pages), so tests can pass plain functions.
function register(ipcMain, backend, appApi = {}) {
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
  ipcMain.handle('app:info', () => (appApi.info ? appApi.info() : missing()));
  ipcMain.handle('app:check-updates', () => (appApi.checkForUpdates ? appApi.checkForUpdates() : missing()));
  ipcMain.handle('app:show-data-folder', () => (appApi.showDataFolder ? appApi.showDataFolder() : missing()));
  ipcMain.handle('app:open-releases', (_event, url) => (appApi.openReleases ? appApi.openReleases(url) : missing()));
}

module.exports = { CHANNELS, register };
// Maps renderer IPC channels to backend HTTP routes. The renderer never sees the port or token.
const CHANNELS = ['connectors:link', 'connectors:posting', 'connectors:search', 'connectors:list', 'connectors:categories'];

const bad = (message) => ({ error: { code: 'validation_failed', message, details: null } });
const seg = (s) => encodeURIComponent(String(s));

function register(ipcMain, backend) {
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
    return backend.call('POST', '/v1/connectors/greenhouse/fetch', {query});
  });

  ipcMain.handle('connectors:categories', () => backend.call('GET', '/v1/connectors/greenhouse/categories'));

  ipcMain.handle('connectors:list', () => backend.call('GET', '/v1/connectors'));
}

module.exports = { CHANNELS, register };
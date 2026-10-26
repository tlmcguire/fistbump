// Maps renderer IPC channels to backend HTTP routes. The renderer never sees the port or token.
const CHANNELS = ['connectors:link', 'connectors:posting', 'connectors:search', 'connectors:list', 'connectors:categories', 'connectors:cancel-search', 'connectors:logo',
   'resume:get', 'resume:putProfile', 'resume:preview',
   'experiences:create', 'experiences:update', 'experiences:delete',
   'education:create', 'education:update', 'education:delete',
   'resumes:list', 'resumes:get', 'resumes:create', 'resumes:update', 'resumes:delete',
   'settings:get', 'settings:put'];

const bad = (message) => ({ error: { code: 'validation_failed', message, details: null } });
const seg = (s) => encodeURIComponent(String(s));

// Ids become path segments, so only positive integers pass. Anything else could reach another route.
const isId = (id) => Number.isSafeInteger(id) && id > 0;
const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

// Channels that map straight to one route
// Each builder checks its arguments, so the renderer can only reach a route it was given.
const ROUTES = {
  'resume:get': () => ['GET', '/v1/resume'],
  'resume:putProfile': (profile) => isObject(profile) ? ['PUT', '/v1/resume/profile', profile] : bad('Invalid profile'),
  'resume:preview': () => ['GET', '/v1/resume/preview'],

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

  'settings:get': () => ['GET', '/v1/settings'],
  'settings:put': (values) => isObject(values) ? ['PUT', '/v1/settings', values] : bad('Invalid settings'),
};

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

  //Loop over routes to add an ipcMain.handle to each channel
  for (const [channel, build] of Object.entries(ROUTES)) {
    ipcMain.handle(channel, (_event, ...args) => {
      const route = build(...args);
      if (!Array.isArray(route)) return route;
      const [method, urlPath, body] = route;
      return backend.call(method, urlPath, body);
    });
  }
}
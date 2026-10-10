// Exposes a small named API to the renderer. The renderer cannot reach ipcRenderer, the port or the token.
// The channel list must match app/main/ipc-handlers.js (app/tests checks this).
const { contextBridge, ipcRenderer } = require('electron');

const CHANNELS = {
  resume: ['get', 'saveProfile', 'addExperience', 'updateExperience', 'deleteExperience', 'addEducation', 'updateEducation', 'deleteEducation',
    'importText', 'importFile', 'importData', 'applyImport', 'summary', 'preview', 'export'],
  resumes: ['list', 'create', 'get', 'update', 'remove'],
  jobs: ['parse', 'create', 'list', 'get', 'update', 'remove', 'analyze', 'archive', 'unarchive'],
  revisions: ['create', 'list', 'get', 'cancel', 'remove', 'accept', 'reject', 'edit'],
  tailored: ['create', 'list', 'get', 'diff', 'remove', 'export'],
  applications: ['list', 'create', 'get', 'update', 'remove', 'bulkStatus'],
  connectors: ['list', 'setEnabled', 'fetch', 'import', 'categories', 'setCategory', 'addBoard', 'removeBoard', 'posting'],
  ai: ['status', 'setMode', 'test', 'setRemote', 'clearRemote'],
  models: ['list', 'download', 'downloadStatus', 'cancelDownload', 'remove', 'setToken', 'clearToken'],
  settings: ['get', 'set'],
  storage: ['get', 'cleanup'],
  app: ['readTextFile', 'saveCsv', 'info', 'getTheme', 'setTheme', 'openDataFolder', 'paths', 'getCustomCss', 'setCustomCss', 'openCustomCss'],
};

const api = {};
for (const [group, methods] of Object.entries(CHANNELS)) {
  api[group] = {};
  for (const m of methods) {
    api[group][m] = (...args) => ipcRenderer.invoke(`${group}:${m}`, ...args);
  }
}
contextBridge.exposeInMainWorld('api', Object.freeze(api));

// Menu and shortcut commands from the main process, and the platform for window-chrome layout.
contextBridge.exposeInMainWorld('fistbump', Object.freeze({
  platform: process.platform,
  onCommand: (cb) => {
    const listener = (_e, msg) => cb(msg);
    ipcRenderer.on('app:command', listener);
    return () => ipcRenderer.removeListener('app:command', listener);
  },
}));

const { contextBridge, ipcRenderer, webFrame } = require('electron');

contextBridge.exposeInMainWorld('api', {
  // The renderer styles itself for the platform, for example to leave room for the macOS window buttons.
  platform: process.platform,

  // The text size setting zooms the whole page, layout included. Only these steps are allowed.
  setZoom: (factor) => {
    if ([0.9, 1, 1.15, 1.3].includes(factor)) webFrame.setZoomFactor(factor);
  },

  connectors: {
    link: (url) => ipcRenderer.invoke('connectors:link', url),
    posting: (board, jobId) => ipcRenderer.invoke('connectors:posting', board, jobId),

    //new in phase 2
    search: (query) => ipcRenderer.invoke('connectors:search', query),
    list: () => ipcRenderer.invoke('connectors:list'),
    categories: () => ipcRenderer.invoke('connectors:categories'),
    logo: (url, board) => ipcRenderer.invoke('connectors:logo', url, board),
    cancelSearch: () => ipcRenderer.invoke('connectors:cancel-search'),
  },
  storage: {
    report: () => ipcRenderer.invoke('storage:report'),
    cleanup: (targets) => ipcRenderer.invoke('storage:cleanup', targets),
  },
  app: {
    info: () => ipcRenderer.invoke('app:info'),
    checkForUpdates: () => ipcRenderer.invoke('app:check-updates'),
    showDataFolder: () => ipcRenderer.invoke('app:show-data-folder'),
    openReleases: (url) => ipcRenderer.invoke('app:open-releases', url),
    // The menu's Settings item (Cmd/Ctrl+,) asks the page to show its settings.
    onOpenSettings: (callback) => ipcRenderer.on('app:open-settings', () => callback()),
  },
});
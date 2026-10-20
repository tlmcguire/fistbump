const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('api', {
  connectors: {
    link: (url) => ipcRenderer.invoke('connectors:link', url),
    posting: (board, jobId) => ipcRenderer.invoke('connectors:posting', board, jobId),
  },
});
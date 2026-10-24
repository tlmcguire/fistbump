const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('api', {
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
});
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
    //added for phase 
    import: (postings) => ipcRenderer.invoke('connectors:import', postings),
  },

  //Master resume with options for profile, experience, education, and preview
  resume: {
    get: () => ipcRenderer.invoke('resume:get'),
    putProfile: (profile) => ipcRenderer.invoke('resume:putProfile', profile),
    preview: () => ipcRenderer.invoke('resume:preview'),
    createExperience: (entry) => ipcRenderer.invoke('experiences:create', entry),
    updateExperience: (id, entry) => ipcRenderer.invoke('experiences:update', id, entry),
    deleteExperience: (id) => ipcRenderer.invoke('experiences:delete', id),
    createEducation: (entry) => ipcRenderer.invoke('education:create', entry),
    updateEducation: (id, entry) => ipcRenderer.invoke('education:update', id, entry),
    deleteEducation: (id) => ipcRenderer.invoke('education:delete', id),
    // Import: a draft to review, then apply. importData takes the dropped file's bytes.
    importFile: () => ipcRenderer.invoke('resume:importFile'),
    importData: (filename, bytes) => ipcRenderer.invoke('resume:importData', filename, bytes),
    importText: (text) => ipcRenderer.invoke('resume:importText', text),
    importApply: (body) => ipcRenderer.invoke('resume:importApply', body),
  },

  //Saved resume versions
  resumes: {
    list: () => ipcRenderer.invoke('resumes:list'),
    get: (id) => ipcRenderer.invoke('resumes:get', id),
    create: (body) => ipcRenderer.invoke('resumes:create', body),
    update: (id, body) => ipcRenderer.invoke('resumes:update', id, body),
    delete: (id) => ipcRenderer.invoke('resumes:delete', id),
  },

  //new in phase 5, for revisions suggestions and tailored resumes
  revisions: {
    create: (body) => ipcRenderer.invoke('revisions:create', body),
    list: (jobId) => ipcRenderer.invoke('revisions:list', jobId),
    get: (id) => ipcRenderer.invoke('revisions:get', id),
    cancel: (id) => ipcRenderer.invoke('revisions:cancel', id),
    delete: (id) => ipcRenderer.invoke('revisions:delete', id),
  },
  suggestions: {
    accept: (rid, sid) => ipcRenderer.invoke('suggestions:accept', rid, sid),
    reject: (rid, sid) => ipcRenderer.invoke('suggestions:reject', rid, sid),
    edit: (rid, sid, text) => ipcRenderer.invoke('suggestions:edit', rid, sid, text),
  },
  tailored: {
    create: (revisionId) => ipcRenderer.invoke('tailored:create', revisionId),
    list: (jobId) => ipcRenderer.invoke('tailored:list', jobId),
    get: (id) => ipcRenderer.invoke('tailored:get', id),
    diff: (id) => ipcRenderer.invoke('tailored:diff', id),
    delete: (id) => ipcRenderer.invoke('tailored:delete', id),
  },
  

  //new in phase 3
  settings: {
    get: () => ipcRenderer.invoke('settings:get'),
    put: (values) => ipcRenderer.invoke('settings:put', values),
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
  },

  //added for phase 4
  jobs: {
    parse: (t) => ipcRenderer.invoke('jobs:parse', t),
    create: (b) => ipcRenderer.invoke('jobs:create', b),
    list: (q) => ipcRenderer.invoke('jobs:list', q),
    get: (id) => ipcRenderer.invoke('jobs:get', id),
    update: (id, b) => ipcRenderer.invoke('jobs:update', id, b),
    delete: (id) => ipcRenderer.invoke('jobs:delete', id),
    analyze: (id) => ipcRenderer.invoke('jobs:analyze', id),
  },
});
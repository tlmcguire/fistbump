// Tauri v2 bridge implementing `window.api` for the renderer.
// Skips initialization when a test harness or mock already defines `window.api`.
(() => {
  if (window.api || !window.__TAURI__) return;

  const { invoke } = window.__TAURI__.core;
  const { listen } = window.__TAURI__.event;
  const { getCurrentWebview } = window.__TAURI__.webview;

  const call = (channel, ...args) => invoke('ipc_call', { channel, args });

  const bytesToBase64 = (input) => {
    const view = input instanceof ArrayBuffer
      ? new Uint8Array(input)
      : ArrayBuffer.isView(input)
        ? new Uint8Array(input.buffer, input.byteOffset, input.byteLength)
        : null;
    if (!view) return '';
    let binary = '';
    const chunk = 0x8000;
    for (let i = 0; i < view.length; i += chunk) {
      binary += String.fromCharCode(...view.subarray(i, i + chunk));
    }
    return btoa(binary);
  };

  const detectPlatform = () => {
    const raw = window.__TAURI_INTERNALS__?.metadata?.currentPlatform || '';
    if (raw === 'macos' || raw === 'darwin') return 'darwin';
    if (raw === 'windows' || raw === 'win32') return 'win32';
    if (raw === 'linux') return 'linux';
    const ua = navigator.userAgent || '';
    if (ua.includes('Mac')) return 'darwin';
    if (ua.includes('Win')) return 'win32';
    return 'linux';
  };

  const platform = detectPlatform();
  const isMac = platform === 'darwin';

  // Block browser-style keyboard zoom shortcuts (Cmd/Ctrl + +/-/0) while keeping Settings text size working.
  window.addEventListener('keydown', (event) => {
    const mod = isMac ? event.metaKey : event.ctrlKey;
    if (mod && ['+', '=', '-', '_', '0'].includes(event.key)) {
      event.preventDefault();
    }
  }, true);

  window.api = {
    platform,

    setZoom: (factor) => {
      if ([0.9, 1, 1.15, 1.3].includes(factor)) {
        getCurrentWebview().setZoom(factor).catch(() => {});
      }
    },

    onBackendRestarted: (cb) => {
      listen('backend:restarted', () => cb());
    },

    error: {
      details: () => call('error:details'),
      copyDetails: () => call('error:copyDetails'),
      quit: () => call('error:quit'),
    },

    connectors: {
      link: (url) => call('connectors:link', url),
      posting: (board, jobId) => call('connectors:posting', board, jobId),
      search: (query) => call('connectors:search', query),
      suggest: (query) => call('connectors:suggest', query),
      list: () => call('connectors:list'),
      categories: () => call('connectors:categories'),
      logo: (url, board) => call('connectors:logo', url, board),
      cancelSearch: () => call('connectors:cancel-search'),
      import: (postings) => call('connectors:import', postings),
    },

    resume: {
      get: () => call('resume:get'),
      putProfile: (profile) => call('resume:putProfile', profile),
      preview: () => call('resume:preview'),
      createExperience: (entry) => call('experiences:create', entry),
      updateExperience: (id, entry) => call('experiences:update', id, entry),
      deleteExperience: (id) => call('experiences:delete', id),
      createEducation: (entry) => call('education:create', entry),
      updateEducation: (id, entry) => call('education:update', id, entry),
      deleteEducation: (id) => call('education:delete', id),
      importFile: () => call('resume:importFile'),
      importData: (filename, bytes) => call('resume:importData', filename, bytesToBase64(bytes)),
      importText: (text) => call('resume:importText', text),
      importApply: (body) => call('resume:importApply', body),
      audit: (style) => call('resume:audit', style),
      export: (format, style) => call('resume:export', format, style),
      previewPdf: (style) => call('resume:previewPdf', style),
    },

    resumes: {
      list: () => call('resumes:list'),
      get: (id) => call('resumes:get', id),
      create: (body) => call('resumes:create', body),
      update: (id, body) => call('resumes:update', id, body),
      delete: (id) => call('resumes:delete', id),
    },

    revisions: {
      create: (body) => call('revisions:create', body),
      list: (jobId) => call('revisions:list', jobId),
      get: (id) => call('revisions:get', id),
      cancel: (id) => call('revisions:cancel', id),
      delete: (id) => call('revisions:delete', id),
    },

    suggestions: {
      accept: (rid, sid) => call('suggestions:accept', rid, sid),
      reject: (rid, sid) => call('suggestions:reject', rid, sid),
      edit: (rid, sid, text) => call('suggestions:edit', rid, sid, text),
    },

    tailored: {
      create: (revisionId) => call('tailored:create', revisionId),
      createBlank: (jobId, baseResumeId) => call('tailored:createBlank', jobId, baseResumeId),
      list: (jobId) => call('tailored:list', jobId),
      get: (id) => call('tailored:get', id),
      update: (id, content) => call('tailored:update', id, content),
      diff: (id) => call('tailored:diff', id),
      delete: (id) => call('tailored:delete', id),
      export: (id, format, style) => call('tailored:export', id, format, style),
      previewPdf: (id, style) => call('tailored:previewPdf', id, style),
    },

    ai: {
      status: () => call('ai:status'),
      setMode: (mode) => call('ai:setMode', mode),
      setRemote: (remote) => call('ai:setRemote', remote),
      clearRemote: () => call('ai:clearRemote'),
      test: (engine) => call('ai:test', engine),
    },

    shell: {
      showItem: (filePath) => call('shell:showItem', filePath),
    },

    settings: {
      get: () => call('settings:get'),
      put: (values) => call('settings:put', values),
    },

    storage: {
      report: () => call('storage:report'),
      cleanup: (payload) => call('storage:cleanup', payload),
    },

    app: {
      info: () => call('app:info'),
      checkForUpdates: () => call('app:check-updates'),
      showDataFolder: () => call('app:show-data-folder'),
      openReleases: (url) => call('app:open-releases', url),
      downloadUpdate: () => call('app:download-update'),
      cancelUpdate: () => call('app:cancel-update'),
      openUpdate: () => call('app:open-update'),
      installUpdate: () => call('app:install-update'),
      onUpdateProgress: (cb) => {
        listen('update:progress', (event) => {
          const payload = event.payload || {};
          cb(payload.received ?? 0, payload.total ?? 0);
        });
      },
      onOpenSettings: (cb) => {
        listen('app:open-settings', () => cb());
      },
    },

    //opt-in desktop integration (desktop.rs): global shortcut, menu-bar item, notifications, deep links
    events: {
      list: (q) => call('events:list', q || {}),
      create: (b) => call('events:create', b),
      update: (id, b) => call('events:update', id, b),
      delete: (id) => call('events:delete', id),
    },
    calendar: {
      feeds: () => call('calendar:feeds'),
      addFeed: (url) => call('calendar:add-feed', url),
      clearFeeds: () => call('calendar:clear-feeds'),
      busy: () => call('calendar:busy'),
      openIcs: (name, text) => call('calendar:open-ics', name, text),
    },

    community: {
      similar: (words) => call('relay:similar', words),
      report: (body) => call('relay:report', body),
      shareBoards: (body) => call('relay:share-boards', body),
      boards: () => call('relay:boards'),
      shareListing: (body) => call('relay:share-listing', body),
      listings: () => call('relay:listings'),
    },

    desktop: {
      setShortcut: (on) => call('app:setShortcut', Boolean(on)),
      setTray: (on, info) => call('app:setTray', Boolean(on), info || {}),
      notify: (title, body) => call('app:notify', title, body || ''),
      onFocusSearch: (cb) => { listen('app:focus-search', () => cb()); },
      onDeepLink: (cb) => { listen('app:deep-link', (event) => cb(String(event.payload || ''))); },
      setMenuState: (state) => call('app:setMenuState', state || {}),
      onMenu: (cb) => { listen('app:menu', (event) => cb(String(event.payload || ''))); },
    },

    jobs: {
      parse: (t) => call('jobs:parse', t),
      create: (b) => call('jobs:create', b),
      list: (q, opts) => call('jobs:list', q, opts),
      get: (id) => call('jobs:get', id),
      update: (id, b) => call('jobs:update', id, b),
      delete: (id) => call('jobs:delete', id),
      analyze: (id) => call('jobs:analyze', id),
      analyzeText: (t) => call('jobs:analyzeText', t),
      archive: (id, opts) => call('jobs:archive', id, opts),
      unarchive: (id) => call('jobs:unarchive', id),
      trash: (id) => call('jobs:trash', id),
      restore: (id) => call('jobs:restore', id),
    },

    cost: {
      analyze: (body) => call('cost:analyze', body),
      compare: (body) => call('cost:compare', body),
    },

    models: {
      list: () => call('models:list'),
      get: (id) => call('models:get', id),
      download: (id) => call('models:download', id),
      downloadStatus: (id) => call('models:downloadStatus', id),
      cancelDownload: (id) => call('models:cancelDownload', id),
      delete: (id) => call('models:delete', id),
    },

    applications: {
      list: (filters) => call('applications:list', filters),
      stats: () => call('applications:stats'),
      create: (body) => call('applications:create', body),
      get: (id) => call('applications:get', id),
      update: (id, patch) => call('applications:update', id, patch),
      delete: (id) => call('applications:delete', id),
      bulkStatus: (ids, status) => call('applications:bulkStatus', ids, status),
      archive: (id) => call('applications:archive', id),
      unarchive: (id) => call('applications:unarchive', id),
      bulkArchive: (ids, archived) => call('applications:bulkArchive', ids, archived),
    },

    savedSearches: {
      list: () => call('savedSearches:list'),
      create: (body) => call('savedSearches:create', body),
      markSeen: (id, seenKeys) => call('savedSearches:markSeen', id, seenKeys),
      delete: (id) => call('savedSearches:delete', id),
    },

    drafts: {
      list: (jobId) => call('drafts:list', jobId),
      create: (jobId, body) => call('drafts:create', jobId, body),
      update: (jobId, did, body) => call('drafts:update', jobId, did, body),
      delete: (jobId, did) => call('drafts:delete', jobId, did),
    },
  };

  // Native-style Windows titlebar buttons for frameless Mica windows.
  if (platform === 'win32' && window.__TAURI__.window?.getCurrentWindow) {
    const mountWinControls = () => {
      if (document.querySelector('.win-controls')) return;
      const win = window.__TAURI__.window.getCurrentWindow();
      const bar = document.createElement('div');
      bar.className = 'win-controls';
      bar.setAttribute('aria-label', 'Window controls');
      const addBtn = (label, cls, glyph, onClick) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = `win-btn ${cls}`;
        btn.setAttribute('aria-label', label);
        btn.innerHTML = glyph;
        btn.addEventListener('click', onClick);
        bar.appendChild(btn);
      };
      addBtn('Minimize', 'win-min', '<svg viewBox="0 0 10 10"><path d="M1 5h8"/></svg>', () => win.minimize());
      addBtn('Maximize', 'win-max', '<svg viewBox="0 0 10 10"><rect x="1.5" y="1.5" width="7" height="7"/></svg>', () => win.toggleMaximize());
      addBtn('Close', 'win-close', '<svg viewBox="0 0 10 10"><path d="M2 2l6 6M8 2L2 8"/></svg>', () => win.close());
      document.body.appendChild(bar);
    };
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mountWinControls);
    else mountWinControls();
  }

  // Right-click menu for https: links (Open in Browser / Copy Link).
  let ctxMenu = null;
  const closeCtxMenu = () => {
    if (ctxMenu) {
      ctxMenu.remove();
      ctxMenu = null;
    }
  };
  document.addEventListener('click', closeCtxMenu);
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeCtxMenu(); });
  window.addEventListener('blur', closeCtxMenu);

  document.addEventListener('contextmenu', (event) => {
    closeCtxMenu();
    const link = event.target?.closest?.('a[href]');
    const href = link?.href || '';
    if (href.startsWith('https://')) {
      event.preventDefault();
      const menu = document.createElement('div');
      menu.className = 'tauri-context-menu';
      menu.setAttribute('role', 'menu');
      const addItem = (label, action) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.setAttribute('role', 'menuitem');
        btn.textContent = label;
        btn.addEventListener('click', () => { closeCtxMenu(); action(); });
        menu.appendChild(btn);
      };
      addItem('Open in Browser', () => call('app:open-releases', href));
      addItem('Copy Link', () => navigator.clipboard?.writeText?.(href).catch(() => {}));
      document.body.appendChild(menu);
      const rect = menu.getBoundingClientRect();
      menu.style.left = `${Math.min(event.clientX, window.innerWidth - rect.width - 8)}px`;
      menu.style.top = `${Math.min(event.clientY, window.innerHeight - rect.height - 8)}px`;
      ctxMenu = menu;
      return;
    }
    const editable = event.target?.closest?.('input, textarea, [contenteditable="true"]');
    const hasSelection = Boolean(window.getSelection?.()?.toString?.().trim());
    if (!editable && !hasSelection) {
      event.preventDefault();
    }
  });
})();

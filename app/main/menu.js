// Native application menu and the right-click menu. Commands go to the renderer as 'app:command'.
const { app, Menu, shell, clipboard } = require('electron');

const isMac = process.platform === 'darwin';

function buildMenu({ send, prefs, dataDir, isDev }) {
  const theme = prefs.get('theme') || 'system';
  const themeItem = (id, label) => ({ label, type: 'radio', checked: theme === id, click: () => prefs.set('theme', id) });
  const settings = { label: isMac ? 'Settings…' : 'Settings', accelerator: 'CmdOrCtrl+,', click: () => send('settings') };
  const go = (route, label, key) => ({ label, accelerator: `CmdOrCtrl+${key}`, click: () => send('navigate', route) });
  const template = [
    ...(isMac ? [{
      label: app.name,
      submenu: [{ role: 'about', label: 'About fistbump' }, { type: 'separator' }, settings, { type: 'separator' },
        { role: 'services' }, { type: 'separator' }, { role: 'hide' }, { role: 'hideOthers' }, { role: 'unhide' }, { type: 'separator' }, { role: 'quit' }],
    }] : []),
    {
      label: 'File',
      submenu: [
        { label: 'Paste a Job Posting', accelerator: 'CmdOrCtrl+N', click: () => send('paste-job') },
        { label: 'Import Resume…', accelerator: 'CmdOrCtrl+O', click: () => send('import-resume') },
        { label: 'Download Resume as PDF…', accelerator: 'CmdOrCtrl+Shift+E', click: () => send('export-resume') },
        { type: 'separator' },
        { label: 'Open Data Folder', click: () => shell.openPath(dataDir) },
        ...(isMac ? [] : [{ type: 'separator' }, settings]),
        { type: 'separator' },
        isMac ? { role: 'close' } : { role: 'quit' },
      ],
    },
    {
      label: 'Edit',
      submenu: [
        { role: 'undo' }, { role: 'redo' }, { type: 'separator' },
        { role: 'cut' }, { role: 'copy' }, { role: 'paste' }, { role: 'pasteAndMatchStyle' }, { role: 'selectAll' },
        { type: 'separator' },
        { label: 'Find', accelerator: 'CmdOrCtrl+F', click: () => send('find') },
      ],
    },
    {
      label: 'View',
      submenu: [
        { label: 'Home', accelerator: 'CmdOrCtrl+0', click: () => send('navigate', 'home') },
        go('resume', 'Resume', 1), go('jobs', 'Jobs', 2), go('revisions', 'Tailor', 3), go('tracker', 'Tracker', 4),
        { type: 'separator' },
        { label: 'Theme', submenu: [themeItem('system', 'Match System'), themeItem('light', 'Light'), themeItem('dark', 'Dark')] },
        { type: 'separator' },
        { role: 'resetZoom' }, { role: 'zoomIn' }, { role: 'zoomOut' }, { type: 'separator' }, { role: 'togglefullscreen' },
        ...(isDev ? [{ type: 'separator' }, { role: 'reload' }, { role: 'toggleDevTools' }] : []),
      ],
    },
    { role: 'windowMenu' },
    {
      role: 'help',
      submenu: [
        { label: 'Keyboard Shortcuts', accelerator: 'CmdOrCtrl+/', click: () => send('shortcuts') },
        { label: 'Open Data Folder', click: () => shell.openPath(dataDir) },
      ],
    },
  ];
  return Menu.buildFromTemplate(template);
}

// attachContextMenu gives text fields and selections the right-click menu users expect, with
// spelling suggestions, and links an "Open in Browser" item.
function attachContextMenu(win) {
  win.webContents.on('context-menu', (_e, p) => {
    const items = [];
    if (p.misspelledWord) {
      for (const s of p.dictionarySuggestions.slice(0, 5)) items.push({ label: s, click: () => win.webContents.replaceMisspelling(s) });
      if (!p.dictionarySuggestions.length) items.push({ label: 'No suggestions', enabled: false });
      items.push({ label: 'Add to Dictionary', click: () => win.webContents.session.addWordToSpellCheckerDictionary(p.misspelledWord) }, { type: 'separator' });
    }
    if (/^https?:\/\//.test(p.linkURL || '')) {
      items.push({ label: 'Open Link in Browser', click: () => shell.openExternal(p.linkURL) },
        { label: 'Copy Link', click: () => clipboard.writeText(p.linkURL) }, { type: 'separator' });
    }
    if (p.isEditable) {
      items.push({ role: 'cut', enabled: p.editFlags.canCut }, { role: 'copy', enabled: p.editFlags.canCopy },
        { role: 'paste', enabled: p.editFlags.canPaste }, { type: 'separator' }, { role: 'selectAll' });
    } else if (p.selectionText) {
      items.push({ role: 'copy' });
    }
    while (items.length && items[items.length - 1].type === 'separator') items.pop();
    if (items.length) Menu.buildFromTemplate(items).popup({ window: win });
  });
}

module.exports = { buildMenu, attachContextMenu };

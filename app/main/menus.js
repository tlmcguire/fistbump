// The application menu and the right-click menu. Built from Electron's standard roles so they behave
// like any native app's: copy and paste, window controls, and no developer items in a release build.
const { app, Menu, shell } = require('electron');

const isMac = process.platform === 'darwin';

function appMenu() {
  const dev = !app.isPackaged;
  const template = [
    ...(isMac ? [{
      label: 'Fistbump',
      submenu: [
        { role: 'about', label: 'About Fistbump' },
        { type: 'separator' },
        { role: 'services' },
        { type: 'separator' },
        { role: 'hide', label: 'Hide Fistbump' },
        { role: 'hideOthers' },
        { role: 'unhide' },
        { type: 'separator' },
        { role: 'quit', label: 'Quit Fistbump' },
      ],
    }] : []),
    { label: 'File', submenu: [isMac ? { role: 'close' } : { role: 'quit' }] },
    {
      label: 'Edit',
      submenu: [
        { role: 'undo' }, { role: 'redo' }, { type: 'separator' },
        { role: 'cut' }, { role: 'copy' }, { role: 'paste' }, { role: 'selectAll' },
      ],
    },
    {
      label: 'View',
      submenu: [
        // Reload and developer tools are for development only; zooming the page is not offered.
        ...(dev ? [{ role: 'reload' }, { role: 'forceReload' }, { role: 'toggleDevTools' }, { type: 'separator' }] : []),
        { role: 'togglefullscreen' },
      ],
    },
    { role: 'windowMenu' },
    {
      role: 'help',
      submenu: [{ label: 'Fistbump on GitHub', click: () => shell.openExternal('https://github.com/tlmcguire/fistbump') }],
    },
  ];
  Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}

// contextMenu gives text fields and selected text the usual Cut, Copy and Paste menu, and links an
// "Open in browser" item. Elsewhere a right-click does nothing, as in native apps.
function contextMenu(win, openInBrowser) {
  win.webContents.on('context-menu', (_event, params) => {
    const items = [];
    if (params.linkURL && params.linkURL.startsWith('https:')) {
      items.push({ label: 'Open in Browser', click: () => openInBrowser(params.linkURL) });
      items.push({ label: 'Copy Link', click: () => require('electron').clipboard.writeText(params.linkURL) });
    }
    if (params.isEditable) {
      if (items.length) items.push({ type: 'separator' });
      items.push(
        { role: 'cut', enabled: params.editFlags.canCut },
        { role: 'copy', enabled: params.editFlags.canCopy },
        { role: 'paste', enabled: params.editFlags.canPaste },
        { type: 'separator' },
        { role: 'selectAll', enabled: params.editFlags.canSelectAll },
      );
    } else if (params.selectionText.trim()) {
      if (items.length) items.push({ type: 'separator' });
      items.push({ role: 'copy' });
    }
    if (items.length) Menu.buildFromTemplate(items).popup({ window: win });
  });
}

module.exports = { appMenu, contextMenu };

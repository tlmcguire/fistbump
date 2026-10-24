const { app, BrowserWindow, dialog, ipcMain, nativeTheme, shell } = require('electron');
const path = require('path');
const { Backend } = require('./backend');
const { register } = require('./ipc-handlers');
const windowState = require('./window-state');
const { appMenu, contextMenu } = require('./menus');

app.setName('Fistbump');
// Data stays where earlier builds kept it (named after the package, "fistbump"), so the product name in
// package.json does not move the database, caches and saved searches on case-sensitive file systems.
app.setPath('userData', path.join(app.getPath('appData'), 'fistbump'));

let backend;

// Only https links leave the app, and they open in the user's default browser, never in an Electron window.
function openInBrowser(url) {
  try {
    if (new URL(url).protocol === 'https:') shell.openExternal(url);
  } catch {
    // not a URL; ignore it
  }
}

const isMac = process.platform === 'darwin';
const ICON = path.join(__dirname, '..', '..', 'build', process.platform === 'win32' ? 'icon.ico' : 'icon.png');

// The window follows each platform's conventions so it does not look like a web page in a frame:
// - macOS: the title bar is hidden, the window buttons sit over the sidebar, and the sidebar shows the
//   system's translucent material behind its tint.
// - Windows 11: the system's Mica material behind the window, under the same tint.
// - It opens at its last size and place, and is shown only once painted, so it never flashes white.
function createWindow() {
  const state = windowState.load({ width: 1240, height: 820 });
  const win = new BrowserWindow({
    width: state.width,
    height: state.height,
    ...(Number.isFinite(state.x) ? { x: state.x, y: state.y } : {}),
    minWidth: 760,
    minHeight: 520,
    title: 'Fistbump',
    show: false,
    backgroundColor: isMac ? '#00000000' : '#0b2457',
    ...(isMac ? {
      titleBarStyle: 'hiddenInset',
      trafficLightPosition: { x: 16, y: 19 }, // level with the toolbar's controls
      vibrancy: 'sidebar',
      visualEffectState: 'followWindow',
    } : {}),
    ...(process.platform === 'win32' ? { backgroundMaterial: 'mica' } : {}),
    ...(isMac ? {} : { icon: ICON }), // macOS takes the icon from the app bundle (or the Dock, in development)
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
      spellcheck: false, // red squiggles under job titles and place names look out of place
    },
  });
  if (state.maximized) win.maximize();
  windowState.track(win);
  win.once('ready-to-show', () => win.show());

  // Pages do not zoom with pinch or Ctrl and the scroll wheel, as in native apps.
  win.webContents.setVisualZoomLevelLimits(1, 1);
  win.webContents.on('before-input-event', (event, inputEvent) => {
    const mod = isMac ? inputEvent.meta : inputEvent.control;
    if (mod && ['+', '=', '-', '0'].includes(inputEvent.key) && inputEvent.type === 'keyDown') event.preventDefault();
  });

  // target="_blank" links and window.open: hand them to the default browser and never create a window.
  win.webContents.setWindowOpenHandler(({ url }) => {
    openInBrowser(url);
    return { action: 'deny' };
  });
  // A plain link click, or a file dropped on the window, would replace the app's page. Anything but the
  // app's own file is sent out instead (or ignored, if it is not an https link).
  win.webContents.on('will-navigate', (event, url) => {
    if (url === win.webContents.getURL()) return;
    event.preventDefault();
    openInBrowser(url);
  });
  contextMenu(win, openInBrowser);

  win.loadFile(path.join(__dirname, '..', 'renderer', 'index.html'));
  return win;
}

// One copy of the app at a time: a second would start a second backend on the same database.
if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => {
    const win = BrowserWindow.getAllWindows()[0];
    if (!win) return;
    if (win.isMinimized()) win.restore();
    win.focus();
  });

  //starts the backend, connects the handlers and opens the window
  app.whenReady().then(async () => {
    backend = new Backend({
      binDir: path.join(__dirname, '..', '..', 'bin', `${process.platform}-${process.arch}`),
      dataDir: app.getPath('userData'),
    });
    try {
      await backend.start();
    } catch (err) {
      dialog.showErrorBox('Fistbump could not start', String(err?.message || err));
      app.quit();
      return;
    }
    register(ipcMain, backend);
    nativeTheme.themeSource = 'dark'; // native menus, pickers and the window material match the dark app
    app.setAboutPanelOptions({ applicationName: 'Fistbump', applicationVersion: app.getVersion() });
    appMenu();
    // A packaged app has its icon in its bundle. Running from source, the Dock would show Electron's.
    if (isMac && !app.isPackaged) app.dock.setIcon(path.join(__dirname, '..', '..', 'build', 'icon.png'));
    createWindow();

    // macOS keeps the app running with no window; clicking the Dock icon opens one again.
    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) createWindow();
    });
  });
}

app.on('before-quit', () => backend?.stop());
app.on('window-all-closed', () => {
  if (!isMac) app.quit();
});

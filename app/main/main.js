const { app, BrowserWindow, clipboard, dialog, ipcMain, nativeTheme, safeStorage, shell } = require('electron');
const path = require('path');
const { Backend } = require('./backend');
const { register } = require('./ipc-handlers');
const windowState = require('./window-state');
const { appMenu, contextMenu } = require('./menus');
const updates = require('./updates');
const { createSecrets, restoreRemoteKey } = require('./secrets');
const { showErrorScreen } = require('./error-screen');

app.setName('Fistbump');
// Data stays where earlier builds kept it (named after the package, "fistbump"), so the product name in
// package.json does not move the database, caches and saved searches on case-sensitive file systems.
// FISTBUMP_USER_DATA lets the end-to-end tests use a throwaway folder instead of the real one.
app.setPath('userData', process.env.FISTBUMP_USER_DATA || path.join(app.getPath('appData'), 'fistbump'));

let backend;
// What the error screen shows when the backend will not run. It is filled in before the screen opens.
let startupError = { message: '', stderr: [] };
let errorScreenShown = false;

// The error screen stands in for the app: it says what happened, and shows the last words the
// backend wrote. It is shown once, and the user quits from it.
function showStartupError(message, { quitOnClose = false } = {}) {
  if (errorScreenShown) return;
  errorScreenShown = true;
  startupError = { message, stderr: backend ? backend.stderrLines(40) : [] };
  showErrorScreen({ onClosed: quitOnClose ? () => app.quit() : undefined });
}

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
  // On Windows, hide the menu bar (File, Edit, View and so on). The menu stays set, so its shortcuts,
  // such as Ctrl+, for Settings, still work.
  if (process.platform === 'win32') win.setMenuBarVisibility(false);
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
    // A packaged app carries the backend in its resources; from the source it sits next to the app.
    const binDir = app.isPackaged
      ? path.join(process.resourcesPath, 'bin')
      : path.join(__dirname, '..', '..', 'bin', `${process.platform}-${process.arch}`);
    backend = new Backend({
      binDir,
      dataDir: app.getPath('userData'),
    });
    const secrets = createSecrets(safeStorage, app.getPath('userData'));
    // The handlers are registered before the backend starts, so the error screen can talk to the
    // main process even when the backend never came up.
    register(ipcMain, backend, {
      secrets,
      dialog,
      BrowserWindow,
      shell,
      documentsDir: () => app.getPath('documents'),
      info: () => ({
        version: app.getVersion(),
        electron: process.versions.electron,
        chrome: process.versions.chrome,
        node: process.versions.node,
        platform: process.platform,
        arch: process.arch,
        dataDir: app.getPath('userData'),
      }),
      checkForUpdates: () => updates.checkForUpdates(app.getVersion()),
      showDataFolder: async () => {
        const error = await shell.openPath(app.getPath('userData'));
        return error ? { error: { code: 'internal', message: error, details: null } } : { ok: true };
      },
      // Only GitHub release pages for this project are opened.
      openReleases: (url) => {
        const target = typeof url === 'string' && url.startsWith(updates.RELEASES_PAGE) ? url : updates.RELEASES_PAGE;
        shell.openExternal(target);
        return { ok: true };
      },
      // The error screen: what happened, and the last words from the backend.
      errorDetails: () => startupError,
      copyDetails: () => {
        clipboard.writeText([startupError.message, ...startupError.stderr].join('\n'));
        return { ok: true };
      },
      quit: () => {
        app.quit();
        return { ok: true };
      },
    });

    // The backend came back after a crash: it has a new port and token, so the key is sent again
    // and the views reload their data.
    backend.on('restarted', async () => {
      await restoreRemoteKey(backend, secrets).catch(() => false);
      for (const win of BrowserWindow.getAllWindows()) {
        if (!win.isDestroyed()) win.webContents.send('backend:restarted');
      }
    });

    try {
      await backend.start();
    } catch (err) {
      showStartupError(`The backend could not start. ${String(err?.message || err)}`, { quitOnClose: true });
      return;
    }
    // The stored key goes to the backend at launch, so it never has to be read back from the page.
    await restoreRemoteKey(backend, secrets).catch(() => false);

    // After the app started, a backend that gives up shows the error screen too.
    backend.on('state', (state, extra) => {
      if (state !== 'failed') return;
      showStartupError(extra?.reason === 'too many crashes'
        ? 'The backend crashed too many times in a minute and was not restarted.'
        : 'The backend stopped and could not be restarted.');
    });
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

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

// The installer found by the last update check, and the file once it is downloaded.
const update = { asset: null, file: '', abort: null };

const isMac = process.platform === 'darwin';
const ICON = path.join(__dirname, '..', '..', 'build', process.platform === 'win32' ? 'icon.ico' : 'icon.png');

// The window follows each platform's conventions so it does not look like a web page in a frame:
// - macOS: the title bar is hidden, the window buttons sit over the sidebar, and the sidebar shows the
//   system's translucent material behind its tint.
// - Windows 11: the system's Mica material behind the window, under the same tint, and no title bar or
//   menu bar; the window buttons are drawn over the toolbar.
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
    // Windows: no title bar and no menu bar. The system's minimize, maximize and close buttons stay,
    // drawn over the toolbar so the window can still be closed and snapped.
    ...(process.platform === 'win32' ? {
      backgroundMaterial: 'mica',
      titleBarStyle: 'hidden',
      titleBarOverlay: { color: '#00000000', symbolColor: '#f2f5fb', height: 52 }, // 52 is the toolbar's height
    } : {}),
    ...(isMac ? {} : { icon: ICON }), // macOS takes the icon from the app bundle (or the Dock, in development)
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
      spellcheck: false, // red squiggles under job titles and place names look out of place
    },
  });
  // On Windows, hide the menu bar (File, Edit, View and so on) for good, so Alt does not bring it back. The
  // menu stays set, so its shortcuts, such as Ctrl+, for Settings, still work.
  if (process.platform === 'win32') {
    win.setAutoHideMenuBar(false);
    win.setMenuBarVisibility(false);
  }
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
      checkForUpdates: async () => {
        const result = await updates.checkForUpdates(app.getVersion());
        // Main keeps the installer to fetch; the renderer only learns whether there is one and how big.
        update.asset = result.status === 'available' ? result.asset : null;
        update.file = '';
        const { asset, ...rest } = result;
        return { ...rest, downloadable: Boolean(asset), size: asset ? asset.size : 0 };
      },
      downloadUpdate: async () => {
        if (!update.asset) return { error: { code: 'not_found', message: 'There is no update to download. Check for updates first.', details: null } };
        if (update.abort) return { error: { code: 'conflict', message: 'The update is already downloading.', details: null } };
        const controller = new AbortController();
        update.abort = controller;
        let last = 0;
        try {
          const { path: file } = await updates.downloadUpdate(update.asset, app.getPath('downloads'), {
            signal: controller.signal,
            onProgress: (received, total) => {
              const now = Date.now();
              if (now - last < 100 && received !== total) return; // about ten updates a second is plenty
              last = now;
              for (const win of BrowserWindow.getAllWindows()) {
                if (!win.isDestroyed()) win.webContents.send('update:progress', received, total);
              }
            },
          });
          update.file = file;
          return { ok: true, name: path.basename(file), folder: path.dirname(file) };
        } catch (error) {
          if (controller.signal.aborted) return { error: { code: 'canceled', message: 'The download was canceled.', details: null } };
          return { error: { code: 'upstream_error', message: error.message || 'The download failed. Try again.', details: null } };
        } finally {
          update.abort = null;
        }
      },
      cancelUpdate: () => {
        if (update.abort) update.abort.abort();
        return { ok: true };
      },
      // Installs the downloaded update in place of this app and starts the new version: Windows runs the
      // installer silently, macOS swaps the app bundle once this one has quit, Linux replaces the AppImage.
      // When that cannot be done here (say the app runs from the disk image), it says why, and Open installer
      // is the way on.
      installUpdate: async () => {
        if (!update.file) return { error: { code: 'not_found', message: 'Download the update first.', details: null } };
        const fs = require('node:fs');
        const os = require('node:os');
        const canWrite = (target) => { try { fs.accessSync(target, fs.constants.W_OK); return true; } catch { return false; } };
        const scriptPath = path.join(os.tmpdir(), `fistbump-update-${process.pid}.sh`);
        const plan = updates.installPlan({
          platform: process.platform, file: update.file, execPath: process.execPath,
          appImage: process.env.APPIMAGE || '', pid: process.pid, scriptPath, canWrite,
        });
        if (plan.kind === 'manual') return { error: { code: 'manual', message: plan.reason, details: null } };
        try {
          if (plan.kind === 'appimage') {
            const next = `${plan.target}.update`;
            fs.copyFileSync(update.file, next);
            fs.chmodSync(next, 0o755);
            fs.renameSync(next, plan.target);
          }
          if (process.platform === 'darwin') fs.writeFileSync(scriptPath, updates.MAC_INSTALL, { mode: 0o700 });
          // The backend is stopped first, so its files are free to be replaced.
          await backend?.stop();
          if (plan.kind === 'run') {
            require('node:child_process').spawn(plan.command, plan.args, { detached: true, stdio: 'ignore' }).unref();
          } else {
            app.relaunch({ execPath: plan.target, args: process.argv.slice(1) });
          }
        } catch (error) {
          return { error: { code: 'internal', message: `The update could not be installed: ${error.message}`, details: null } };
        }
        setTimeout(() => app.exit(0), 300); // after the answer reaches the page
        return { ok: true };
      },
      // Opens the downloaded installer by hand: the installer on Windows, the disk image on macOS, the file's
      // folder on Linux. It is the way on when the update cannot install itself.
      openUpdate: async () => {
        if (!update.file) return { error: { code: 'not_found', message: 'Download the update first.', details: null } };
        if (process.platform === 'linux') {
          shell.showItemInFolder(update.file);
          return { ok: true };
        }
        const error = await shell.openPath(update.file);
        return error ? { error: { code: 'internal', message: error, details: null } } : { ok: true };
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

const { app, BrowserWindow, Menu, dialog, ipcMain, nativeTheme, shell } = require('electron');
const path = require('path');

// A separate data directory (tests, a second profile) also gets its own preferences, stored keys and
// single-instance lock, so it can never change the real app's settings. Must run before anything reads
// userData.
if (process.env.FISTBUMP_DATA_DIR) app.setPath('userData', path.join(process.env.FISTBUMP_DATA_DIR, 'electron'));

const { Backend } = require('./backend');
const { register, deliverSecrets } = require('./ipc-handlers');
const prefs = require('./prefs');
const { buildMenu, attachContextMenu } = require('./menu');
const customCss = require('./custom-css');

let win = null;
let backend = null;
let quitting = false;

const isMac = process.platform === 'darwin';
const dataDir = process.env.FISTBUMP_DATA_DIR || path.join(app.getPath('userData'), 'data');
const binDir = app.isPackaged
  ? path.join(process.resourcesPath, 'bin')
  : path.join(__dirname, '..', '..', 'bin', `${process.platform}-${process.arch}`);
const indexFile = path.join(__dirname, '..', 'renderer', 'index.html');
const NAVY = '#07235b';
const background = () => (nativeTheme.shouldUseDarkColors ? '#0a1330' : '#f5f6fa');
const overlayColors = () => ({ color: background(), symbolColor: nativeTheme.shouldUseDarkColors ? '#e8ebf4' : '#0e1a3a', height: 52 });

function errorPage(message) {
  const esc = (s) => String(s).replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' })[c]);
  return (
    'data:text/html;charset=utf-8,' +
    encodeURIComponent(`<!doctype html><meta charset="utf-8"><title>fistbump</title>
<body style="font:15px -apple-system,system-ui,sans-serif;margin:0;background:#f3f4f9;color:#0f2048">
<div style="background:${NAVY};height:58px;-webkit-app-region:drag"></div>
<main style="max-width:680px;margin:56px auto;padding:0 24px">
<h1 style="font-size:26px;margin:0 0 8px">fistbump could not start</h1>
<p style="color:#5a6486">The part of the app that stores your data did not start. Quit and open fistbump again. If it keeps happening, include the details below when you report it.</p>
<pre style="background:#fff;border:1px solid #d6dae7;border-left:4px solid #cfa516;padding:14px;border-radius:8px;white-space:pre-wrap;font-size:12px">${esc(message)}</pre>
</main></body>`)
  );
}

// Window size and position persist between launches; a saved position off every screen is ignored
// by Electron and falls back to centered.
function windowBounds() {
  const b = prefs.get('window') || {};
  return { width: b.width || 1240, height: b.height || 840, x: b.x, y: b.y };
}

function createWindow() {
  const bounds = windowBounds();
  win = new BrowserWindow({
    ...bounds,
    minWidth: 960,
    minHeight: 640,
    title: 'fistbump',
    show: false,
    // macOS: a translucent source-list sidebar (vibrancy). The page paints its own content background,
    // so the window itself is transparent there.
    backgroundColor: isMac ? '#00000000' : background(),
    vibrancy: isMac ? 'sidebar' : undefined,
    visualEffectState: isMac ? 'followWindow' : undefined,
    // No system title bar: on macOS the traffic lights sit at the top of the sidebar; on Windows and
    // Linux the window buttons overlay the top-right of the content toolbar, in its colors.
    titleBarStyle: isMac ? 'hiddenInset' : 'hidden',
    trafficLightPosition: isMac ? { x: 18, y: 18 } : undefined,
    titleBarOverlay: isMac ? undefined : overlayColors(),
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webSecurity: true,
      spellcheck: true,
    },
  });
  if (prefs.get('window')?.maximized) win.maximize();
  win.once('ready-to-show', () => win.show());
  // The window only ever shows our own page. External links open in the system browser; anything else,
  // such as a file dropped on the window, is ignored.
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:\/\//.test(url)) shell.openExternal(url);
    return { action: 'deny' };
  });
  win.webContents.on('will-navigate', (e, url) => {
    let target = '';
    try { target = decodeURIComponent(new URL(url).pathname); } catch { /* not a URL */ }
    if (url.startsWith('file://') && target.endsWith('/renderer/index.html')) return;
    e.preventDefault();
    if (/^https?:\/\//.test(url)) shell.openExternal(url);
  });
  attachContextMenu(win);
  customCss.attach(win);
  const saveBounds = () => {
    if (!win || win.isMinimized()) return;
    prefs.set('window', { ...(win.isMaximized() ? prefs.get('window') : win.getBounds()), maximized: win.isMaximized() });
  };
  win.on('close', saveBounds);
  win.on('closed', () => (win = null));
  nativeTheme.on('updated', () => {
    if (!isMac) win?.setBackgroundColor(background());
    if (!isMac) win?.setTitleBarOverlay?.(overlayColors());
  });
  return win;
}

function send(command, arg) {
  if (!win) return;
  if (win.isMinimized()) win.restore();
  win.show();
  win.webContents.send('app:command', { command, arg });
}

function refreshMenu() {
  Menu.setApplicationMenu(buildMenu({ send, prefs, dataDir, isDev: !app.isPackaged }));
}

async function boot() {
  prefs.apply(); // before any window exists, so the first paint uses the chosen theme
  app.setAboutPanelOptions({
    applicationName: 'fistbump',
    applicationVersion: app.getVersion(),
    copyright: 'Your data. Your career. Your terms.',
    credits: 'Tyler McGuire, Tyler Goodman, Joe Cooney',
  });
  prefs.onChange((key) => { if (key === 'theme') { refreshMenu(); send('theme-changed', prefs.get('theme')); } });
  refreshMenu();
  backend = new Backend({ binDir, dataDir });
  backend.on('ready', () => deliverSecrets(backend).catch((e) => console.error('could not deliver secrets:', e.message)));
  backend.on('state', (s) => {
    if (s === 'failed' && !quitting && win) win.loadURL(errorPage(backend.tail() || 'The backend stopped repeatedly.'));
  });
  register({ ipcMain, backend, dialog, getWindow: () => win, app, prefs, shell, dataDir, customCss });
  try {
    await backend.start();
  } catch (err) {
    createWindow();
    win.loadURL(errorPage(err.message));
    return;
  }
  createWindow();
  win.loadFile(indexFile);
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => {
    if (win) {
      if (win.isMinimized()) win.restore();
      win.focus();
    }
  });
  app.whenReady().then(boot);
  app.on('window-all-closed', () => {
    if (!isMac) app.quit();
  });
  app.on('activate', () => {
    if (!win && backend?.state === 'ready') {
      createWindow();
      win.loadFile(indexFile);
    }
  });
  app.on('before-quit', (e) => {
    if (quitting || !backend) return;
    e.preventDefault();
    quitting = true;
    backend.stop().finally(() => app.exit(0));
  });
}

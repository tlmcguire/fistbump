const { app, BrowserWindow, ipcMain, shell } = require('electron');
const path = require('path');
const { Backend } = require('./backend');
const { register } = require('./ipc-handlers');

let backend;

// Only https links leave the app, and they open in the user's default browser, never in an Electron window.
function openInBrowser(url) {
  try {
    if (new URL(url).protocol === 'https:') shell.openExternal(url);
  } catch {
    // not a URL; ignore it
  }
}

//starts backend and connects handlers and opens the window
app.whenReady().then(async () => {
  backend = new Backend({
    binDir: path.join(__dirname, '..', '..', 'bin', `${process.platform}-${process.arch}`),
    dataDir: app.getPath('userData'),
  });
  await backend.start();
  register(ipcMain, backend);
  const win = new BrowserWindow({
    width: 1000,
    height: 700,
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
    },
  });

  // target="_blank" links and window.open: hand them to the default browser and never create a window.
  win.webContents.setWindowOpenHandler(({ url }) => {
    openInBrowser(url);
    return { action: 'deny' };
  });
  // A plain link click would replace the app's page. Anything but the app's own file is sent out instead.
  win.webContents.on('will-navigate', (event, url) => {
    if (url === win.webContents.getURL()) return;
    event.preventDefault();
    openInBrowser(url);
  });

  win.loadFile(path.join(__dirname, '..', 'renderer', 'index.html'));
});

app.on('before-quit', () => backend?.stop());
app.on('window-all-closed', () => app.quit());
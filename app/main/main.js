const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('path');
const { Backend } = require('./backend');
const { register } = require('./ipc-handlers');

let backend;

//starts backend and connects handlers and opens the window
app.whenReady().then(async () => {
  backend = new Backend({
    binDir: path.join(__dirname, '..', '..', 'bin', `${process.platform}-${process.arch}`),
    dataDir: app.getPath('userData'),
  });
  await backend.start();
  register(ipcMain, backend);
  new BrowserWindow({
    width: 1000,
    height: 700,
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
    },
  }).loadFile(path.join(__dirname, '..', 'renderer', 'index.html'));
});

app.on('before-quit', () => backend?.stop());
app.on('window-all-closed', () => app.quit());
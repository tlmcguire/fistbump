// The startup error screen: a window of its own, so it works while the backend is dead.
// The page asks the main process for the details over IPC; it never needs the backend.
const path = require('node:path');
const { BrowserWindow } = require('electron');

const PAGE = path.join(__dirname, '..', 'renderer', 'error.html');

// showErrorScreen opens the error window and returns it. The details are not in the URL: the page
// asks for them with the 'error:details' channel, so a long stderr tail stays readable.
// onClosed runs when the window is closed, so the app can quit if there is nothing else to show.
function showErrorScreen({ onClosed } = {}) {
  const win = new BrowserWindow({
    width: 720,
    height: 600,
    title: 'Fistbump could not start',
    show: false,
    backgroundColor: '#0b2457',
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
      spellcheck: false,
    },
  });
  win.once('ready-to-show', () => win.show());
  win.on('closed', () => onClosed?.());
  win.loadFile(PAGE);
  return win;
}

module.exports = { showErrorScreen, PAGE };

// Remembers the window's size, position and maximised state between launches, as native apps do.
// A saved position is used only while it is still on a connected display.
const fs = require('fs');
const path = require('path');
const { app, screen } = require('electron');

const FILE = () => path.join(app.getPath('userData'), 'window-state.json');

function load(defaults) {
  try {
    const saved = JSON.parse(fs.readFileSync(FILE(), 'utf8'));
    const bounds = { width: saved.width, height: saved.height, x: saved.x, y: saved.y };
    if (![bounds.width, bounds.height].every(Number.isFinite)) return { ...defaults };
    const visible = screen.getAllDisplays().some(({ workArea: a }) =>
      Number.isFinite(bounds.x) && Number.isFinite(bounds.y) &&
      bounds.x < a.x + a.width - 100 && bounds.x + bounds.width > a.x + 100 &&
      bounds.y >= a.y - 10 && bounds.y < a.y + a.height - 100);
    if (!visible) {
      delete bounds.x;
      delete bounds.y;
    }
    return { ...defaults, ...bounds, maximized: saved.maximized === true };
  } catch {
    return { ...defaults };
  }
}

// track saves the state whenever the window stops moving or resizing, and when it closes.
function track(win) {
  let timer = null;
  const save = () => {
    if (win.isDestroyed()) return;
    const state = { ...(win.isMaximized() ? win.getNormalBounds() : win.getBounds()), maximized: win.isMaximized() };
    try {
      fs.writeFileSync(FILE(), JSON.stringify(state));
    } catch {
      // not being able to remember the size is harmless
    }
  };
  const later = () => {
    clearTimeout(timer);
    timer = setTimeout(save, 400);
  };
  win.on('resize', later);
  win.on('move', later);
  win.on('close', save);
}

module.exports = { load, track };

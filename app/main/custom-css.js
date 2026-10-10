// User stylesheet, kept in userData/custom.css. TEMPLATE is mirrored in app/renderer/js/custom-css-template.js.
// The page's content security policy still applies: custom CSS can restyle the app but cannot load
// remote fonts or images.
const { app } = require('electron');
const fs = require('fs');
const path = require('path');

const MAX = 200_000;
const TEMPLATE = `/* fistbump custom CSS
 * Changes apply as soon as you save. Turn it off in Settings > Appearance.
 * Most looks can be changed with design tokens. Uncomment one to try it:
 *
 * :root {
 *   --accent: #0b6e4f;
 *   --highlight: #f2c94c;
 *   --radius: 6px;
 * }
 */
`;

const file = () => path.join(app.getPath('userData'), 'custom.css');

function read() {
  try { return fs.readFileSync(file(), 'utf8'); } catch { return ''; }
}

function write(css) {
  if (typeof css !== 'string' || css.length > MAX) throw new Error('Custom CSS must be text under 200 KB.');
  fs.mkdirSync(path.dirname(file()), { recursive: true });
  fs.writeFileSync(file(), css);
}

// apply tells the page to reload the stylesheet. The page applies it as a constructable stylesheet,
// which comes after the app's own styles, so plain rules such as ":root { --accent: ... }" win
// without !important. (insertCSS sheets come first in the cascade and would lose ties.)
async function apply(win) {
  if (!win || win.isDestroyed()) return;
  win.webContents.send('app:command', { command: 'custom-css-changed' });
}

// attach re-applies the stylesheet when the file changes on disk, so editing in an external editor
// updates the window as you save.
function attach(win) {
  let timer = null;
  try {
    fs.mkdirSync(path.dirname(file()), { recursive: true });
    fs.watch(path.dirname(file()), (_ev, name) => {
      if (name !== 'custom.css') return;
      clearTimeout(timer);
      timer = setTimeout(() => apply(win), 120);
    });
  } catch { /* watching is a convenience */ }
}

function ensureFile() {
  if (!fs.existsSync(file())) write(TEMPLATE);
  return file();
}

module.exports = { read, write, apply, attach, ensureFile, file, TEMPLATE, MAX };

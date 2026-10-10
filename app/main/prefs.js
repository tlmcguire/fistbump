// Small UI preferences kept by the main process (not secrets, not job data), in userData/prefs.json.
const { app, nativeTheme } = require('electron');
const fs = require('fs');
const path = require('path');

const DEFAULTS = { theme: 'system' };
let cache = null;
const listeners = [];

function file() {
  return path.join(app.getPath('userData'), 'prefs.json');
}

function load() {
  if (cache) return cache;
  try {
    cache = { ...DEFAULTS, ...JSON.parse(fs.readFileSync(file(), 'utf8')) };
  } catch {
    cache = { ...DEFAULTS };
  }
  return cache;
}

function apply() {
  const t = load().theme;
  nativeTheme.themeSource = ['light', 'dark'].includes(t) ? t : 'system';
}

function get(key) {
  return load()[key];
}

function set(key, value) {
  load()[key] = value;
  fs.mkdirSync(path.dirname(file()), { recursive: true });
  fs.writeFileSync(file(), JSON.stringify(cache, null, 2));
  if (key === 'theme') apply();
  for (const fn of listeners) fn(key, value);
}

function onChange(fn) {
  listeners.push(fn);
}

module.exports = { get, set, apply, onChange };

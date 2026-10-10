// Stores user-supplied secrets (remote AI key, Hugging Face token) encrypted with safeStorage.
// Only ciphertext reaches disk. Plaintext lives in memory here and in Go, never in SQLite or logs.
const { app, safeStorage } = require('electron');
const fs = require('fs');
const path = require('path');

const memory = new Map(); // used when encryption is unavailable, for this session only
let cache = null;

function file() {
  return path.join(app.getPath('userData'), 'secrets.json');
}

function load() {
  if (cache) return cache;
  try {
    cache = JSON.parse(fs.readFileSync(file(), 'utf8'));
  } catch {
    cache = {};
  }
  return cache;
}

function canPersist() {
  if (!safeStorage.isEncryptionAvailable()) return false;
  // On Linux without a keyring Electron falls back to a plaintext backend. Refuse to save in that case.
  if (process.platform === 'linux' && safeStorage.getSelectedStorageBackend() === 'basic_text') return false;
  return true;
}

function set(name, value) {
  memory.set(name, value);
  if (!canPersist()) return { persisted: false };
  const all = load();
  all[name] = safeStorage.encryptString(value).toString('base64');
  fs.mkdirSync(path.dirname(file()), { recursive: true });
  fs.writeFileSync(file(), JSON.stringify(all), { mode: 0o600 });
  return { persisted: true };
}

function get(name) {
  if (memory.has(name)) return memory.get(name);
  const enc = load()[name];
  if (!enc || !canPersist()) return null;
  try {
    const v = safeStorage.decryptString(Buffer.from(enc, 'base64'));
    memory.set(name, v);
    return v;
  } catch {
    return null;
  }
}

function remove(name) {
  memory.delete(name);
  const all = load();
  if (name in all) {
    delete all[name];
    fs.writeFileSync(file(), JSON.stringify(all), { mode: 0o600 });
  }
}

module.exports = { set, get, remove, has: (n) => get(n) !== null, canPersist };

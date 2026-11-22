// Keeps the remote AI key encrypted on this computer with the system's own store (Keychain, DPAPI, libsecret).
// The backend only ever holds the key in memory, so main sends it again at each launch.
const fs = require('node:fs');
const path = require('node:path');

// safeStorage is Electron's; tests pass a fake. dir is the user-data folder.
function createSecrets(safeStorage, dir) {
  const file = path.join(dir, 'ai-remote-key.bin');
  const usable = () => Boolean(safeStorage?.isEncryptionAvailable?.());
  return {
    canStore: usable,
    get() {
      if (!usable()) return null;
      try {
        return safeStorage.decryptString(fs.readFileSync(file));
      } catch {
        return null; // no key saved, or one this machine cannot read
      }
    },
    set(key) {
      if (!usable()) return false;
      fs.writeFileSync(file, safeStorage.encryptString(key), { mode: 0o600 });
      return true;
    },
    clear() {
      fs.rmSync(file, { force: true });
    },
  };
}

// At launch: when a remote provider is configured and a key was saved, hand the key to the backend.
async function restoreRemoteKey(backend, secrets) {
  const status = await backend.call('GET', '/v1/ai/status');
  const remote = status?.remote;
  if (!remote?.base_url || remote.has_key) return false;
  const key = secrets.get();
  if (!key) return false;
  const res = await backend.call('PUT', '/v1/ai/remote', { base_url: remote.base_url, model: remote.model || '', api_key: key });
  return !res?.error;
}

module.exports = { createSecrets, restoreRemoteKey };

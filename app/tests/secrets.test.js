// The remote AI key is kept encrypted and handed back to the backend at launch.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createSecrets, restoreRemoteKey } = require('../main/secrets');

const fakeStore = (available = true) => ({
  isEncryptionAvailable: () => available,
  encryptString: (text) => Buffer.from(text).reverse(),
  decryptString: (buf) => Buffer.from(buf).reverse().toString(),
});

test('a key is saved encrypted, read back, and cleared', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'fb-secrets-'));
  const secrets = createSecrets(fakeStore(), dir);
  assert.equal(secrets.get(), null);
  assert.equal(secrets.set('sk-secret'), true);
  assert.ok(!fs.readFileSync(path.join(dir, 'ai-remote-key.bin'), 'utf8').includes('sk-secret'));
  assert.equal(secrets.get(), 'sk-secret');
  secrets.clear();
  assert.equal(secrets.get(), null);
});

test('without system encryption nothing is written', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'fb-secrets-'));
  const secrets = createSecrets(fakeStore(false), dir);
  assert.equal(secrets.set('sk-secret'), false);
  assert.deepEqual(fs.readdirSync(dir), []);
});

test('at launch the saved key goes to a configured provider that has none', async () => {
  const calls = [];
  const backend = { call: async (m, u, b) => { calls.push([m, u, b]); return m === 'GET' ? { remote: { base_url: 'https://x/v1', model: 'm', has_key: false } } : {}; } };
  const secrets = { get: () => 'sk-1' };
  assert.equal(await restoreRemoteKey(backend, secrets), true);
  assert.deepEqual(calls[1], ['PUT', '/v1/ai/remote', { base_url: 'https://x/v1', model: 'm', api_key: 'sk-1' }]);

  const none = { call: async () => ({ remote: { base_url: '', has_key: false } }) };
  assert.equal(await restoreRemoteKey(none, secrets), false);
});

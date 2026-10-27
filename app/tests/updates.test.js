// The update check compares versions and reads GitHub's answer without downloading anything.
const test = require('node:test');
const assert = require('node:assert');
const { checkForUpdates, compareVersions } = require('../main/updates');

const reply = (status, body) => async () => ({ status, ok: status >= 200 && status < 300, json: async () => body });

test('compares versions, with pre-releases before their release', () => {
  assert.ok(compareVersions('0.1.0', '0.2.0') < 0);
  assert.ok(compareVersions('v1.10.0', '1.9.3') > 0);
  assert.strictEqual(compareVersions('1.2.3', 'v1.2.3'), 0);
  assert.ok(compareVersions('1.0.0-beta.1', '1.0.0') < 0);
  assert.ok(compareVersions('1.0.0-beta.1', '1.0.0-beta.2') < 0);
});

test('reports a newer release with its page', async () => {
  const r = await checkForUpdates('0.1.0', reply(200, { tag_name: 'v0.2.0', html_url: 'https://github.com/tlmcguire/fistbump/releases/tag/v0.2.0', published_at: '2026-11-10T00:00:00Z' }));
  assert.strictEqual(r.status, 'available');
  assert.strictEqual(r.latest, '0.2.0');
  assert.match(r.url, /releases\/tag\/v0\.2\.0$/);
});

test('says when the app is up to date, or nothing is released', async () => {
  assert.strictEqual((await checkForUpdates('0.2.0', reply(200, { tag_name: 'v0.2.0' }))).status, 'current');
  assert.strictEqual((await checkForUpdates('0.3.0-beta.1', reply(200, { tag_name: 'v0.2.0' }))).status, 'current');
  assert.strictEqual((await checkForUpdates('0.1.0', reply(404, {}))).status, 'none');
});

test('turns failures into a message, never an exception', async () => {
  const offline = await checkForUpdates('0.1.0', async () => { throw new Error('offline'); });
  assert.strictEqual(offline.status, 'error');
  assert.strictEqual((await checkForUpdates('0.1.0', reply(500, {}))).status, 'error');
  assert.strictEqual((await checkForUpdates('0.1.0', reply(200, { tag_name: 'latest' }))).status, 'error');
  // A release page link that is not on GitHub is replaced by the releases page.
  const odd = await checkForUpdates('0.1.0', reply(200, { tag_name: 'v9.0.0', html_url: 'https://evil.example/x' }));
  assert.strictEqual(odd.url, 'https://github.com/tlmcguire/fistbump/releases');
});

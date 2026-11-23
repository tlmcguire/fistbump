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

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { pickAsset, downloadUpdate } = require('../main/updates');

const BASE = 'https://github.com/tlmcguire/fistbump/releases/download/v0.2.0/';
const asset = (name, extra = {}) => ({ name, browser_download_url: BASE + name, size: 5, ...extra });

test('picks this computer\'s installer, and nothing off GitHub', () => {
  const assets = [asset('FistBump-0.2.0-Windows-x64.exe'), asset('FistBump-0.2.0-macOS-arm64.dmg'), asset('FistBump-0.2.0-macOS-x64.dmg'), asset('FistBump-0.2.0-Linux-x86_64.AppImage'), asset('github-banner.png')];
  assert.strictEqual(pickAsset(assets, '0.2.0', 'win32', 'x64').name, 'FistBump-0.2.0-Windows-x64.exe');
  assert.strictEqual(pickAsset(assets, '0.2.0', 'darwin', 'arm64').name, 'FistBump-0.2.0-macOS-arm64.dmg');
  assert.strictEqual(pickAsset(assets, '0.2.0', 'darwin', 'x64').name, 'FistBump-0.2.0-macOS-x64.dmg');
  assert.strictEqual(pickAsset(assets, '0.2.0', 'linux', 'x64').name, 'FistBump-0.2.0-Linux-x86_64.AppImage');
  assert.strictEqual(pickAsset(assets, '0.2.0', 'linux', 'arm64'), null); // none is built for it
  assert.strictEqual(pickAsset([asset('FistBump-0.2.0-Windows-x64.exe', { browser_download_url: 'https://evil.example/a.exe' })], '0.2.0', 'win32', 'x64'), null);
  const sum = 'a'.repeat(64);
  assert.strictEqual(pickAsset([asset('FistBump-0.2.0-Windows-x64.exe', { digest: `sha256:${sum}` })], '0.2.0', 'win32', 'x64').sha256, sum);
});

test('an available update carries its installer', async () => {
  const r = await checkForUpdates('0.1.0', reply(200, { tag_name: 'v0.2.0', assets: [asset('FistBump-0.2.0-Windows-x64.exe')] }), { platform: 'win32', arch: 'x64' });
  assert.strictEqual(r.asset.name, 'FistBump-0.2.0-Windows-x64.exe');
  assert.strictEqual((await checkForUpdates('0.1.0', reply(200, { tag_name: 'v0.2.0', assets: [] }), { platform: 'win32', arch: 'x64' })).asset, null);
});

const body = (text) => ({ ok: true, status: 200, headers: { get: () => null }, body: (async function* () { yield Buffer.from(text); })() });

test('downloads the installer, checks it, and leaves no half file', async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'fb-update-'));
  const a = { name: 'FistBump-0.2.0-Windows-x64.exe', url: BASE + 'FistBump-0.2.0-Windows-x64.exe', size: 5, sha256: crypto.createHash('sha256').update('hello').digest('hex') };
  const seen = [];
  const done = await downloadUpdate(a, dir, { fetchImpl: async () => body('hello'), onProgress: (got, total) => seen.push([got, total]) });
  assert.strictEqual(fs.readFileSync(done.path, 'utf8'), 'hello');
  assert.deepStrictEqual(seen.at(-1), [5, 5]);
  assert.deepStrictEqual(fs.readdirSync(dir), ['FistBump-0.2.0-Windows-x64.exe']);

  fs.rmSync(path.join(dir, a.name));
  await assert.rejects(downloadUpdate({ ...a, sha256: 'b'.repeat(64) }, dir, { fetchImpl: async () => body('hello') }), /checksum/);
  await assert.rejects(downloadUpdate({ ...a, sha256: '' }, dir, { fetchImpl: async () => body('hi') }), /cut short/);
  assert.deepStrictEqual(fs.readdirSync(dir), []); // neither failure left a file behind
  await assert.rejects(downloadUpdate({ ...a, url: 'https://evil.example/x.exe' }, dir, { fetchImpl: async () => body('hello') }), /cannot be downloaded/);
  await assert.rejects(downloadUpdate({ ...a, name: '../evil.exe' }, dir, { fetchImpl: async () => body('hello') }), /cannot be downloaded/);
});

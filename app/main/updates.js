// Checks GitHub Releases for a newer version. Nothing is downloaded or installed: the user is shown the
// release page. Builds are unsigned for now, so installing an update in place is left for later.
const RELEASES_API = 'https://api.github.com/repos/tlmcguire/fistbump/releases/latest';
const RELEASES_PAGE = 'https://github.com/tlmcguire/fistbump/releases';

// parseVersion reads "v1.2.3" or "1.2.3-beta.1" into numbers and a pre-release tag, or null.
function parseVersion(text) {
  const m = /^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$/.exec(String(text || '').trim());
  return m ? { parts: [Number(m[1]), Number(m[2]), Number(m[3])], pre: m[4] || '' } : null;
}

// compareVersions is negative when a is older than b. A pre-release is older than its release.
function compareVersions(a, b) {
  const x = parseVersion(a);
  const y = parseVersion(b);
  if (!x || !y) return 0;
  for (let i = 0; i < 3; i++) {
    if (x.parts[i] !== y.parts[i]) return x.parts[i] - y.parts[i];
  }
  if (x.pre === y.pre) return 0;
  if (!x.pre) return 1;
  if (!y.pre) return -1;
  return x.pre < y.pre ? -1 : 1;
}

const DOWNLOAD_PREFIX = 'https://github.com/tlmcguire/fistbump/releases/download/';
// The installers are named by electron-builder.yml: FistBump-<version>-<OS>-<arch>.<ext>.
const INSTALLER = /^FistBump-\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?-(?:Windows-x64\.exe|macOS-(?:arm64|x64)\.dmg|Linux-x86_64\.AppImage)$/;

// installerName is the file built for this computer, or '' when no installer is published for it.
function installerName(version, platform, arch) {
  if (platform === 'win32' && arch === 'x64') return `FistBump-${version}-Windows-x64.exe`;
  if (platform === 'darwin' && (arch === 'arm64' || arch === 'x64')) return `FistBump-${version}-macOS-${arch}.dmg`;
  if (platform === 'linux' && arch === 'x64') return `FistBump-${version}-Linux-x86_64.AppImage`;
  return '';
}

// pickAsset finds this computer's installer among a release's files. It returns only an installer on this
// project's GitHub downloads, so nothing else can be fetched, or null.
function pickAsset(assets, version, platform, arch) {
  const name = installerName(version, platform, arch);
  if (!name || !Array.isArray(assets)) return null;
  const a = assets.find((x) => x && x.name === name);
  if (!a || typeof a.browser_download_url !== 'string' || !a.browser_download_url.startsWith(DOWNLOAD_PREFIX)) return null;
  const size = Number.isInteger(a.size) && a.size > 0 ? a.size : 0;
  const m = /^sha256:([0-9a-f]{64})$/.exec(String(a.digest || ''));
  return { name, url: a.browser_download_url, size, sha256: m ? m[1] : '' };
}

// checkForUpdates returns { status, current, latest, url, publishedAt, message }. status is "available",
// "current", "none" (no release published yet) or "error". An available update also carries asset: this
// computer's installer, or null. fetchImpl and target are replaceable for tests.
async function checkForUpdates(current, fetchImpl = fetch, target = { platform: process.platform, arch: process.arch }) {
  const base = { current, latest: '', url: RELEASES_PAGE, publishedAt: '' };
  let res;
  try {
    res = await fetchImpl(RELEASES_API, {
      headers: { Accept: 'application/vnd.github+json', 'User-Agent': `fistbump/${current}` },
      signal: AbortSignal.timeout(10_000),
    });
  } catch {
    return { ...base, status: 'error', message: 'Could not reach GitHub. Check your connection and try again.' };
  }
  if (res.status === 404) return { ...base, status: 'none', message: 'No release has been published yet.' };
  if (!res.ok) return { ...base, status: 'error', message: `GitHub answered with an error (${res.status}). Try again later.` };

  let release;
  try {
    release = await res.json();
  } catch {
    return { ...base, status: 'error', message: 'GitHub sent an unreadable answer. Try again later.' };
  }
  const latest = String(release.tag_name || '').replace(/^v/, '');
  const url = typeof release.html_url === 'string' && release.html_url.startsWith('https://github.com/') ? release.html_url : RELEASES_PAGE;
  const found = { ...base, latest, url, publishedAt: release.published_at || '' };
  if (!parseVersion(latest)) return { ...found, status: 'error', message: 'The latest release has a version this app cannot read.' };
  return compareVersions(current, latest) < 0
    ? { ...found, status: 'available', message: `Fistbump ${latest} is available.`, asset: pickAsset(release.assets, latest, target.platform, target.arch) }
    : { ...found, status: 'current', message: `You have the latest version (${current}).` };
}

// downloadUpdate saves the installer into dir and returns { path }. It writes name.part first and renames
// it once the whole file is there and, when GitHub gave one, its SHA-256 matches, so a half file never
// looks like an installer. onProgress(received, total) is called as bytes arrive; signal cancels.
async function downloadUpdate(asset, dir, { fetchImpl = fetch, onProgress, signal } = {}) {
  const fs = require('node:fs');
  const path = require('node:path');
  const crypto = require('node:crypto');
  if (!asset || !INSTALLER.test(asset.name) || typeof asset.url !== 'string' || !asset.url.startsWith(DOWNLOAD_PREFIX)) {
    throw new Error('That update cannot be downloaded.');
  }
  const final = path.join(dir, asset.name);
  const part = `${final}.part`;
  const res = await fetchImpl(asset.url, { headers: { 'User-Agent': 'fistbump' }, signal });
  if (!res.ok || !res.body) throw new Error(`GitHub answered with an error (${res.status}).`);
  const total = asset.size || Number(res.headers.get('content-length')) || 0;
  fs.mkdirSync(dir, { recursive: true });
  const out = fs.createWriteStream(part);
  const hash = crypto.createHash('sha256');
  let received = 0;
  try {
    for await (const chunk of res.body) {
      hash.update(chunk);
      received += chunk.length;
      if (!out.write(chunk)) await new Promise((resolve) => out.once('drain', resolve));
      if (onProgress) onProgress(received, total);
    }
    await new Promise((resolve, reject) => out.end((err) => (err ? reject(err) : resolve())));
    if (asset.size && received !== asset.size) throw new Error('The download was cut short. Try again.');
    if (asset.sha256 && hash.digest('hex') !== asset.sha256) throw new Error('The download did not match the published checksum, so it was removed.');
    fs.renameSync(part, final);
  } catch (error) {
    out.destroy();
    fs.rmSync(part, { force: true });
    throw error;
  }
  if (process.platform === 'linux') fs.chmodSync(final, 0o755); // an AppImage runs once it is executable
  return { path: final };
}

module.exports = { checkForUpdates, compareVersions, parseVersion, pickAsset, installerName, downloadUpdate, RELEASES_PAGE };

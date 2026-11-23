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

// appBundle is the FistBump.app the running app lives in, or '' when it cannot be replaced in place: not
// inside a .app, run from the disk image (/Volumes) or from the copy macOS makes of an app it has not
// moved yet (App Translocation).
function appBundle(execPath) {
  const path = require('node:path');
  const bundle = path.resolve(execPath, '..', '..', '..');
  if (!bundle.endsWith('.app') || bundle.startsWith('/Volumes/') || bundle.includes('/AppTranslocation/')) return '';
  return bundle;
}

// MAC_INSTALL is the script that replaces the app on macOS once it has quit. It is run as
// bash MAC_INSTALL <pid> <dmg> <app>, so no path is pasted into the script. It waits for the app to exit,
// copies the new app out of the disk image next to the old one, swaps the two, and opens the new one.
// If anything fails before the swap, the old app is left where it was and opened again.
const MAC_INSTALL = `#!/bin/bash
pid="$1"; dmg="$2"; app="$3"
new="$app.update"; old="$app.previous"
mnt="$(mktemp -d)"
cleanup() { hdiutil detach -quiet "$mnt" >/dev/null 2>&1; rmdir "$mnt" 2>/dev/null; }
fail() { cleanup; rm -rf "$new"; open "$app"; exit 1; }
for _ in $(seq 1 300); do kill -0 "$pid" 2>/dev/null || break; sleep 0.2; done
hdiutil attach -nobrowse -noautoopen -quiet -mountpoint "$mnt" "$dmg" || fail
src="$(find "$mnt" -maxdepth 1 -name '*.app' -print -quit)"
[ -n "$src" ] || fail
rm -rf "$new"
ditto "$src" "$new" || fail
cleanup
rm -rf "$old"
mv "$app" "$old" || { rm -rf "$new"; open "$app"; exit 1; }
if ! mv "$new" "$app"; then mv "$old" "$app"; open "$app"; exit 1; fi
rm -rf "$old"
xattr -dr com.apple.quarantine "$app" 2>/dev/null
rm -f "$dmg"
open "$app"
`;

// installPlan says how to put the downloaded installer in place of the running app, then start it again:
//   { kind: 'run', command, args }: start this and quit; it installs and starts the new version.
//   { kind: 'appimage', target }:   copy the file over target, then relaunch.
//   { kind: 'manual', reason }:     it cannot be done here; the user opens the installer.
function installPlan({ platform, file, execPath, appImage, pid, scriptPath, canWrite }) {
  if (platform === 'win32') {
    // electron-builder's installer: /S installs without its pages, --force-run starts the app afterwards.
    return { kind: 'run', command: file, args: ['/S', '--force-run'] };
  }
  if (platform === 'darwin') {
    const bundle = appBundle(execPath);
    if (!bundle) return { kind: 'manual', reason: 'Fistbump is running from the disk image or a temporary copy, so it cannot replace itself. Move it to Applications first.' };
    if (!canWrite(require('node:path').dirname(bundle)) || !canWrite(bundle)) return { kind: 'manual', reason: `Fistbump cannot write to ${bundle}.` };
    return { kind: 'run', command: '/bin/bash', args: [scriptPath, String(pid), file, bundle] };
  }
  if (platform === 'linux') {
    if (!appImage) return { kind: 'manual', reason: 'Fistbump was not started from an AppImage, so it cannot replace itself.' };
    if (!canWrite(require('node:path').dirname(appImage))) return { kind: 'manual', reason: `Fistbump cannot write to the folder ${require('node:path').dirname(appImage)}.` };
    return { kind: 'appimage', target: appImage };
  }
  return { kind: 'manual', reason: 'Updates cannot be installed on this system.' };
}

module.exports = { checkForUpdates, compareVersions, parseVersion, pickAsset, installerName, downloadUpdate, appBundle, installPlan, MAC_INSTALL, RELEASES_PAGE };

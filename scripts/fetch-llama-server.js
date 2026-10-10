#!/usr/bin/env node
// Downloads the llama.cpp release build that contains llama-server for this platform into bin/<os>-<arch>/.
// Optional: without it the app still works with the remote and rules engines, or with llama-server on PATH.
//   node scripts/fetch-llama-server.js
const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');
const { spawnSync } = require('child_process');
const hostArch = require('./host-arch');

const root = path.join(__dirname, '..');
const PATTERN = {
  'darwin-arm64': /bin-macos-arm64\.(zip|tar\.gz)$/,
  'darwin-x64': /bin-macos-x64\.(zip|tar\.gz)$/,
  'linux-x64': /bin-ubuntu-x64\.(zip|tar\.gz)$/,
  'win32-x64': /bin-win-cpu-x64\.zip$/,
  'win32-arm64': /bin-win-cpu-arm64\.zip$/,
};

async function main() {
  const key = `${process.platform}-${hostArch()}`;
  const pattern = PATTERN[key];
  if (!pattern) throw new Error(`no llama.cpp release build mapped for ${key}`);
  // Binary builds are published as "bNNNN" prereleases, so releases/latest does not point at one.
  const releases = await (await fetch('https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=30', {
    headers: { 'User-Agent': 'fistbump' },
  })).json();
  if (!Array.isArray(releases)) throw new Error(`GitHub API error: ${releases.message || 'unexpected response'}`);
  let rel = null;
  let asset = null;
  for (const r of releases) {
    if (!/^b\d+$/.test(r.tag_name)) continue;
    asset = (r.assets || []).find((a) => pattern.test(a.name));
    if (asset) { rel = r; break; }
  }
  if (!asset) throw new Error(`no recent llama.cpp build has an asset matching ${pattern}`);
  console.log(`downloading ${asset.name} (${(asset.size / 1e6).toFixed(0)} MB) from ${rel.tag_name}`);
  const buf = Buffer.from(await (await fetch(asset.browser_download_url)).arrayBuffer());
  if (asset.digest && asset.digest.startsWith('sha256:')) {
    const sum = crypto.createHash('sha256').update(buf).digest('hex');
    if (sum !== asset.digest.slice(7)) throw new Error('checksum mismatch, refusing to install');
    console.log('sha256 verified');
  } else {
    console.warn('release lists no digest; checksum not verified');
  }
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'llama-'));
  const archive = path.join(tmp, asset.name);
  fs.writeFileSync(archive, buf);
  const dest = path.join(root, 'bin', key);
  fs.mkdirSync(dest, { recursive: true });
  const extract = asset.name.endsWith('.zip')
    ? spawnSync(process.platform === 'win32' ? 'tar' : 'unzip', process.platform === 'win32' ? ['-xf', archive, '-C', tmp] : ['-q', archive, '-d', tmp])
    : spawnSync('tar', ['-xzf', archive, '-C', tmp]);
  if (extract.status !== 0) throw new Error('could not extract the archive (need unzip or tar)');
  // The archive nests files under a build directory; copy the server and its shared libraries.
  const exe = process.platform === 'win32' ? 'llama-server.exe' : 'llama-server';
  const found = [];
  (function walk(d) {
    for (const e of fs.readdirSync(d, { withFileTypes: true })) {
      const p = path.join(d, e.name);
      if (e.isDirectory()) walk(p);
      else if (e.name === exe) found.push(path.dirname(p));
    }
  })(tmp);
  if (!found.length) throw new Error(`${exe} not found in the archive`);
  for (const f of fs.readdirSync(found[0])) {
    if (/\.(dylib|so|dll|metal)$|\.so\.|^llama-server/.test(f)) fs.copyFileSync(path.join(found[0], f), path.join(dest, f));
  }
  if (process.platform !== 'win32') fs.chmodSync(path.join(dest, exe), 0o755);
  fs.rmSync(tmp, { recursive: true, force: true });
  console.log(`installed ${path.relative(root, path.join(dest, exe))}`);
}

main().catch((e) => {
  console.error(e.message);
  process.exit(1);
});

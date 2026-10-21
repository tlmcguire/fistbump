#!/usr/bin/env node
// Builds the Go backend into bin/<os>-<arch>/. No cgo is needed, so any host can build any target.
//   node scripts/build-go.js          current platform
//   node scripts/build-go.js --all    darwin, linux and windows on amd64 and arm64
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const hostArch = require('./host-arch');

const root = path.join(__dirname, '..');
const GOOS = { darwin: 'darwin', linux: 'linux', win32: 'windows' };
const GOARCH = { x64: 'amd64', arm64: 'arm64' };

function build(os, arch) {
  const dir = path.join(root, 'bin', `${os}-${arch}`);
  fs.mkdirSync(dir, { recursive: true });
  const out = path.join(dir, os === 'win32' ? 'fistbump-core.exe' : 'fistbump-core');
  const r = spawnSync('go', ['build', '-buildvcs=false', '-trimpath', '-ldflags', '-s -w', '-o', out, './cmd/fistbump-core'], {
    cwd: path.join(root, 'backend'),
    stdio: 'inherit',
    env: { ...process.env, CGO_ENABLED: '0', GOOS: GOOS[os], GOARCH: GOARCH[arch] },
  });
  if (r.error || r.status !== 0) {
    console.error(`build failed for ${os}-${arch}${r.error ? `: ${r.error.message}` : ''}`);
    process.exit(r.status || 1);
  }
  console.log(`built ${path.relative(root, out)}`);
}

if (process.argv.includes('--all')) {
  for (const os of Object.keys(GOOS)) for (const arch of Object.keys(GOARCH)) build(os, arch);
} else {
  const arch = hostArch();
  if (!GOOS[process.platform] || !GOARCH[arch]) {
    console.error(`unsupported platform ${process.platform}-${arch}`);
    process.exit(1);
  }
  build(process.platform, arch);
}

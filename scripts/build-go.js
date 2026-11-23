#!/usr/bin/env node
// Builds the Go backend into bin/<os>-<arch>/. No cgo is needed, so any host can build any target.
// The folders use Node's names, so electron-builder's ${arch} macro finds them.
//   node scripts/build-go.js                                 the platform this computer runs
//   node scripts/build-go.js --platform darwin --arch x64    one platform and architecture
//   node scripts/build-go.js --platform darwin               a platform, with the architectures it ships
//   node scripts/build-go.js --all                           every platform the app ships for
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const { parseArgs } = require('util');
const hostArch = require('./host-arch');

const root = path.join(__dirname, '..');
const GOOS = { darwin: 'darwin', linux: 'linux', win32: 'windows' };
const GOARCH = { x64: 'amd64', arm64: 'arm64' };
//the architectures each platform ships, in the order the installers are built
const SHIPPED = { darwin: ['arm64', 'x64'], win32: ['x64'], linux: ['x64'] };
//every target the app ships for; the arm64 variants of Linux and Windows are named with --arch
const ALL = ['darwin-arm64', 'darwin-x64', 'win32-x64', 'linux-x64'];

const { values } = parseArgs({
  options: {
    platform: { type: 'string' },
    arch: { type: 'string' },
    all: { type: 'boolean' },
  },
  allowPositionals: true, //extra arguments are ignored, as they always were
  strict: false, //an unknown flag is ignored, as it always was
});

//the version the backend reports at GET /v1/health, so a release build is not "dev"
const pkg = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'));
const LDFLAGS = `-s -w -X main.version=${pkg.version}`;

function build(os, arch) {
  const dir = path.join(root, 'bin', `${os}-${arch}`);
  fs.mkdirSync(dir, { recursive: true });
  const out = path.join(dir, os === 'win32' ? 'fistbump-core.exe' : 'fistbump-core');
  const r = spawnSync('go', ['build', '-buildvcs=false', '-trimpath', '-ldflags', LDFLAGS, '-o', out, './cmd/fistbump-core'], {
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

//the targets one run builds
const targets = () => {
  if (values.all) return ALL.map(target => target.split('-'));
  const os = values.platform || process.platform;
  if (!GOOS[os]) {
    console.error(`unsupported platform ${os}`);
    process.exit(1);
  }
  if (values.arch) {
    if (!GOARCH[values.arch]) {
      console.error(`unsupported architecture ${values.arch}`);
      process.exit(1);
    }
    return [[os, values.arch]];
  }
  //no architecture was named: a named platform builds the architectures it ships,
  //and this computer's platform also builds the architecture it runs on
  const arches = values.platform ? SHIPPED[os] : [...new Set([hostArch(), ...SHIPPED[os]])];
  for (const arch of arches) {
    if (!GOARCH[arch]) {
      console.error(`unsupported architecture ${arch}`);
      process.exit(1);
    }
  }
  return arches.map(arch => [os, arch]);
};

for (const [os, arch] of targets()) build(os, arch);

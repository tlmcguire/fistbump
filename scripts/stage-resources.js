#!/usr/bin/env node
// Stages bin/<platform>-<arch>/ into src-tauri/resources/bin/ so Tauri's bundle.resources
// packages fistbump-core, llama-server, and all shared libraries for the active build target.
const fs = require('node:fs');
const path = require('node:path');
const { parseArgs } = require('util');
const hostArch = require('./host-arch');

const root = path.join(__dirname, '..');
const { values } = parseArgs({
  options: {
    platform: { type: 'string' },
    arch: { type: 'string' },
  },
  allowPositionals: true,
  strict: false,
});

const platform = values.platform || process.platform;
const arch = values.arch || hostArch();
const src = path.join(root, 'bin', `${platform}-${arch}`);
const dest = path.join(root, 'src-tauri', 'resources', 'bin');

if (!fs.existsSync(src)) {
  console.error(`Missing backend binaries at ${path.relative(root, src)}. Run "npm run build:go" first.`);
  process.exit(1);
}

fs.rmSync(dest, { recursive: true, force: true });
fs.mkdirSync(dest, { recursive: true });
fs.writeFileSync(path.join(dest, '.gitkeep'), '');
fs.cpSync(src, dest, { recursive: true, verbatimSymlinks: true });
console.log(`staged ${path.relative(root, src)} -> ${path.relative(root, dest)}`);

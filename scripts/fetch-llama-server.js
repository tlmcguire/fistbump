#!/usr/bin/env node
// Puts a llama-server next to fistbump-core, so the local engine can run on this computer.
// The llama.cpp release is pinned: a new version is a decision, never an automatic update.
// The folders use Node's names, so they are the folders the backend is built into.
//   node scripts/fetch-llama-server.js                                  the platform this computer runs
//   node scripts/fetch-llama-server.js --platform darwin --arch x64     one platform and architecture
//   node scripts/fetch-llama-server.js --platform darwin                a platform, with the architectures it ships
//   node scripts/fetch-llama-server.js --all                            every platform the app ships for
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const zlib = require('node:zlib');
const https = require('node:https');
const { execFileSync } = require('node:child_process');
const { parseArgs } = require('node:util');
const hostArch = require('./host-arch');

// The pinned llama.cpp release, with the asset and its SHA-256 per platform.
const TAG = 'b12633';
const ASSETS = {
  'darwin-arm64': {file: 'llama-' + TAG + '-bin-macos-arm64.tar.gz', sha256: '99657876053df9004753d92075813bbb3b484e79f8a402809b51d00c8c540062'},
  'darwin-amd64': {file: 'llama-' + TAG + '-bin-macos-x64.tar.gz', sha256: '42e69b4d89531197ec0551f288347a71856c4613e52030196486b888c82b8892'},
  'linux-arm64': {file: 'llama-' + TAG + '-bin-ubuntu-arm64.tar.gz', sha256: 'b54c26bb79eac84283c8b6f45722603a080bf1bab9ce120c932ce3945063e0a4'},
  'linux-amd64': {file: 'llama-' + TAG + '-bin-ubuntu-x64.tar.gz', sha256: '9c4a2c1bd1437f22831e0b56cc75329b321bac87225ffb5c83cd8d13b1f44fa2'},
  'windows-arm64': {file: 'llama-' + TAG + '-bin-win-cpu-arm64.zip', sha256: 'c37f444b153e43a9b27c5d00b78c4c300bdb2878de76ab49112f4558a245e784'},
  'windows-amd64': {file: 'llama-' + TAG + '-bin-win-cpu-x64.zip', sha256: 'bad9fe0c1b554bcb1ce5630d66ad34a2285a8b390400513cc02f56feafd62c8e'},
};

// Each asset lands in the folder the backend for that platform is built into: Node's names, so
// electron-builder's ${arch} macro finds them.
const TARGETS = {
  'darwin-arm64': {asset: 'darwin-arm64', folder: 'darwin-arm64'},
  'darwin-x64': {asset: 'darwin-amd64', folder: 'darwin-x64'},
  'linux-arm64': {asset: 'linux-arm64', folder: 'linux-arm64'},
  'linux-x64': {asset: 'linux-amd64', folder: 'linux-x64'},
  'win32-arm64': {asset: 'windows-arm64', folder: 'win32-arm64'},
  'win32-x64': {asset: 'windows-amd64', folder: 'win32-x64'},
};

// The architectures each platform ships, and the targets the app ships for.
const SHIPPED_ARCHES = {darwin: ['arm64', 'x64'], win32: ['x64'], linux: ['x64']};
//the targets the app ships for; the arm64 variants of Linux and Windows are named with --arch
const ALL_TARGETS = ['darwin-arm64', 'darwin-x64', 'win32-x64', 'linux-x64'];

const root = path.join(__dirname, '..');

const { values } = parseArgs({
  options: {
    platform: {type: 'string'},
    arch: {type: 'string'},
    all: {type: 'boolean'},
  },
  allowPositionals: true, //extra arguments are ignored, as they always were
  strict: false, //an unknown flag is ignored, as it always was
});

//follows the redirect GitHub sends to the file itself
const download = (url, redirects = 0) => new Promise((resolve, reject) => {
  https.get(url, {headers: {'user-agent': 'fistbump-build'}}, res => {
    if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
      res.resume();
      if (redirects >= 5) return reject(new Error('too many redirects'));
      return resolve(download(new URL(res.headers.location, url).toString(), redirects + 1));
    }
    if (res.statusCode !== 200) {
      res.resume();
      return reject(new Error(url + ' answered ' + res.statusCode));
    }
    const chunks = [];
    res.on('data', chunk => chunks.push(chunk));
    res.on('end', () => resolve(Buffer.concat(chunks)));
  }).on('error', reject);
});

const sha256 = buffer => crypto.createHash('sha256').update(buffer).digest('hex');

//untar reads a tar archive into a map of name to content. Only the file entries are kept.
//A symbolic link is kept as a copy of the file it points at: llama-server finds its libraries by
//their version names (libllama.1.dylib), and those names are links to the real files in the archive.
const untar = buffer => {
  const files = new Map();
  const links = []; //{name, target}, resolved once the whole archive is read
  let offset = 0;
  let longName = null;
  while (offset + 512 <= buffer.length) {
    const header = buffer.subarray(offset, offset + 512);
    if (header.every(byte => byte === 0)) break; //the end of the archive
    const name = header.subarray(0, 100).toString('utf8').replace(/\0.*$/, '');
    const size = parseInt(header.subarray(124, 136).toString('ascii').replace(/[\0 ]/g, ''), 8) || 0;
    const type = String.fromCharCode(header[156]);
    const linkName = header.subarray(157, 257).toString('utf8').replace(/\0.*$/, '');
    offset += 512;
    const data = buffer.subarray(offset, offset + size);
    offset += Math.ceil(size / 512) * 512;
    if (type === 'L') { //a GNU long name; the entry after it holds the file
      longName = data.toString('utf8').replace(/\0.*$/, '');
      continue;
    }
    if (type === '0' || type === '\0' || type === '') {
      files.set(longName || name, data);
      longName = null;
    } else if (type === '2') { //a symbolic link, relative to the folder the link is in
      const entry = longName || name;
      longName = null;
      links.push({name: entry, target: path.posix.join(path.posix.dirname(entry), linkName)});
    }
  }
  //a link can point at another link, so the links are resolved until no more can be
  let pending = links;
  while (pending.length) {
    const next = pending.filter(({name, target}) => {
      if (!files.has(target)) return true;
      files.set(name, files.get(target));
      return false;
    });
    if (next.length === pending.length) break; //the rest point at nothing in the archive
    pending = next;
  }
  return files;
};

//unzip reads a zip archive into a map of name to content. Stored and deflated entries are both read.
const unzip = buffer => {
  const files = new Map();
  let end = -1;
  for (let i = buffer.length - 22; i >= 0 && i >= buffer.length - 22 - 65536; i--) {
    if (buffer.readUInt32LE(i) === 0x06054b50) { //the end of central directory
      end = i;
      break;
    }
  }
  if (end < 0) throw new Error('not a zip file');
  const count = buffer.readUInt16LE(end + 10);
  let offset = buffer.readUInt32LE(end + 16);
  for (let i = 0; i < count; i++) {
    if (buffer.readUInt32LE(offset) !== 0x02014b50) throw new Error('bad zip directory');
    const method = buffer.readUInt16LE(offset + 10);
    const compressedSize = buffer.readUInt32LE(offset + 20);
    const nameLength = buffer.readUInt16LE(offset + 28);
    const extraLength = buffer.readUInt16LE(offset + 30);
    const commentLength = buffer.readUInt16LE(offset + 32);
    const localOffset = buffer.readUInt32LE(offset + 42);
    const name = buffer.subarray(offset + 46, offset + 46 + nameLength).toString('utf8');
    const localName = buffer.readUInt16LE(localOffset + 26);
    const localExtra = buffer.readUInt16LE(localOffset + 28);
    const start = localOffset + 30 + localName + localExtra;
    const raw = buffer.subarray(start, start + compressedSize);
    files.set(name, method === 0 ? raw : zlib.inflateRawSync(raw));
    offset += 46 + nameLength + extraLength + commentLength;
  }
  return files;
};

//the shared libraries llama-server loads at runtime, next to the binary it was built with
const LIBRARY = /\.(so(\.[0-9]+)*|dylib|dll)$/i;
const SERVER = /(^|\/)llama-server(\.exe)?$/;

//picks the server and the libraries beside it out of the archive
const pickServerFiles = files => {
  const server = [...files.keys()].find(name => SERVER.test(name));
  if (!server) throw new Error('the archive has no llama-server');
  const folder = server.slice(0, server.lastIndexOf('/') + 1);
  const picked = new Map();
  for (const [name, data] of files) {
    if (!name.startsWith(folder)) continue;
    if (name === server || LIBRARY.test(name)) picked.set(name.slice(folder.length), data);
  }
  return picked;
};

//writes the server and its libraries into the folder the backend for that platform lives in
const install = (target, files) => {
  const dir = path.join(root, 'bin', target);
  fs.mkdirSync(dir, {recursive: true});
  const names = [];
  for (const [name, data] of files) {
    const out = path.join(dir, name);
    fs.writeFileSync(out, data);
    fs.chmodSync(out, 0o755);
    names.push(name);
  }
  return {dir, names};
};

//the libraries a macOS binary asks for at load time, as otool -L lists them, that are not in the folder
const missingLibraries = (otoolOutput, names) => {
  const missing = [];
  for (const line of otoolOutput.split('\n')) {
    const match = line.match(/^\t@rpath\/(\S+) /);
    if (match && !names.has(match[1])) missing.push(match[1]);
  }
  return missing;
};

//checks that the installed server can load. A release that drops a library fails here, at build
//time, rather than when a user asks the AI for the first time.
const verifyInstall = (target, dir, names) => {
  const present = new Set(names);
  if (process.platform === 'darwin') {
    for (const name of names) {
      if (!LIBRARY.test(name) && name !== 'llama-server') continue;
      const out = execFileSync('otool', ['-L', path.join(dir, name)], {encoding: 'utf8'});
      const missing = missingLibraries(out, present);
      if (missing.length) {
        throw new Error(name + ' needs ' + missing.join(', ') + ', which the archive does not include');
      }
    }
  }
  //when the target is this computer, the server is run, so a load failure shows here
  if (target === process.platform + '-' + hostArch()) {
    const server = path.join(dir, 'llama-server' + (process.platform === 'win32' ? '.exe' : ''));
    execFileSync(server, ['--version'], {stdio: 'ignore'});
  }
};

const fetchFor = async target => {
  const mapping = TARGETS[target];
  if (!mapping) throw new Error('no target for ' + target);
  const asset = ASSETS[mapping.asset];
  if (!asset) throw new Error('no asset for ' + target);
  const url = 'https://github.com/ggml-org/llama.cpp/releases/download/' + TAG + '/' + asset.file;
  process.stdout.write('fetching ' + asset.file + '... ');
  const buffer = await download(url);
  const hash = sha256(buffer);
  if (hash !== asset.sha256) {
    throw new Error('the hash is ' + hash + ', but ' + asset.sha256 + ' was expected');
  }
  const files = asset.file.endsWith('.zip') ? unzip(buffer) : untar(zlib.gunzipSync(buffer));
  const done = install(mapping.folder, pickServerFiles(files));
  verifyInstall(target, done.dir, done.names);
  console.log('ok, ' + done.names.length + ' files in ' + path.relative(root, done.dir));
};

//the targets one run installs
const targets = () => {
  if (values.all) return ALL_TARGETS;
  const platform = values.platform || process.platform;
  if (!SHIPPED_ARCHES[platform]) {
    throw new Error('unsupported platform ' + platform);
  }
  //without an architecture: a named platform fetches the architectures it ships,
  //and this computer's platform also fetches the architecture it runs on
  const arches = values.arch
    ? [values.arch]
    : values.platform
      ? SHIPPED_ARCHES[platform]
      : [...new Set([hostArch(), ...SHIPPED_ARCHES[platform]])];
  const list = arches.map(arch => platform + '-' + arch);
  for (const target of list) {
    if (!TARGETS[target]) throw new Error('no target for ' + target);
  }
  return list;
};

const run = async () => {
  for (const target of targets()) await fetchFor(target);
};

module.exports = {TAG, ASSETS, TARGETS, untar, unzip, pickServerFiles, missingLibraries, download, sha256};

if (require.main === module) {
  run().catch(error => {
    console.error('fetch-llama-server:', error.message);
    process.exit(1);
  });
}

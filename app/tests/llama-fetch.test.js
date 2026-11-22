// The fetch script reads the llama.cpp archives without any dependency, so the readers are
// checked here against archives built in memory.
const test = require("node:test");
const assert = require("node:assert");
const zlib = require("node:zlib");

const {untar, unzip, pickServerFiles, TAG, ASSETS} = require("../../scripts/fetch-llama-server.js");

//one file entry in a tar archive
const tarEntry = (name, content) => {
  const header = Buffer.alloc(512);
  header.write(name, 0, 100, "utf8");
  header.write("0000644", 100, 8, "ascii");
  header.write("0000000", 108, 8, "ascii");
  header.write("0000000", 116, 8, "ascii");
  header.write(content.length.toString(8).padStart(11, "0") + " ", 124, 12, "ascii");
  header.write("00000000000 ", 136, 12, "ascii");
  header.write("        ", 148, 8, "ascii");
  header.write("0", 156, 1, "ascii");
  header.write("ustar\0" + "00", 257, 8, "ascii");
  let sum = 0;
  for (const byte of header) sum += byte;
  header.write(sum.toString(8).padStart(6, "0") + "\0 ", 148, 8, "ascii");
  const data = Buffer.alloc(Math.ceil(content.length / 512) * 512);
  content.copy(data);
  return Buffer.concat([header, data]);
};

//a zip archive: a local header, a central directory and an end record
const zipArchive = entries => {
  const locals = [];
  const central = [];
  let offset = 0;
  for (const {name, content, deflate} of entries) {
    const nameBuf = Buffer.from(name, "utf8");
    const data = deflate ? zlib.deflateRawSync(content) : content;
    const method = deflate ? 8 : 0;
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(0, 6);
    local.writeUInt16LE(method, 8);
    local.writeUInt32LE(0, 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(content.length, 22);
    local.writeUInt16LE(nameBuf.length, 26);
    local.writeUInt16LE(0, 28);
    locals.push(Buffer.concat([local, nameBuf, data]));

    const dir = Buffer.alloc(46);
    dir.writeUInt32LE(0x02014b50, 0);
    dir.writeUInt16LE(20, 4);
    dir.writeUInt16LE(20, 6);
    dir.writeUInt16LE(0, 8);
    dir.writeUInt16LE(method, 10);
    dir.writeUInt32LE(0, 16);
    dir.writeUInt32LE(data.length, 20);
    dir.writeUInt32LE(content.length, 24);
    dir.writeUInt16LE(nameBuf.length, 28);
    dir.writeUInt16LE(0, 30);
    dir.writeUInt16LE(0, 32);
    dir.writeUInt16LE(0, 34);
    dir.writeUInt16LE(0, 36);
    dir.writeUInt32LE(0, 38);
    dir.writeUInt32LE(offset, 42);
    central.push(Buffer.concat([dir, nameBuf]));
    offset += local.length + nameBuf.length + data.length;
  }
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  const centralSize = central.reduce((sum, part) => sum + part.length, 0);
  end.writeUInt32LE(centralSize, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, ...central, end]);
};

test("the pinned release has an asset and a hash for every platform", () => {
  assert.match(TAG, /^b\d+$/);
  for (const target of Object.keys(ASSETS)) {
    const asset = ASSETS[target];
    assert.ok(asset.file.includes(TAG), target + " is not from the pinned release");
    assert.match(asset.sha256, /^[0-9a-f]{64}$/, target + " has no hash");
  }
});

test("a tar archive is read and the server with its libraries is picked", () => {
  const folder = "llama-" + TAG + "-bin-macos-arm64/build/bin/";
  const tar = Buffer.concat([
    tarEntry(folder + "llama-server", Buffer.from("server")),
    tarEntry(folder + "libllama.dylib", Buffer.from("library")),
    tarEntry(folder + "libggml.dylib", Buffer.from("library")),
    tarEntry(folder + "README.md", Buffer.from("text")),
    Buffer.alloc(1024),
  ]);
  const files = untar(tar);
  assert.equal(files.size, 4);
  assert.equal(files.get(folder + "llama-server").toString(), "server");

  const picked = pickServerFiles(files);
  assert.deepEqual([...picked.keys()].sort(), ["libggml.dylib", "libllama.dylib", "llama-server"]);
});

test("a zip archive is read, stored and deflated alike", () => {
  const folder = "llama-" + TAG + "-bin-win-cpu-x64/";
  const zip = zipArchive([
    {name: folder + "llama-server.exe", content: Buffer.from("server"), deflate: false},
    {name: folder + "ggml.dll", content: Buffer.from("library"), deflate: true},
    {name: folder + "notes.txt", content: Buffer.from("text"), deflate: true},
  ]);
  const files = unzip(zip);
  assert.equal(files.size, 3);
  assert.equal(files.get(folder + "llama-server.exe").toString(), "server");
  assert.equal(files.get(folder + "ggml.dll").toString(), "library");

  const picked = pickServerFiles(files);
  assert.deepEqual([...picked.keys()].sort(), ["ggml.dll", "llama-server.exe"]);
});

test("an archive without a server is refused", () => {
  assert.throws(() => pickServerFiles(new Map([["build/bin/readme.md", Buffer.from("x")]])), /no llama-server/);
});

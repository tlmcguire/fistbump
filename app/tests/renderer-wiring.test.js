// The renderer is plain scripts with no build step, so a typo in an id or an API name is only found
// when someone clicks. These tests check that what the scripts ask for exists.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');

const renderer = path.join(__dirname, '..', 'renderer');
const read = file => fs.readFileSync(path.join(renderer, file), 'utf8');
const scripts = (function list(dir) {
  return fs.readdirSync(path.join(renderer, dir), {withFileTypes: true}).flatMap(entry => {
    const rel = path.join(dir, entry.name);
    if (entry.isDirectory()) return list(rel);
    return entry.name.endsWith('.js') ? [rel] : [];
  });
})('.');

const matches = (text, re) => [...text.matchAll(re)].map(m => m[1]);

test('every element id a script looks up exists in the page or is created by a script', () => {
  const html = read('index.html');
  const known = new Set(matches(html, /\bid="([\w-]+)"/g));
  for (const file of scripts) {
    for (const id of matches(read(file), /\bid:\s*"([\w-]+)"/g)) known.add(id);
    for (const id of matches(read(file), /\.id\s*=\s*"([\w-]+)"/g)) known.add(id);
  }
  const missing = [];
  for (const file of scripts) {
    for (const id of matches(read(file), /(?:\$|getElementById)\(\s*"([\w-]+)"\s*\)/g)) {
      if (!known.has(id)) missing.push(`${file}: #${id}`);
    }
  }
  assert.deepEqual([...new Set(missing)], [], 'ids that nothing defines');
});

test('every FistBumpAPI group and function the views call is defined and reaches a Tauri bridge method', () => {
  const api = read('js/api.js');
  const bridge = read('js/tauri-bridge.js');

  // groups wrapped in api.js, with the bridge functions they call
  const wrapped = {};
  for (const m of api.matchAll(/FistBumpAPI\.(\w+)\s*=\s*wrap\(\s*"(\w+)"\s*,\s*\[([^\]]*)\]/g)) {
    wrapped[m[1]] = {group: m[2], names: matches(m[3], /"(\w+)"/g)};
  }
  const problems = [];
  for (const [name, {group, names}] of Object.entries(wrapped)) {
    const block = bridge.match(new RegExp(`\\n    ${group}: \\{([\\s\\S]*?)\\n    \\}`));
    if (!block) { problems.push(`tauri-bridge has no group ${group}`); continue; }
    for (const fn of names) {
      if (!new RegExp(`\\b${fn}:`).test(block[1])) problems.push(`tauri-bridge.${group}.${fn} (wrapped as FistBumpAPI.${name}.${fn})`);
    }
  }

  // what the views call
  const defined = new Set([...Object.keys(wrapped), ...matches(api, /FistBumpAPI\.(\w+)\s*=/g), ...matches(api, /^ {4}(?:async\s+)?(\w+)\s*\(/gm)]);
  for (const file of scripts) {
    const text = read(file);
    for (const m of text.matchAll(/FistBumpAPI\.(\w+)(?:\.(\w+))?/g)) {
      const [, group, fn] = m;
      if (!defined.has(group)) problems.push(`${file}: FistBumpAPI.${group} is not defined`);
      else if (fn && wrapped[group] && !wrapped[group].names.includes(fn)) problems.push(`${file}: FistBumpAPI.${group}.${fn} is not wrapped`);
    }
  }
  assert.deepEqual([...new Set(problems)], []);
});

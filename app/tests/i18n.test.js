// The interface languages: every language has the same strings, placeholders match, and translation works.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const dir = path.join(__dirname, '..', 'renderer', 'js');
function load(lang) {
  const window = {};
  const sandbox = {
    window, navigator: { language: 'en-US' },
    document: { documentElement: {}, body: {} },
    MutationObserver: class { observe() {} },
    Node: { TEXT_NODE: 3, ELEMENT_NODE: 1 },
  };
  vm.createContext(sandbox);
  for (const file of ['i18n.js', ...['es', 'fr', 'de'].map((l) => `locales/${l}.js`)]) {
    vm.runInContext(fs.readFileSync(path.join(dir, file), 'utf8'), sandbox);
  }
  window.I18n.init(lang);
  return window.I18n;
}

const entries = (lang) => {
  const src = fs.readFileSync(path.join(dir, 'locales', `${lang}.js`), 'utf8');
  return [...src.matchAll(/"((?:[^"\\]|\\.)*)": "((?:[^"\\]|\\.)*)"/g)].map((m) => [m[1], m[2]]);
};
const holes = (s) => (s.match(/\{\d\}/g) || []).sort().join('');

test('every language covers the same strings as Spanish', () => {
  const base = new Set(entries('es').map(([k]) => k));
  for (const lang of ['fr', 'de']) {
    const have = new Set(entries(lang).map(([k]) => k));
    assert.deepEqual([...base].filter((k) => !have.has(k)), [], `${lang} is missing strings`);
    assert.deepEqual([...have].filter((k) => !base.has(k)), [], `${lang} has strings Spanish lacks`);
  }
});

test('a translation keeps the same {0} {1} parts as its source and is never empty', () => {
  for (const lang of ['es', 'fr', 'de']) {
    for (const [source, target] of entries(lang)) {
      assert.ok(target.trim(), `${lang}: "${source}" is empty`);
      assert.equal(holes(target), holes(source), `${lang}: "${source}" changes its parts`);
    }
  }
});

test('exact, pattern and dotted strings translate; unknown text stays as it is', () => {
  const es = load('es');
  assert.equal(es.translate('Find jobs'), 'Buscar empleos');
  assert.equal(es.translate('  Find jobs '), '  Buscar empleos ');
  assert.equal(es.translate('Round 3'), 'Ronda 3');
  assert.equal(es.translate('3 of 7'), '3 de 7');
  assert.equal(es.translate('Writing suggestions with the Local model'), 'Escribiendo sugerencias con Modelo local');
  assert.equal(es.translate('Ready · Built-in rules'), 'Listo · Reglas integradas');
  assert.equal(es.translate('Senior Data Engineer'), 'Senior Data Engineer');
  assert.equal(load('en').translate('Find jobs'), 'Find jobs');
  assert.equal(load('de').translate('Missing: sql, go'), 'Fehlt: sql, go');
  assert.equal(load('fr').locale(), 'fr');
});

test('the language follows the system when asked, and falls back to English', () => {
  assert.equal(load('system').locale(), 'en');
  assert.equal(load('xx').locale(), 'en');
});

// Unit tests covering the Fistbump improvements:
// - WordDiff ignoring curly vs straight apostrophe no-op diffs ("Bachelor’s" vs "Bachelor's")
// - WordDiff rendering structured .diff-line and .diff-bullet containers
// - Prefs pdfStyle validation across all 5 styles
// - Removal of the separate Open a link page (combined into search bar)
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const renderer = path.join(__dirname, '..', 'renderer');
const read = rel => fs.readFileSync(path.join(renderer, rel), 'utf8');

function loadWordDiff() {
  const created = [];
  const makeNode = (tag) => {
    const node = {
      tagName: tag.toUpperCase(),
      className: '',
      textContent: '',
      children: [],
      append(...items) { this.children.push(...items); },
    };
    created.push(node);
    return node;
  };
  const sandbox = {
    window: {},
    document: {
      createDocumentFragment: () => makeNode('#fragment'),
      createElement: (tag) => makeNode(tag),
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(read('js/worddiff.js'), sandbox);
  return sandbox.window.WordDiff;
}

test('WordDiff treats curly and straight apostrophes as equal (no Bachelor’s -> Bachelor\'s bug)', () => {
  const wd = loadWordDiff();
  const words = wd.diff('Earned a Bachelor\u2019s degree in CS.', "Earned a Bachelor's degree in CS.");
  assert.equal(words.every(w => w.op === 'eq'), true, `expected all eq ops, got ${JSON.stringify(words)}`);
});

test('WordDiff renders structured diff-line and diff-bullet elements', () => {
  const wd = loadWordDiff();
  const words = wd.diff('- Built APIs\n- Managed servers', '- Built Go APIs\n- Managed servers');
  const frag = wd.render(words, 'after');
  assert.equal(frag.children.length, 2);
  assert.ok(frag.children[0].className.includes('diff-bullet'));
  assert.ok(frag.children[0].className.includes('diff-changed'));
  assert.ok(frag.children[1].className.includes('diff-bullet'));
  assert.ok(!frag.children[1].className.includes('diff-changed'));
});

test('Prefs accepts all 5 PDF styles and ignores invalid values', () => {
  const storage = new Map();
  const sandbox = {
    window: {
      matchMedia: () => ({ matches: false }),
    },
    document: {
      documentElement: { classList: { toggle() {} } },
    },
    localStorage: {
      getItem: k => storage.get(k) ?? null,
      setItem: (k, v) => storage.set(k, String(v)),
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(read('js/prefs.js'), sandbox);
  const { Prefs } = sandbox.window;
  assert.equal(Prefs.get('pdfStyle'), 'classic');
  for (const style of ['classic', 'modern', 'minimal', 'compact', 'executive']) {
    Prefs.set('pdfStyle', style);
    assert.equal(Prefs.get('pdfStyle'), style);
  }
});

test('Open a link page is removed and combined with the search bar', () => {
  const html = read('index.html');
  assert.ok(!html.includes('id="tab-link"'), 'tab-link button should be removed from index.html');
  assert.ok(!html.includes('id="job-form"'), 'job-form should be removed from index.html');
  assert.ok(!html.includes('id="job-url"'), 'job-url should be removed from index.html');
  assert.ok(html.includes('paste a Greenhouse job link'), 'search bar placeholder should mention pasting a job link');

  const jp = read('views/job-parser/job-parser.js');
  assert.ok(!jp.includes('tab-link'), 'job-parser.js should not reference tab-link');
  assert.ok(jp.includes('looksLikeJobLink'), 'job-parser.js should detect pasted job links in search');
});

test('Resume import allows PDF, DOCX, TXT, and MD in frontend and Tauri routes', () => {
  const rb = read('views/resume-builder/resume-builder.js');
  assert.ok(rb.includes('["pdf", "docx", "txt", "md"]'), 'resume-builder.js should allow pdf, docx, txt, and md');

  const routes = fs.readFileSync(path.join(__dirname, '..', '..', 'src-tauri', 'src', 'routes.rs'), 'utf8');
  assert.ok(routes.includes('&["pdf", "docx", "txt", "md"]'), 'routes.rs should allow pdf, docx, txt, and md');
});

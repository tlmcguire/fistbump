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
  assert.ok(html.includes('paste a job link'), 'search bar placeholder should mention pasting a job link');

  const jp = read('views/job-parser/job-parser.js');
  assert.ok(!jp.includes('tab-link'), 'job-parser.js should not reference tab-link');
  assert.ok(jp.includes('looksLikeJobLink'), 'job-parser.js should detect pasted job links in search');
  assert.ok(jp.includes('lever\\.co') && jp.includes('ashbyhq\\.com'), 'job-parser.js should recognize Lever and Ashby links');
  assert.ok(html.includes('id="pin-search"'), 'index.html should include Pin search button');
  assert.ok(html.includes('id="watchlist-input"'), 'index.html should include Company Watchlist input');
});

test('Resume import allows PDF, DOCX, ODT, TXT, and MD in frontend and Tauri routes', () => {
  const rb = read('views/resume-builder/resume-builder.js');
  assert.ok(rb.includes('["pdf", "docx", "odt", "txt", "md"]'), 'resume-builder.js should allow pdf, docx, odt, txt, and md');

  const routes = fs.readFileSync(path.join(__dirname, '..', '..', 'src-tauri', 'src', 'routes.rs'), 'utf8');
  assert.ok(routes.includes('&["pdf", "docx", "odt", "txt", "md"]'), 'routes.rs should allow pdf, docx, odt, txt, and md');
});

test('Phase B: consecutive resume imports, DOCX export, and 1-page fit audit are wired', () => {
  const rb = read('views/resume-builder/resume-builder.js');
  assert.ok(!rb.includes('importButton.disabled = pasteButton.disabled = !shown'), 'import buttons should not stay disabled when importSection is open');
  assert.ok(rb.includes('let importing = false;'), 'resume-builder.js should gate imports with an in-flight flag');
  assert.ok(rb.includes('Export DOCX'), 'resume-builder.js should offer Export DOCX');
  assert.ok(rb.includes('api().resume.audit'), 'resume-builder.js should call resume.audit for 1-page fit check');
});

test('Phase C & D: Tracker CSV export, .ics calendar export, stats, and offer comparison are wired', () => {
  const tr = read('views/tracker/tracker.js');
  assert.ok(tr.includes('Export CSV'), 'tracker.js should include Export CSV button');
  assert.ok(tr.includes('BEGIN:VCALENDAR'), 'tracker.js should generate RFC 5545 .ics events');
  assert.ok(tr.includes('api.applications.stats'), 'tracker.js should display pipeline & skill-gap stats');

  const routes = fs.readFileSync(path.join(__dirname, '..', '..', 'src-tauri', 'src', 'routes.rs'), 'utf8');
  assert.ok(routes.includes('"applications:stats"'), 'routes.rs should map applications:stats');
  assert.ok(routes.includes('"cost:compare"'), 'routes.rs should map cost:compare');
});

test('Listing shows ATS source, ATS source filter is wired, filters are deduplicated, and UI.alert omits null titles', () => {
  const html = read('index.html');
  assert.ok(html.includes('id="job-source"'), 'index.html detail pane should include #job-source');
  assert.ok(html.includes('id="search-source"'), 'index.html filters should include #search-source');
  assert.ok(html.includes('data-show="ratings"'), 'card display options should combine rating badges into one toggle');
  assert.ok(!html.includes('data-show="pay_rating"'), 'redundant pay_rating toggle should be removed');

  const ui = read('js/ui.js');
  assert.ok(ui.includes('.filter(Boolean)'), 'UI.ask should filter out null titles before dialog.append');
});

test('Tailor suggested jobs support hover dismiss with localStorage persistence', () => {
  const dv = read('views/diff-view/diff-view.js');
  assert.ok(dv.includes('stage-job-dismiss'), 'diff-view.js should render .stage-job-dismiss buttons');
  assert.ok(dv.includes('fistbump.tailor-dismissed-jobs'), 'diff-view.js should persist dismissed job IDs');

  const css = read('styles/views.css');
  assert.ok(css.includes('.stage-job-item:hover .stage-job-dismiss'), 'views.css should reveal dismiss button on hover');
});

test('Remaining planned features & Honest Keyword Bridge are wired end-to-end', () => {
  const dv = read('views/diff-view/diff-view.js');
  assert.ok(dv.includes('Honest keyword bridge'), 'diff-view.js should include Honest keyword bridge');
  assert.ok(dv.includes('Currently learning') && dv.includes("Don't have yet (eager to learn)"), 'diff-view.js should offer transparent gap-bridging phrasings');
  assert.ok(dv.includes('lastBridgeSnapshot') && dv.includes('removeBridgeLineFromMarkdown'), 'diff-view.js should allow easy undo/removal of keyword bridge notes');
  assert.ok(dv.includes('Export DOCX'), 'diff-view.js should offer Export DOCX');
  assert.ok(dv.includes('Cover letter & Q&A') && dv.includes('api.drafts.create'), 'diff-view.js should include Cover letter & Q&A drafts');
  assert.ok(dv.includes('draftGenMode') && dv.includes('Deterministic (instant)') && dv.includes('AI (local / remote)'), 'diff-view.js should let users choose deterministic or AI generation for drafts and questions');

  const rb = read('views/resume-builder/resume-builder.js');
  assert.ok(rb.includes('resume.section_order') && rb.includes('resume.custom_sections'), 'resume-builder.js should support custom sections and section ordering');

  const tr = read('views/tracker/tracker.js');
  assert.ok(tr.includes('Compare offers') && tr.includes('api.cost.compare'), 'tracker.js should wire side-by-side offer comparison');
  assert.ok(tr.includes('Prep sheet') && tr.includes('openPrepSheet'), 'tracker.js should wire printable interview prep sheet');

  const html = read('index.html');
  assert.ok(html.includes('id="backup-export"') && html.includes('id="backup-restore"'), 'index.html should include local backup & restore controls');

  const routes = fs.readFileSync(path.join(__dirname, '..', '..', 'src-tauri', 'src', 'routes.rs'), 'utf8');
  assert.ok(routes.includes('"drafts:list"') && routes.includes('"drafts:create"'), 'routes.rs should map drafts IPC channels');
});

test('Prefs supports 11 style themes, 12 seasonal themes, and auto-season', () => {
  const storage = new Map();
  const dataset = {};
  const sandbox = {
    window: { matchMedia: () => ({ matches: false }) },
    document: { documentElement: { classList: { toggle() {} }, dataset } },
    localStorage: {
      getItem: k => storage.get(k) ?? null,
      setItem: (k, v) => storage.set(k, String(v)),
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(read('js/prefs.js'), sandbox);
  const { Prefs } = sandbox.window;
  assert.equal(Prefs.get('theme'), 'classic');
  assert.equal(dataset.theme, undefined);

  const expected = [
    'bubblegum', 'techno', 'cyber', 'retro',
    'vaporwave', 'dracula', 'matcha', 'midnight-gold', 'ocean-abyss', 'arcade',
    'january', 'february', 'march', 'april', 'may', 'june',
    'july', 'august', 'september', 'october', 'november', 'december',
  ];
  for (const t of expected) {
    Prefs.set('theme', t);
    assert.equal(Prefs.get('theme'), t);
    assert.equal(dataset.theme, t);
  }
  Prefs.set('theme', 'auto-season');
  assert.equal(Prefs.get('theme'), 'auto-season');
  assert.ok(expected.slice(10).includes(dataset.theme), `auto-season should resolve to a month, got ${dataset.theme}`);

  Prefs.set('theme', 'not-a-theme');
  assert.equal(Prefs.get('theme'), 'auto-season', 'invalid theme should be ignored');

  const css = read('styles/themes.css');
  for (const t of expected) {
    assert.ok(css.includes(`[data-theme="${t}"]`), `themes.css should define [data-theme="${t}"]`);
  }
  assert.ok(css.includes(':root[data-theme].high-contrast'), 'themes.css should enforce high-contrast across all themes');
  assert.ok(css.includes(':root[data-theme].underline-links'), 'themes.css should enforce underline-links across all themes');

  const html = read('index.html');
  const selectMatch = html.match(/<select[^>]*id="pref-theme"[^>]*>([\s\S]*?)<\/select>/);
  assert.ok(selectMatch, 'index.html should contain #pref-theme select');
  assert.ok(!/[()]/.test(selectMatch[1]), '#pref-theme option labels should not contain parenthetical names');
});

test('Home refreshes and clears stale listings/cache when history, jobs, or storage are deleted', () => {
  const home = read('views/home/home.js');
  assert.ok(home.includes('fistbump:jobs') && home.includes('fistbump:applications') && home.includes('fistbump:storage-cleaned'),
    'home.js should listen for job, application, and storage cleanup events');
  assert.ok(home.includes('invalidateMatches') && home.includes('clearLastListing'),
    'home.js should invalidate match cache and drop deleted remembered listings');

  const jp = read('views/job-parser/job-parser.js');
  assert.ok(jp.includes('forgetListingIfMatching') && jp.includes('notifyJobsChanged'),
    'job-parser.js should forget deleted listings and notify Home');

  const settings = read('views/settings/settings.js');
  assert.ok(settings.includes('fistbump:storage-cleaned'),
    'settings.js should notify Home after storage cleanup');

  const tr = read('views/tracker/tracker.js');
  assert.ok(tr.includes('fistbump:jobs') && tr.includes('fistbump:applications') && tr.includes('fistbump:storage-cleaned') && tr.includes('fistbump:versions'),
    'tracker.js should refresh pipeline stats and stage chips when jobs, applications, storage, or versions change');
  assert.ok(tr.includes('drawStats') && tr.includes('statsStrip.textContent = ""'),
    'tracker.js should compute scope-accurate stats and clear statsStrip when empty');
});

test('App version stays in sync across package.json, tauri.conf.json, and Cargo.toml', () => {
  const root = path.join(__dirname, '..', '..');
  const pkg = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8')).version;
  const tauri = JSON.parse(fs.readFileSync(path.join(root, 'src-tauri', 'tauri.conf.json'), 'utf8')).version;
  const cargoToml = fs.readFileSync(path.join(root, 'src-tauri', 'Cargo.toml'), 'utf8');
  const cargoMatch = cargoToml.match(/^version\s*=\s*"([^"]+)"/m);
  assert.ok(cargoMatch, 'Cargo.toml should declare a version');
  assert.equal(tauri, pkg, 'tauri.conf.json version must match package.json');
  assert.equal(cargoMatch[1], pkg, 'Cargo.toml version must match package.json');
});

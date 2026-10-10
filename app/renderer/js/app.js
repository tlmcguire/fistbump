import { api } from './api.js';
import { icon } from './icons.js';
import { request } from './bus.js';
import { h } from './dom.js';
import { modal, run, themeSwitch, toast } from './ui.js';

// [name, label, loader, keepAlive]. Kept-alive views stay mounted (hidden) when you switch tabs, so work
// in progress and background polling continue. Others are rebuilt each visit to show fresh data.
// Settings is not a view: it opens as a sheet over whatever you are doing.
const routes = [
  ['home', 'Home', () => import('../views/home/home.js'), false],
  ['resume', 'Resume', () => import('../views/resume-builder/resume-builder.js'), true],
  ['jobs', 'Jobs', () => import('../views/job-parser/job-parser.js'), true],
  ['revisions', 'Tailor', () => import('../views/diff-view/diff-view.js'), true],
  ['tracker', 'Tracker', () => import('../views/tracker/tracker.js'), false],
];

const view = document.getElementById('view');
const nav = document.getElementById('nav');
const mounted = new Map(); // name -> { el, key, inst, scroll }
let token = 0;
let lastRoute = 'home';
document.documentElement.dataset.platform = window.fistbump?.platform || 'web';

// Hash format: #/name?key=value&key2=value2
function parseHash() {
  const [path, query = ''] = location.hash.replace(/^#\/?/, '').split('?');
  return { name: path || 'home', params: Object.fromEntries(new URLSearchParams(query)) };
}

export function navigate(name, params = {}) {
  if (name === 'settings') { openSettings(params.section); return; }
  const q = new URLSearchParams(params).toString();
  location.hash = `#/${name}${q ? `?${q}` : ''}`;
}

export async function openSettings(section) {
  const mod = await import('../views/settings/settings.js');
  mod.openSettings(section);
}

function unmount(name) {
  const m = mounted.get(name);
  if (!m) return;
  try { (typeof m.inst === 'function' ? m.inst : m.inst?.cleanup)?.(); } catch { /* ignore */ }
  m.el.remove();
  mounted.delete(name);
}

async function render() {
  const { name, params } = parseHash();
  if (name === 'settings') { // old links: open the sheet over the last view
    history.replaceState(null, '', `#/${lastRoute}`);
    openSettings(params.section);
    return;
  }
  const route = routes.find((r) => r[0] === name) || routes[0];
  const [rname, , load, keep] = route;
  lastRoute = rname;
  const key = JSON.stringify(params);
  const mine = ++token;
  for (const a of nav.querySelectorAll('a')) {
    if (a.dataset.route === rname) a.setAttribute('aria-current', 'page');
    else a.removeAttribute('aria-current');
  }
  for (const [n, m] of mounted) m.el.hidden = n !== rname;

  // Plain tab clicks (no parameters) return to a kept-alive view as you left it.
  const existing = mounted.get(rname);
  if (existing && keep && (Object.keys(params).length === 0 || existing.key === key)) {
    existing.el.hidden = false;
    try { existing.inst?.onShow?.(); } catch { /* ignore */ }
    view.scrollTop = existing.scroll || 0;
    view.classList.toggle('scrolled', view.scrollTop > 2);
    return;
  }
  unmount(rname);
  const el = h('div', { class: 'view-root' });
  view.append(el);
  mounted.set(rname, { el, key, inst: null });
  view.scrollTop = 0;
  try {
    const mod = await load();
    if (mine !== token && parseHash().name !== rname) return;
    const inst = await mod.mount(el, { params, navigate, toast });
    const m = mounted.get(rname);
    if (m && m.el === el) m.inst = inst;
  } catch (err) {
    el.append(h('div', { class: 'notice error' }, `Could not load this view: ${err.message}`));
  }
}

// Remember each view's scroll position so returning to a tab lands where you were.
view.addEventListener('scroll', () => {
  const m = mounted.get(parseHash().name);
  if (m) m.scroll = view.scrollTop;
  view.classList.toggle('scrolled', view.scrollTop > 2); // toolbar hairline only once content moves under it
});

async function refreshPill() {
  const pill = document.getElementById('status-pill');
  try {
    const st = await api.ai.status();
    const label = { local: 'Local AI', remote: 'Remote AI', rules: 'Rules only' }[st.active_engine] || st.active_engine;
    pill.replaceChildren(h('span', { class: 'engine-dot' }), h('span', { class: 'label' }, label));
    pill.dataset.engine = st.active_engine;
    pill.title = 'The engine that writes suggestions. Click to change it.';
  } catch {
    pill.textContent = 'Engine unknown';
  }
}
window.addEventListener('fistbump:ai-changed', refreshPill);

// Background watcher: suggestion rounds keep running in the backend while you use other tabs. The Tailor
// tab shows a dot while any round runs. When one finishes you get a notice in the app, and a system
// notification if fistbump is not in front.
const watching = new Map(); // revision id -> job id
let watchTimer = null;
async function watchRevisions() {
  clearTimeout(watchTimer);
  let revs = [];
  try { revs = await api.revisions.list(); } catch { /* backend restarting */ }
  for (const r of revs.filter((x) => x.status === 'queued' || x.status === 'running')) watching.set(r.id, r.job_id);
  for (const [id, jobId] of watching) {
    const r = revs.find((x) => x.id === id);
    if (!r || r.status === 'queued' || r.status === 'running') continue;
    watching.delete(id);
    const onTailor = parseHash().name === 'revisions' && document.hasFocus();
    if (onTailor) continue;
    let label = 'your job';
    try { const j = await api.jobs.get(jobId); label = `${j.position_title} at ${j.company_name}`; } catch { /* deleted */ }
    const open = () => navigate('revisions', { job: jobId, revision: id });
    if (r.status === 'done') toast(`Suggestions are ready for ${label}.`, 'info', { label: 'Review', onClick: open });
    else if (r.status === 'failed') toast(`Suggestions failed for ${label}: ${r.error || 'unknown error'}`, 'error');
    if (!document.hasFocus() && 'Notification' in window && r.status === 'done') {
      const n = new Notification('Suggestions are ready', { body: label, silent: false });
      n.onclick = () => { window.focus(); open(); };
    }
  }
  const tailor = nav.querySelector('a[data-route="revisions"]');
  tailor?.querySelector('.dot')?.remove();
  if (watching.size) {
    tailor?.append(h('span', { class: 'dot', title: 'Suggestions are being written' }));
    watchTimer = setTimeout(watchRevisions, 1500);
  }
}
window.addEventListener('fistbump:revision-started', watchRevisions);

// Drop a resume anywhere on the window to import it. Any other drop is ignored, so a stray file can
// never replace the app page.
const dropZone = h('div', { class: 'drop-zone', hidden: true }, h('div', { class: 'drop-card' }, h('strong', {}, 'Drop your resume to import it'),
  h('span', { class: 'muted' }, 'PDF, text or Markdown. You will check every field before anything is saved.')));
document.body.append(dropZone);
let dragDepth = 0;
const hasFiles = (e) => [...(e.dataTransfer?.types || [])].includes('Files');
window.addEventListener('dragenter', (e) => { if (!hasFiles(e)) return; e.preventDefault(); dragDepth++; dropZone.hidden = false; });
window.addEventListener('dragleave', (e) => { if (!hasFiles(e)) return; dragDepth = Math.max(0, dragDepth - 1); if (!dragDepth) dropZone.hidden = true; });
window.addEventListener('dragover', (e) => { e.preventDefault(); if (e.dataTransfer) e.dataTransfer.dropEffect = hasFiles(e) ? 'copy' : 'none'; });
window.addEventListener('drop', async (e) => {
  e.preventDefault();
  dragDepth = 0;
  dropZone.hidden = true;
  const file = e.dataTransfer?.files?.[0];
  if (!file) return;
  if (!/\.(pdf|txt|md|markdown)$/i.test(file.name)) { toast('Drop a PDF, .txt or .md resume to import it.', 'error'); return; }
  if (file.size > 5_000_000) { toast('That file is larger than 5 MB.', 'error'); return; }
  toast(`Reading ${file.name}...`);
  const data = new Uint8Array(await file.arrayBuffer());
  const out = await run(() => api.resume.importData({ name: file.name, data }));
  if (out) { request('resume', 'review', out); navigate('resume'); }
});

// Custom CSS (Settings > Appearance) as a constructable stylesheet: it cascades after the app's own
// styles, so ordinary rules override the design tokens.
const customSheet = new CSSStyleSheet();
document.adoptedStyleSheets = [...document.adoptedStyleSheets, customSheet];
async function applyCustomCss() {
  try {
    const r = await api.app.getCustomCss();
    customSheet.replaceSync(r.enabled ? r.css : '');
  } catch (err) {
    toast(`Custom CSS could not be applied: ${err.message}`, 'error');
  }
}
applyCustomCss();

// Menu bar and shortcut commands from the main process.
function focusFind() {
  const root = [...view.querySelectorAll('.view-root')].find((el) => !el.hidden) || view;
  const target = root.querySelector('input[type="search"]') || root.querySelector('input[type="text"]');
  if (target) { target.focus(); target.select?.(); }
}

function showShortcuts() {
  import('../views/settings/settings.js').then((m) => {
    const dlg = modal('Keyboard shortcuts', m.shortcutsTable(), h('div', { class: 'row', style: 'margin-top:12px;justify-content:flex-end' },
      h('button', { class: 'primary', onclick: () => dlg.close() }, 'Done')));
  });
}

window.fistbump?.onCommand(async ({ command, arg }) => {
  switch (command) {
    case 'settings': openSettings(); break;
    case 'navigate': navigate(arg); break;
    case 'paste-job': navigate('jobs', { tab: 'paste' }); break;
    case 'import-resume': request('resume', 'import'); navigate('resume'); break;
    case 'export-resume': {
      const r = await run(() => api.resume.export('pdf'));
      if (r?.saved) toast(`Saved to ${r.path}`);
      break;
    }
    case 'find': focusFind(); break;
    case 'shortcuts': showShortcuts(); break;
    case 'theme-changed': window.dispatchEvent(new CustomEvent('fistbump:theme', { detail: arg })); break;
    case 'custom-css-changed': applyCustomCss(); break;
    default: break;
  }
});

const NAV_ICONS = { home: 'home', resume: 'resume', jobs: 'jobs', revisions: 'tailor', tracker: 'tracker' };
const KEYS = { home: '0', resume: '1', jobs: '2', revisions: '3', tracker: '4' };
const mod = window.fistbump?.platform === 'darwin' ? '⌘' : 'Ctrl+';
for (const [name, label] of routes) {
  nav.append(h('a', { href: `#/${name}`, 'data-route': name, title: `${label} (${mod}${KEYS[name]})` }, icon(NAV_ICONS[name], { size: 18 }), h('span', { class: 'label' }, label)));
}
document.getElementById('settings-btn').replaceChildren(icon('settings', { size: 18 }), h('span', { class: 'label' }, 'Settings'));
document.getElementById('status-pill').addEventListener('click', () => openSettings('ai'));
document.getElementById('settings-btn').addEventListener('click', () => openSettings());
document.getElementById('theme-switch').replaceWith(themeSwitch());
window.addEventListener('hashchange', render);
refreshPill();
watchRevisions();
render();

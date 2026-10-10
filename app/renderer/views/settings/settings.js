import { api } from '../../js/api.js';
import { icon } from '../../js/icons.js';
import { h, clear, fmtBytes } from '../../js/dom.js';
import { badge, confirmAction, empty, field, poll, run, spinner, themeSwitch, toast } from '../../js/ui.js';

const SECTIONS = [
  ['appearance', 'Appearance', 'palette'],
  ['ai', 'AI engine', 'bolt'],
  ['models', 'Local models', 'chip'],
  ['connectors', 'Job search', 'search'],
  ['privacy', 'Privacy and data', 'shield'],
  ['about', 'About', 'info'],
];

let openDialog = null;

// openSettings shows Settings as a sheet over the current view, so you never lose your place.
// It closes with Esc, the close button, or a click on the backdrop.
export function openSettings(section = 'appearance') {
  if (openDialog) { openDialog.show(section); return; }
  const stops = [];
  const body = h('div', { class: 'sheet-body', tabindex: '-1' });
  const nav = h('nav', { class: 'sheet-nav', 'aria-label': 'Settings sections' });
  const sections = { appearance: appearanceSection, ai: aiSection, models: modelsSection, connectors: connectorsSection, privacy: storageSection, about: aboutSection };
  const dlg = h('dialog', { class: 'sheet', 'aria-label': 'Settings' },
    h('div', { class: 'sheet-side' }, h('div', { class: 'sheet-title' }, 'Settings'), nav),
    h('div', { class: 'sheet-main' },
      h('button', { class: 'sheet-close icon-btn', 'aria-label': 'Close settings', title: 'Close (Esc)', onclick: () => dlg.close() }, icon('x', { size: 18 })),
      body));
  const show = (id) => {
    stops.splice(0).forEach((stop) => stop());
    clear(nav);
    for (const [key, label, glyph] of SECTIONS) {
      nav.append(h('button', { class: 'sheet-link', 'aria-current': key === id ? 'page' : null, onclick: () => show(key) },
        icon(glyph), label));
    }
    clear(body);
    body.scrollTop = 0;
    const title = SECTIONS.find((x) => x[0] === id)?.[1] || '';
    body.append(h('h1', { class: 'sheet-heading' }, title));
    sections[id](body, stops).catch((e) => body.append(h('div', { class: 'notice error' }, e.message)));
    body.focus({ preventScroll: true });
  };
  dlg.addEventListener('close', () => { stops.splice(0).forEach((stop) => stop()); dlg.remove(); openDialog = null; });
  dlg.addEventListener('click', (e) => { if (e.target === dlg) dlg.close(); });
  document.body.append(dlg);
  dlg.showModal();
  openDialog = { show };
  show(SECTIONS.some((x) => x[0] === section) ? section : 'appearance');
}

const notifyAI = () => window.dispatchEvent(new Event('fistbump:ai-changed'));

const TOKENS = [
  ['--accent', 'Primary buttons and current selection'], ['--highlight', 'AI-suggested text and the active tab marker'],
  ['--bg', 'Window background'], ['--surface', 'Cards and panels'], ['--text', 'Body text'], ['--muted', 'Secondary text'],
  ['--border', 'Hairlines'], ['--radius', 'Corner rounding'], ['--font', 'Typeface (installed fonts only)'], ['--topbar', 'Top bar background'],
];

async function appearanceSection(body) {
  body.append(h('section', { class: 'card stack' }, h('h2', {}, 'Theme'), themeSwitch({ labels: true }),
    h('p', { class: 'hint' }, 'Also under View > Theme.')));

  // Custom CSS: written to a file in the settings folder and applied by the app as you save.
  const cur = await api.app.getCustomCss();
  const enabled = h('input', { type: 'checkbox', checked: cur.enabled });
  const editor = h('textarea', { class: 'code', spellcheck: 'false', value: cur.css, 'aria-label': 'Custom CSS' });
  const status = h('span', { class: 'hint' });
  const setStatus = (t) => { status.textContent = t; setTimeout(() => { if (status.textContent === t) status.textContent = ''; }, 2500); };
  enabled.addEventListener('change', async () => {
    if (await run(() => api.app.setCustomCss({ enabled: enabled.checked }))) setStatus(enabled.checked ? 'On' : 'Off');
  });
  const save = async (button) => {
    if (await run(() => api.app.setCustomCss({ css: editor.value, enabled: true }), { button })) { enabled.checked = true; setStatus('Saved and applied'); }
  };
  editor.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 's') { e.preventDefault(); save(); }
    if (e.key === 'Tab') { e.preventDefault(); editor.setRangeText('  ', editor.selectionStart, editor.selectionEnd, 'end'); }
  });
  body.append(h('section', { class: 'card stack' },
    h('div', { class: 'row between' }, h('h2', { style: 'margin:0' }, 'Custom CSS'), h('label', { class: 'row small' }, enabled, 'Enabled')),
    h('p', { class: 'muted small' }, 'Restyle the app with your own CSS. It cannot load fonts or images from the internet. If something breaks, turn it off here or delete the file.'),
    editor,
    h('div', { class: 'row' },
      h('button', { class: 'primary', onclick: (e) => save(e.currentTarget) }, 'Save'),
      h('button', { onclick: async () => { if (await run(() => api.app.openCustomCss())) setStatus('Opened. Changes apply when you save the file.'); } }, icon('external', { size: 14 }), 'Edit in your editor'),
      h('button', { class: 'ghost', onclick: async () => {
        if (!confirmAction('Replace your custom CSS with the starter template?')) return;
        const t = await api.app.getCustomCss();
        editor.value = (await import('../../js/custom-css-template.js')).TEMPLATE;
        await run(() => api.app.setCustomCss({ css: editor.value, enabled: t.enabled }));
        setStatus('Reset');
      } }, 'Reset'),
      h('span', { class: 'spacer' }), status),
    h('details', {}, h('summary', { class: 'small' }, 'Design tokens you can override'),
      h('table', { class: 'simple tokens' }, TOKENS.map(([t, what]) => h('tr', {}, h('td', {}, h('code', {}, t)), h('td', { class: 'muted' }, what)))))));
}

async function aboutSection(body) {
  const info = await api.app.info();
  body.append(
    h('section', { class: 'card stack about' }, h('img', { src: 'img/logo.png', alt: '', class: 'about-logo' }),
      h('div', {}, h('h2', { style: 'margin:0' }, 'fistbump'), h('p', { class: 'muted', style: 'margin:0' }, `Version ${info.version}`)),
      h('p', {}, 'The resume assistant that keeps your data yours.')),
    h('section', { class: 'card stack' }, h('h2', {}, 'Keyboard shortcuts'), shortcutsTable()));
}

export function shortcutsTable() {
  const mac = window.fistbump?.platform === 'darwin';
  const k = (key, shift) => (mac ? `⌘${shift ? '⇧' : ''}${key}` : `Ctrl+${shift ? 'Shift+' : ''}${key}`);
  const rows = [
    [k(','), 'Open settings'], [`${k('1')} to ${k('4')}`, 'Resume, Jobs, Tailor, Tracker'], [k('0'), 'Home'],
    [k('N'), 'Paste a job posting'], [k('O'), 'Import a resume (or drop the file on the window)'], [k('E', true), 'Download your resume as PDF'],
    [k('F'), 'Find on the current page'], ['Esc', 'Close a panel or dialog'], [k('/'), 'Show shortcuts'],
  ];
  return h('table', { class: 'simple shortcuts' }, rows.map(([key, what]) => h('tr', {}, h('td', {}, h('kbd', {}, key)), h('td', {}, what))));
}

async function aiSection(body) {
  const [status, info] = await Promise.all([api.ai.status(), api.app.info()]);
  const modeSel = h('select', {}, [['auto', 'Automatic: best available (recommended)'], ['local', 'Local model only'], ['remote', 'Remote provider only'], ['rules', 'No AI: rule-based suggestions only']]
    .map(([v, l]) => h('option', { value: v, selected: v === status.mode }, l)));
  modeSel.addEventListener('change', async () => { if (await run(() => api.ai.setMode(modeSel.value))) { toast('Mode saved'); notifyAI(); } });

  const base = h('input', { type: 'text', value: status.remote.base_url, placeholder: 'https://api.openai.com/v1 or http://localhost:11434/v1' });
  const model = h('input', { type: 'text', value: status.remote.model, placeholder: 'model name' });
  const key = h('input', { type: 'password', placeholder: status.remote.has_key ? 'Key saved. Type to replace.' : 'Optional for local servers like Ollama', autocomplete: 'off' });
  const out = h('div');
  const testBox = h('div', { class: 'small' });

  body.append(
    h('section', { class: 'card stack' }, h('h2', {}, 'Suggestion engine'), field('Use', modeSel),
      h('p', { class: 'muted small' }, `Active now: ${({ local: `local model (${status.local.model})`, remote: `remote provider (${status.remote.model})`, rules: 'rule-based suggestions, no AI' })[status.active_engine]}. `,
        'Automatic tries the local model, then a remote provider, then rules. Skill matching never uses AI.'),
      status.local.installed && status.local.binary_found ? h('div', { class: 'notice info' }, 'Local AI is set up. It runs on this computer; nothing is sent out.')
        : h('div', { class: 'notice info' }, 'To use AI without sending data anywhere, download a model in the Local models tab.')),
    h('section', { class: 'card stack' }, h('h2', {}, 'Remote provider (optional)'),
      h('div', { class: 'notice' }, 'When enabled, your resume text and the job posting are sent to this provider to write suggestions.'),
      status.remote.needs_key ? h('div', { class: 'notice error' }, 'This provider has no API key right now, so it is skipped. Enter the key again below. On Linux this happens when no keyring is available to store it.') : null,
      h('div', { class: 'form-grid' }, h('div', { class: 'wide' }, field('Base URL', base, 'HTTPS is required unless the host is localhost.')), h('div', {}, field('Model', model)),
        h('div', {}, field('API key', key, info.key_storage_encrypted ? 'Stored encrypted by your operating system. Never shown again.' : 'Encryption is unavailable here, so the key is kept for this session only.'))),
      h('div', { class: 'row' },
        h('button', { class: 'primary', onclick: async (e) => {
          const args = { base_url: base.value.trim(), model: model.value.trim() };
          if (key.value) args.api_key = key.value;
          const r = await run(() => api.ai.setRemote(args), { button: e.target });
          if (r) { toast('Remote provider saved'); notifyAI(); key.value = ''; clear(body); aiSection(body); }
        } }, 'Save'),
        h('button', { onclick: async (e) => {
          clear(testBox); testBox.append(spinner('Testing...'));
          const r = await run(() => api.ai.test('remote'), { button: e.target });
          clear(testBox);
          if (r) testBox.append(r.ok ? h('span', { class: 'chip good' }, `OK in ${r.latency_ms} ms`) : h('span', { class: 'error-text' }, r.error));
        } }, 'Test connection'),
        status.remote.configured ? h('button', { class: 'danger', onclick: async () => {
          if (!confirmAction('Remove the remote provider and delete the stored key?')) return;
          if (await run(() => api.ai.clearRemote()) !== undefined) { notifyAI(); clear(body); aiSection(body); }
        } }, 'Remove') : null, testBox)),
    out);
}

async function modelsSection(body, stops) {
  const [models, status] = await Promise.all([api.models.list(), api.ai.status()]);
  const list = h('div');
  const downloads = new Map(); // model id -> download id
  const testOut = h('div', { class: 'small' });
  body.append(h('section', { class: 'card stack' }, h('h2', {}, 'Local models'),
    h('p', { class: 'muted' }, 'Models run on this computer through llama-server. They download on demand from Hugging Face and are verified with SHA-256. Nothing is downloaded until you click Download.'),
    status.local.binary_found ? null : h('div', { class: 'notice' }, 'llama-server was not found. Run "node scripts/fetch-llama-server.js", install llama.cpp so llama-server is on your PATH, or set FISTBUMP_LLAMA_SERVER.'),
    list,
    h('div', { class: 'row' }, h('button', { onclick: async (e) => {
      clear(testOut); testOut.append(spinner('Starting the model, this can take a minute...'));
      const r = await run(() => api.ai.test('local'), { button: e.target });
      clear(testOut);
      if (r) testOut.append(r.ok ? h('span', { class: 'chip good' }, `OK in ${r.latency_ms} ms`) : h('span', { class: 'error-text' }, r.error));
    } }, 'Test selected model')), testOut));

  function draw(items) {
    clear(list);
    for (const m of items) {
      const prog = h('div');
      const row = h('div', { class: 'list-item' }, h('div', { class: 'grow' }, h('strong', {}, m.name), h('div', { class: 'small muted' }, `${fmtBytes(m.size_bytes)} · needs about ${m.min_ram_gb} GB RAM${m.fits_this_machine ? '' : ' (more than this computer has)'}`), prog),
        m.installed ? [m.selected ? badge('selected', 'good') : h('button', { onclick: async () => { if (await run(() => api.settings.set({ 'ai.selected_model': m.id }))) { toast('Model selected'); notifyAI(); refresh(); } } }, 'Use this model'),
          h('button', { class: 'danger', onclick: async () => {
            if (!confirmAction(`Delete ${m.name} from disk?`)) return;
            if (await run(() => api.models.remove(m.id)) !== undefined) { notifyAI(); refresh(); }
          } }, 'Delete')]
          : h('button', { onclick: async (e) => {
            const d = await run(() => api.models.download(m.id), { button: e.target });
            if (d) { downloads.set(m.id, d.download_id); watch(m, prog, d.download_id); }
          } }, 'Download'));
      list.append(row);
      if (downloads.has(m.id)) watch(m, prog, downloads.get(m.id));
    }
  }
  function watch(m, prog, id) {
    const stop = poll(async () => {
      const st = await api.models.downloadStatus(id);
      clear(prog);
      if (st.state === 'running' || st.state === 'verifying') {
        const pct = st.total ? Math.round((st.bytes / st.total) * 100) : 0;
        prog.append(h('div', { class: 'meter', style: 'margin-top:4px' }, h('div', { style: `width:${pct}%` })), h('div', { class: 'small muted' }, st.state === 'verifying' ? 'Verifying checksum...' : `${fmtBytes(st.bytes)} of ${fmtBytes(st.total)}`),
          h('button', { class: 'link', onclick: () => run(() => api.models.cancelDownload(id)) }, 'Cancel'));
        return false;
      }
      downloads.delete(m.id);
      if (st.state === 'done') { toast(`${m.name} downloaded`); refresh(); }
      else prog.append(h('div', { class: st.state === 'failed' ? 'error-text' : 'muted small' }, st.state === 'failed' ? st.error : 'Canceled. The partial file is kept so a later download resumes.'));
      return true;
    }, 1000);
    stops.push(stop);
  }
  async function refresh() { draw(await api.models.list()); }
  draw(models);

  const tok = h('input', { type: 'password', placeholder: 'hf_... (only for gated models)', autocomplete: 'off' });
  body.append(h('section', { class: 'card stack' }, h('h2', {}, 'Hugging Face token (optional)'),
    h('div', { class: 'row' }, tok, h('button', { onclick: async () => { if (tok.value && await run(() => api.models.setToken(tok.value)) !== undefined) { toast('Token saved'); tok.value = ''; } } }, 'Save'),
      h('button', { onclick: async () => { if (await run(() => api.models.clearToken()) !== undefined) toast('Token removed'); } }, 'Remove'))));

  const settings = await api.settings.get();
  const idle = h('input', { type: 'number', value: settings['ai.idle_minutes'], min: '1' });
  body.append(h('section', { class: 'card stack' }, h('h2', {}, 'Local server'),
    h('div', { class: 'row' }, field('Stop the model after (minutes idle)', idle), h('button', { style: 'align-self:flex-end', onclick: async () => {
      if (await run(() => api.settings.set({ 'ai.idle_minutes': parseInt(idle.value, 10) }))) toast('Saved');
    } }, 'Save'))));
}

async function connectorsSection(body) {
  const [connectors, cats] = await Promise.all([api.connectors.list(), api.connectors.categories()]);
  const gh = connectors.find((c) => c.id === 'greenhouse');
  const enable = h('input', { type: 'checkbox', checked: gh.enabled });
  enable.addEventListener('change', async () => {
    if (await run(() => api.connectors.setEnabled('greenhouse', enable.checked))) toast(enable.checked ? 'Job search turned on' : 'Job search turned off');
    else enable.checked = !enable.checked;
  });
  const total = cats.categories.filter((c) => !c.parent).reduce((n, c) => n + c.board_count, 0);
  body.append(h('section', { class: 'card stack' }, h('h2', {}, 'Job search (Greenhouse)'),
    h('label', { class: 'row' }, enable, 'Search company job boards from the Jobs page'),
    h('p', { class: 'muted' }, `Searches about ${total} companies automatically and ranks results against your resume. Requests go to Greenhouse only when you search or save a posting. Listings are cached for a few hours.`),
    h('div', { class: 'notice info' }, cats.notice, ` Company list last checked ${cats.verified_at}.`)));

  const board = h('input', { type: 'text', placeholder: 'e.g. acme (from boards.greenhouse.io/acme)' });
  const boardList = h('div');
  let boards = cats.custom_boards;
  const drawBoards = () => {
    clear(boardList);
    if (!boards.length) boardList.append(h('p', { class: 'muted small' }, 'None added.'));
    for (const b of boards) boardList.append(h('div', { class: 'list-item' }, h('div', { class: 'grow' }, b), h('button', { class: 'danger', onclick: async () => {
      if (await run(() => api.connectors.removeBoard(b)) !== undefined) { boards = boards.filter((x) => x !== b); drawBoards(); }
    } }, 'Remove')));
  };
  drawBoards();
  body.append(h('section', { class: 'card stack' }, h('h2', {}, 'Add a company'),
    h('p', { class: 'muted small' }, 'Missing a company that uses Greenhouse? Add its board name and it will be included in every search.'),
    h('div', { class: 'row' }, board, h('button', { onclick: async (e) => {
      const r = await run(() => api.connectors.addBoard(board.value.trim()), { button: e.target });
      if (r) { toast(`Added ${r.company}: ${r.open_postings} open jobs`); board.value = ''; boards = [...new Set([...boards, r.board])]; drawBoards(); }
    } }, 'Add')), boardList));
}

async function storageSection(body) {
  const [st, settings, paths] = await Promise.all([api.storage.get(), api.settings.get(), api.app.paths()]);
  body.append(h('section', { class: 'card stack' }, h('h2', {}, 'Where your data lives'),
    h('p', {}, 'Everything you enter stays on this computer: your resume, saved jobs, tailored resumes and applications are in one database in this folder. Removing the app does not remove it; delete the folder to erase your data.'),
    h('p', { class: 'muted small selectable' }, paths.data_dir),
    h('div', { class: 'row' }, h('button', { onclick: () => run(() => api.app.openDataFolder()) }, 'Open data folder')),
    h('h3', { style: 'margin-top:8px' }, 'What leaves this computer'),
    h('ul', { class: 'plain-list' },
      h('li', {}, 'Job search downloads public job listings from Greenhouse. Your resume and search terms are not sent.'),
      h('li', {}, 'Model downloads come from Hugging Face, only when you click Download.'),
      h('li', {}, 'A remote AI provider, only if you set one up, receives your resume and the job posting when it writes suggestions.'))));
  const row = (k, v) => h('tr', {}, h('td', {}, k), h('td', {}, v));
  const targets = { cache: 'Cache', partial_downloads: 'Partial downloads', old_jobs: 'Old jobs (no application)', unused_models: 'Models not selected' };
  const checks = Object.fromEntries(Object.keys(targets).map((k) => [k, h('input', { type: 'checkbox' })]));
  const days = h('input', { type: 'number', min: '0', value: settings['jobs.retention_days'] });
  const result = h('div', { class: 'small' });
  body.append(
    h('section', { class: 'card stack' }, h('h2', {}, 'Disk usage'),
      h('table', { class: 'simple' }, row('Database', fmtBytes(st.db_bytes)), row('Models', fmtBytes(st.models_bytes)), row('Cache', fmtBytes(st.cache_bytes)),
        row('Partial downloads', fmtBytes(st.partial_downloads_bytes)), row('Jobs stored', st.job_count), row('Total', fmtBytes(st.total_bytes)))),
    h('section', { class: 'card stack' }, h('h2', {}, 'Clean up'),
      h('p', { class: 'muted small' }, 'Preview first. Jobs with an application are never removed.'),
      Object.entries(targets).map(([k, label]) => h('label', { class: 'row' }, checks[k], label)),
      field('Job retention (days since last seen)', days),
      h('div', { class: 'row' },
        h('button', { onclick: (e) => go(e, true) }, 'Preview'), h('button', { class: 'danger', onclick: (e) => go(e, false) }, 'Clean up now')), result));
  async function go(e, dryRun) {
    const picked = Object.keys(checks).filter((k) => checks[k].checked);
    if (!picked.length) { toast('Pick something to clean', 'error'); return; }
    if (!dryRun && !confirmAction('Permanently remove the selected items?')) return;
    const r = await run(() => api.storage.cleanup({ targets: picked, older_than_days: parseInt(days.value, 10), dry_run: dryRun }), { button: e.target });
    if (!r) return;
    clear(result);
    result.append(`${dryRun ? 'Would remove' : 'Removed'} ${r.removed.old_jobs} jobs and ${r.removed.files} files, freeing ${fmtBytes(r.freed_bytes)}.`);
    if (!dryRun) { clear(body); storageSection(body); }
  }
}

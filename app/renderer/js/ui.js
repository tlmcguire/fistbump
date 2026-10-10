import { h, clear } from './dom.js';
import { icon } from './icons.js';

// toast shows a short message. action is { label, onClick } for a button inside the toast.
export function toast(message, kind = 'info', action) {
  const box = document.getElementById('toasts');
  const t = h('div', { class: `toast ${kind === 'error' ? 'error' : ''}` }, message,
    action ? h('button', { class: 'toast-action', onclick: () => { t.remove(); action.onClick(); } }, action.label) : null);
  box.append(t);
  setTimeout(() => t.remove(), action ? 12000 : kind === 'error' ? 7000 : 3500);
}

// run() calls fn, toasts any error, and returns undefined on failure.
export async function run(fn, { button } = {}) {
  if (button) button.disabled = true;
  try {
    return await fn();
  } catch (err) {
    toast(err.message || String(err), 'error');
    return undefined;
  } finally {
    if (button) button.disabled = false;
  }
}

export function spinner(text) {
  return h('span', { class: 'muted' }, h('span', { class: 'spinner' }), text ? ` ${text}` : '');
}

export function empty(text) {
  return h('div', { class: 'empty' }, text);
}

export function chips(list, cls = '') {
  return h('div', { class: 'chips' }, (list || []).map((s) => h('span', { class: `chip ${cls}` }, s)));
}

export function badge(text, kind = '') {
  return h('span', { class: `badge ${kind}` }, text);
}

export function field(label, input, hint) {
  return h('label', { class: 'field' }, h('span', {}, label), input, hint ? h('div', { class: 'hint' }, hint) : null);
}

export const splitList = (s) => s.split(',').map((x) => x.trim()).filter(Boolean);

// formFor builds a form from field specs and calls onSubmit with typed values.
// Spec: { name, label, type: text|textarea|select|list|int|float|date, options, required, wide, hint, placeholder }
export function formFor(specs, values, { submitLabel = 'Save', onSubmit, onCancel } = {}) {
  const inputs = {};
  const grid = h('div', { class: 'form-grid' });
  for (const f of specs) {
    const v = values?.[f.name];
    let input;
    if (f.type === 'textarea') input = h('textarea', { value: v ?? '', placeholder: f.placeholder });
    else if (f.type === 'select') {
      input = h('select', {}, (f.options || []).map((o) => {
        const [val, label] = Array.isArray(o) ? o : [o, o || 'None'];
        return h('option', { value: val, selected: (v ?? '') === val }, label);
      }));
    } else {
      const type = { int: 'number', float: 'number', date: 'text' }[f.type] || 'text';
      const shown = f.type === 'list' ? (v || []).join(', ') : (v ?? '');
      input = h('input', { type, value: shown, placeholder: f.placeholder || (f.type === 'date' ? 'YYYY-MM-DD' : undefined), step: f.type === 'float' ? '0.01' : undefined });
    }
    inputs[f.name] = input;
    grid.append(h('div', { class: f.wide ? 'wide' : '' }, field(f.label + (f.required ? ' *' : ''), input, f.hint)));
  }
  const errorBox = h('div', { class: 'error-text' });
  const read = () => {
    const out = {};
    for (const f of specs) {
      const raw = inputs[f.name].value;
      if (f.type === 'list') out[f.name] = splitList(raw);
      else if (f.type === 'int') out[f.name] = raw === '' ? null : parseInt(raw, 10);
      else if (f.type === 'float') out[f.name] = raw === '' ? null : parseFloat(raw);
      else out[f.name] = raw.trim() === '' ? (f.required ? '' : null) : raw.trim();
    }
    return out;
  };
  const submit = h('button', { class: 'primary', type: 'submit', style: submitLabel ? '' : 'display:none' }, submitLabel || 'Save');
  const form = h('form', {
    class: 'stack',
    onsubmit: async (e) => {
      e.preventDefault();
      errorBox.textContent = '';
      submit.disabled = true;
      try {
        if (onSubmit) await onSubmit(read());
      } catch (err) {
        errorBox.textContent = err.message || String(err);
      } finally {
        submit.disabled = false;
      }
    },
  }, grid, errorBox, h('div', { class: 'row' }, submit, onCancel ? h('button', { type: 'button', onclick: onCancel }, 'Cancel') : null));
  form.read = read; // lets callers collect values without submitting
  form.inputs = inputs;
  return form;
}

export function confirmAction(message) {
  return window.confirm(message);
}

// tabs renders a tab row. Arrow keys, Home and End move between tabs, as in native tab controls.
export function tabs(items, selected, onSelect) {
  const list = h('div', { class: 'tabs', role: 'tablist' }, items.map(([id, label]) =>
    h('button', { role: 'tab', 'aria-selected': id === selected ? 'true' : 'false', tabindex: id === selected ? '0' : '-1', dataset: { tab: id }, onclick: () => onSelect(id) }, label)));
  list.addEventListener('keydown', (e) => {
    const ids = items.map(([id]) => id);
    const i = ids.indexOf(document.activeElement?.dataset?.tab);
    if (i < 0) return;
    const next = { ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: ids.length - 1 }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    const id = ids[(next + ids.length) % ids.length];
    onSelect(id);
    requestAnimationFrame(() => document.querySelector(`.tabs [data-tab="${id}"]`)?.focus());
  });
  return list;
}

// poll calls fn every ms until it returns true or stop() is called.
export function poll(fn, ms = 1000) {
  let stopped = false;
  let timer = null;
  const tick = async () => {
    if (stopped) return;
    let done = false;
    try { done = await fn(); } catch { done = false; }
    if (!stopped && !done) timer = setTimeout(tick, ms);
  };
  timer = setTimeout(tick, ms);
  return () => { stopped = true; clearTimeout(timer); };
}

export { clear };

// renderMarkdown turns the resume Markdown subset into DOM nodes (no innerHTML).
export function renderMarkdown(md) {
  const root = h('div', { class: 'preview-doc' });
  let list = null;
  for (const raw of md.split('\n')) {
    const line = raw.trimEnd();
    if (line.startsWith('- ') || line.startsWith('* ')) {
      if (!list) { list = h('ul'); root.append(list); }
      list.append(h('li', {}, line.slice(2)));
      continue;
    }
    list = null;
    if (line.startsWith('### ')) root.append(h('h3', {}, line.slice(4)));
    else if (line.startsWith('## ')) root.append(h('h2', {}, line.slice(3)));
    else if (line.startsWith('# ')) root.append(h('h1', {}, line.slice(2)));
    else if (line) root.append(h('p', {}, line.startsWith('\\#') ? line.slice(1) : line));
  }
  return root;
}

export function modal(title, ...content) {
  const dlg = h('dialog', {}, h('h2', {}, title), ...content);
  dlg.addEventListener('close', () => dlg.remove());
  document.body.append(dlg);
  dlg.showModal();
  return dlg;
}

// renderPosting formats cleaned posting text: short standalone lines become headings, "- " lines bullets.
export function renderPosting(text) {
  const root = h('div', { class: 'posting-body' });
  let list = null;
  const lines = (text || '').split('\n').map((l) => l.trim());
  lines.forEach((line, i) => {
    if (!line) { list = null; return; }
    if (/^[-•*]\s+/.test(line)) {
      if (!list) { list = h('ul'); root.append(list); }
      list.append(h('li', {}, line.replace(/^[-•*]\s+/, '')));
      return;
    }
    list = null;
    const next = lines[i + 1] || '';
    const heading = line.length <= 70 && !/[.,;]$/.test(line) && (line.endsWith(':') || line === line.toUpperCase() || /^[-•*]\s+/.test(next));
    root.append(heading ? h('h3', {}, line.replace(/:$/, '')) : h('p', {}, line));
  });
  return root;
}

// showPosting opens a side panel with a job posting. load() returns { text } or a promise of it, so the
// panel can open instantly and fill in. actions are extra buttons for the panel footer.
export function showPosting({ title, company, location, url, text, load, actions = [] }) {
  const body = h('div', { class: 'panel-scroll', tabindex: '-1', autofocus: true }, text ? renderPosting(text) : spinner('Loading the posting...'));
  const close = () => { dlg.close(); };
  const dlg = h('dialog', { class: 'panel', 'aria-label': `${title} posting` },
    h('header', { class: 'panel-head' },
      h('div', {}, h('h2', {}, title), h('div', { class: 'muted' }, [company, location].filter(Boolean).join(' · '))),
      h('button', { class: 'icon-btn ghost', 'aria-label': 'Close', title: 'Close (Esc)', onclick: close }, icon('x', { size: 18 }))),
    body,
    h('footer', { class: 'panel-foot row' }, actions, h('span', { class: 'spacer' }),
      url ? h('a', { href: url, target: '_blank', rel: 'noreferrer', class: 'small row', style: 'gap:5px' }, 'Open in browser', icon('external', { size: 14 })) : null));
  dlg.addEventListener('close', () => dlg.remove());
  dlg.addEventListener('click', (e) => { if (e.target === dlg) close(); }); // click on the backdrop
  document.body.append(dlg);
  dlg.showModal();
  if (!text && load) {
    Promise.resolve(load()).then((r) => { clear(body); body.append(r?.text ? renderPosting(r.text) : empty('This posting has no description.')); })
      .catch((err) => { clear(body); body.append(h('div', { class: 'notice error' }, err.message)); });
  }
  return dlg;
}

const THEMES = [['system', 'monitor', 'System'], ['light', 'sun', 'Light'], ['dark', 'moon', 'Dark']];

// themeSwitch renders a System / Light / Dark control. Every instance stays in sync.
export function themeSwitch({ labels = false } = {}) {
  const el = h('div', { class: `seg ${labels ? 'labeled' : ''}`, role: 'radiogroup', 'aria-label': 'Theme' });
  const draw = (current) => {
    clear(el);
    for (const [id, glyph, label] of THEMES) {
      el.append(h('button', { role: 'radio', 'aria-checked': id === current ? 'true' : 'false', title: label, 'aria-label': label,
        onclick: async () => {
          const r = await window.api.app.setTheme(id);
          if (r.ok) window.dispatchEvent(new CustomEvent('fistbump:theme', { detail: id }));
        } }, icon(glyph), labels ? label : ''));
    }
  };
  window.addEventListener('fistbump:theme', (e) => draw(e.detail));
  window.api.app.getTheme().then((r) => draw(r.ok ? r.data.theme : 'system'));
  draw('system');
  return el;
}

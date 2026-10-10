import { api } from '../../js/api.js';
import { icon } from '../../js/icons.js';
import { h, clear } from '../../js/dom.js';
import { confirmAction, empty, field, modal, run, toast } from '../../js/ui.js';

const STATUSES = ['Saved', 'Applied', 'Interviewing', 'Offered', 'Rejected', 'Archived'];
const COLUMNS = [
  ['company_name', 'Company'], ['position_title', 'Position'], ['status', 'Status'], ['date_applied', 'Applied'],
  ['next_step_date', 'Next step'], ['notes', 'Notes'], ['resume', 'Resume'], ['link', 'Posting'],
];

export async function mount(root, ctx) {
  const page = h('div', { class: 'page full' });
  root.append(page);
  let apps = [];
  let jobsById = new Map();
  let sort = { key: 'updated_at', dir: -1 };
  const search = h('input', { type: 'search', placeholder: 'Search company or position', style: 'width:260px' });
  const statusFilter = h('select', { style: 'width:auto' }, h('option', { value: '' }, 'All statuses'), STATUSES.map((s) => h('option', { value: s }, s)));
  const hideArchived = h('input', { type: 'checkbox', checked: true });
  const wrap = h('div');
  const bulk = h('div');
  const summary = h('span', { class: 'muted small' });
  const picked = new Set(); // selected application ids
  let exportBtn = null;
  for (const el of [search, statusFilter, hideArchived]) el.addEventListener(el === search ? 'input' : 'change', drawTable);

  page.append(
    h('header', { class: 'row between' }, h('h1', {}, 'Tracker'),
      h('div', { class: 'row' }, (exportBtn = h('button', { onclick: exportCsv, title: 'Exports the rows shown, after search and filters' }, 'Export CSV')), h('button', { class: 'primary', onclick: addRow }, icon('plus'), 'Add application'))),
    h('div', { class: 'row', style: 'margin-bottom:10px' }, search, statusFilter, h('label', { class: 'row small' }, hideArchived, 'Hide archived'), h('span', { class: 'spacer' }), summary),
    bulk, wrap);

  async function load() {
    const [a, jobs] = await Promise.all([api.applications.list(), api.jobs.list({ limit: 500 })]);
    apps = a;
    jobsById = new Map(jobs.map((j) => [j.id, j]));
    drawTable();
  }

  function visible() {
    const q = search.value.trim().toLowerCase();
    return apps.filter((a) => (!statusFilter.value || a.status === statusFilter.value) && (!hideArchived.checked || statusFilter.value === 'Archived' || a.status !== 'Archived')
      && (!q || `${a.company_name} ${a.position_title}`.toLowerCase().includes(q)))
      .sort((x, y) => {
        const k = sort.key;
        const vx = k === 'status' ? STATUSES.indexOf(x.status) : (x[k] ?? '');
        const vy = k === 'status' ? STATUSES.indexOf(y.status) : (y[k] ?? '');
        if (vx === vy) return 0;
        if (vx === '') return 1; // blanks last either way
        if (vy === '') return -1;
        return (vx < vy ? -1 : 1) * sort.dir;
      });
  }

  async function save(a, patch, td) {
    const prev = { ...a };
    Object.assign(a, patch);
    try {
      const next = await api.applications.update(a.id, patch);
      Object.assign(a, next);
      td.classList.remove('saved'); void td.offsetWidth; td.classList.add('saved');
      if ('status' in patch && next.date_applied && !prev.date_applied) drawTable(); // Applied fills today's date
      summarize();
    } catch (err) {
      Object.assign(a, prev);
      toast(err.message, 'error');
      drawTable();
    }
  }

  function summarize() {
    if (exportBtn) { const n = visible().length; exportBtn.textContent = `Export ${n} ${n === 1 ? 'row' : 'rows'} to CSV`; }
    const counts = STATUSES.map((s) => `${apps.filter((a) => a.status === s).length} ${s.toLowerCase()}`);
    summary.textContent = counts.join(' · ');
  }

  function drawBulk() {
    clear(bulk);
    for (const id of [...picked]) if (!apps.some((a) => a.id === id)) picked.delete(id);
    if (!picked.size) return;
    const to = h('select', { style: 'width:auto' }, STATUSES.map((s) => h('option', { value: s }, s)));
    const setStatus = async (status, button) => {
      const ids = [...picked];
      const out = await run(() => api.applications.bulkStatus(ids, status), { button });
      if (!out) return;
      for (const u of out) Object.assign(apps.find((a) => a.id === u.id) || {}, u);
      picked.clear();
      toast(`${out.length} ${out.length === 1 ? 'application' : 'applications'} set to ${status}`);
      drawTable();
    };
    bulk.append(h('div', { class: 'bulkbar' }, h('strong', {}, `${picked.size} selected`),
      h('button', { class: 'primary', onclick: (e) => setStatus('Archived', e.target) }, 'Archive'),
      h('span', { class: 'small' }, 'or set status'), to, h('button', { onclick: (e) => setStatus(to.value, e.target) }, 'Apply'),
      h('span', { class: 'spacer' }), h('button', { class: 'link', onclick: () => { picked.clear(); drawTable(); } }, 'Clear selection')));
  }

  function drawTable() {
    clear(wrap);
    summarize();
    drawBulk();
    if (!apps.length) {
      wrap.append(h('section', { class: 'card' }, empty('No applications yet. Add one here, or use "Add to tracker" on a saved job.'),
        h('div', { class: 'row', style: 'justify-content:center' }, h('button', { class: 'primary', onclick: addRow }, icon('plus'), 'Add application'), h('button', { onclick: () => ctx.navigate('jobs') }, 'Find jobs'))));
      return;
    }
    const rows = visible();
    const all = h('input', { type: 'checkbox', 'aria-label': 'Select all rows', checked: rows.length > 0 && rows.every((a) => picked.has(a.id)) });
    all.addEventListener('change', () => { for (const a of rows) all.checked ? picked.add(a.id) : picked.delete(a.id); drawTable(); });
    const head = h('tr', {}, h('th', { class: 'check' }, all), h('th', { class: 'num' }, '#'), COLUMNS.map(([k, label]) => {
      const sortable = !['resume', 'link'].includes(k);
      const arrow = sort.key === k ? (sort.dir === 1 ? ' ▲' : ' ▼') : '';
      return h('th', { class: sortable ? 'sortable' : '', onclick: sortable ? () => { sort = { key: k, dir: sort.key === k ? -sort.dir : 1 }; drawTable(); } : null,
        'aria-sort': sort.key === k ? (sort.dir === 1 ? 'ascending' : 'descending') : null }, label + arrow);
    }), h('th', {}, ''));
    const tbody = h('tbody');
    rows.forEach((a, i) => tbody.append(row(a, i + 1)));
    wrap.append(h('div', { class: 'sheet-wrap' }, h('table', { class: 'sheet' }, h('thead', {}, head), tbody)));
    if (!rows.length) {
      const archivedOnly = hideArchived.checked && apps.some((a) => a.status === 'Archived') && !search.value.trim() && !statusFilter.value;
      wrap.append(h('div', { class: 'row', style: 'padding:14px 12px' }, h('span', { class: 'muted' }, archivedOnly ? 'Every application is archived.' : 'No applications match these filters.'),
        archivedOnly ? h('button', { class: 'link', onclick: () => { hideArchived.checked = false; drawTable(); } }, 'Show archived') : null));
    }
  }

  function row(a, n) {
    const job = jobsById.get(a.job_id);
    const td = (child) => h('td', {}, child);
    const textCell = (key) => {
      const input = h('input', { type: 'text', value: a[key] || '', 'aria-label': key, title: a[key] || '', placeholder: 'Add a note' });
      const cell = td(input);
      input.addEventListener('change', () => save(a, { [key]: input.value.trim() || null }, cell));
      return cell;
    };
    const dateCell = (key) => {
      const input = h('input', { type: 'date', value: a[key] || '', 'aria-label': key, class: a[key] ? '' : 'empty' });
      const cell = td(input);
      input.addEventListener('change', () => { input.classList.toggle('empty', !input.value); save(a, { [key]: input.value || null }, cell); });
      return cell;
    };
    const status = h('select', { class: `status-${a.status}`, 'aria-label': 'Status' }, STATUSES.map((s) => h('option', { value: s, selected: s === a.status }, s)));
    const statusCell = td(status);
    status.addEventListener('change', () => { status.className = `status-${status.value}`; save(a, { status: status.value }, statusCell); });
    const resumeCell = a.tailored_resume_id
      ? h('button', { class: 'link', title: a.exported_pdf_path || 'Download the tailored resume', onclick: async (e) => {
        const r = await run(() => api.tailored.export({ id: a.tailored_resume_id, format: 'pdf', applicationId: a.id }), { button: e.target });
        if (r?.saved) { toast(`Saved to ${r.path}`); a.exported_pdf_path = r.path; }
      } }, a.exported_pdf_path ? [icon('check', { size: 13 }), 'PDF'] : [icon('download', { size: 13 }), 'PDF'])
      : h('button', { class: 'link', onclick: () => ctx.navigate('jobs', { tab: 'saved', job: a.job_id }) }, 'Tailor');
    const pick = h('input', { type: 'checkbox', 'aria-label': `Select ${a.company_name}`, checked: picked.has(a.id) });
    pick.addEventListener('change', () => { pick.checked ? picked.add(a.id) : picked.delete(a.id); drawBulk(); });
    const archived = a.status === 'Archived';
    return h('tr', { class: archived ? 'is-archived' : '' },
      h('td', { class: 'check' }, pick),
      h('td', { class: 'num' }, n),
      td(h('div', { class: 'cell' }, h('button', { class: 'link', style: 'padding:0;text-align:left', onclick: () => ctx.navigate('jobs', { tab: 'saved', job: a.job_id }) }, a.company_name))),
      td(h('div', { class: 'cell' }, a.position_title)),
      statusCell, dateCell('date_applied'), dateCell('next_step_date'), textCell('notes'),
      td(h('div', { class: 'cell' }, resumeCell)),
      td(h('div', { class: 'cell' }, job?.listing_url ? h('a', { href: job.listing_url, target: '_blank', rel: 'noreferrer' }, 'Open') : h('span', { class: 'muted' }, '—'))),
      td(h('div', { class: 'cell' },
        h('button', { class: 'icon-btn small', title: archived ? 'Restore to Saved' : 'Archive', 'aria-label': archived ? 'Restore' : 'Archive', onclick: async () => {
          const prevStatus = a.status;
          const next = await run(() => api.applications.update(a.id, { status: archived ? 'Saved' : 'Archived' }));
          if (!next) return;
          Object.assign(a, next);
          toast(archived ? 'Restored' : 'Archived', 'info', archived ? undefined : { label: 'Undo', onClick: async () => { Object.assign(a, await api.applications.update(a.id, { status: prevStatus })); drawTable(); } });
          drawTable();
        } }, icon(archived ? 'restore' : 'archive', { size: 14 })),
        h('button', { class: 'icon-btn small', title: 'Remove from tracker', 'aria-label': 'Remove', onclick: async () => {
        if (!confirmAction(`Remove ${a.position_title} at ${a.company_name} from the tracker? The saved job stays.`)) return;
        if (await run(() => api.applications.remove(a.id)) !== undefined) { apps = apps.filter((x) => x !== a); drawTable(); }
      } }, icon('trash', { size: 14 })))));
  }

  async function addRow() {
    const jobs = [...jobsById.values()];
    const tracked = new Set(apps.map((a) => a.job_id));
    const untracked = jobs.filter((j) => !tracked.has(j.id));
    const pick = h('select', {}, untracked.map((j) => h('option', { value: j.id }, `${j.position_title} at ${j.company_name}`)), h('option', { value: 'new' }, 'A job I have not saved yet...'));
    const company = h('input', { type: 'text' });
    const title = h('input', { type: 'text' });
    const newBox = h('div', { class: 'form-grid' }, h('div', {}, field('Company', company)), h('div', {}, field('Position', title)));
    const status = h('select', {}, STATUSES.map((s) => h('option', { value: s, selected: s === 'Applied' }, s)));
    const syncNew = () => { newBox.style.display = pick.value === 'new' ? '' : 'none'; };
    pick.addEventListener('change', syncNew);
    syncNew();
    const dlg = modal('Add application', h('div', { class: 'stack' }, field('Job', pick), newBox, field('Status', status),
      h('div', { class: 'row' }, h('button', { class: 'primary', onclick: async (e) => {
        let jobId = Number(pick.value);
        if (pick.value === 'new') {
          const job = await run(() => api.jobs.create({ source: 'manual', company_name: company.value, position_title: title.value }), { button: e.target });
          if (!job) return;
          jobId = job.id;
        }
        if (await run(() => api.applications.create({ job_id: jobId, status: status.value }), { button: e.target })) { dlg.close(); load(); }
      } }, 'Add'), h('button', { onclick: () => dlg.close() }, 'Cancel'))));
  }

  async function exportCsv() {
    const esc = (v) => { const s = v == null ? '' : String(v); return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s; };
    const header = ['Company', 'Position', 'Status', 'Date applied', 'Next step', 'Notes', 'Posting URL', 'Exported PDF'];
    const lines = [header, ...visible().map((a) => [a.company_name, a.position_title, a.status, a.date_applied, a.next_step_date, a.notes, jobsById.get(a.job_id)?.listing_url, a.exported_pdf_path])];
    const r = await run(() => api.app.saveCsv({ name: 'applications.csv', csv: lines.map((l) => l.map(esc).join(',')).join('\r\n') }));
    if (r?.saved) toast(`Saved to ${r.path}`);
  }

  await load();
}

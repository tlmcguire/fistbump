import { api } from '../../js/api.js';
import { icon } from '../../js/icons.js';
import { h, append, clear, fmtDate, fmtMonth } from '../../js/dom.js';
import { take } from '../../js/bus.js';
import { chips, confirmAction, empty, field, formFor, modal, renderMarkdown, run, spinner, tabs, toast } from '../../js/ui.js';

const MODES = [['', 'No preference'], 'Remote', 'Hybrid', 'On-site', 'Any'];

const profileFields = [
  { name: 'full_name', label: 'Full name', required: true },
  { name: 'email', label: 'Email' },
  { name: 'phone', label: 'Phone' },
  { name: 'home_location', label: 'Location', placeholder: 'City, ST', hint: 'Job search ranks nearby postings higher.' },
  { name: 'links', label: 'Links', type: 'list', wide: true, placeholder: 'linkedin.com/in/you, github.com/you, yoursite.com', hint: 'Shown on your resume\'s contact line, as written.' },
  { name: 'target_positions', label: 'Job titles you want', type: 'list', placeholder: 'Backend Engineer, Platform Engineer', hint: 'Comma-separated. Used to search for jobs.' },
  { name: 'preferred_mode', label: 'Preferred work mode', type: 'select', options: MODES },
  { name: 'summary', label: 'Professional summary', type: 'textarea', wide: true, hint: '2-3 sentences at the top of your resume.' },
  { name: 'min_desired_pay', label: 'Minimum desired pay', type: 'int' },
  { name: 'skills', label: 'Other skills', type: 'list', wide: true, hint: 'Comma-separated. Skills you have that are not tied to one role. Role skills are set on each experience.' },
  { name: 'clearance_certs', label: 'Certifications and clearances', type: 'list', hint: 'Comma-separated. Count as skills when matching jobs.' },
];

const experienceFields = [
  { name: 'job_title', label: 'Job title', required: true },
  { name: 'company_name', label: 'Company', required: true },
  { name: 'start_date', label: 'Start', type: 'date', placeholder: 'YYYY-MM' },
  { name: 'end_date', label: 'End', type: 'date', placeholder: 'Blank if current' },
  { name: 'location', label: 'Location', wide: true },
  { name: 'description', label: 'What you did', type: 'textarea', wide: true, hint: 'One point per line. Start lines with "- " for bullets.' },
  { name: 'impact', label: 'Results', type: 'textarea', wide: true, hint: 'Numbers help: "Cut costs 20%", "Served 2M users".' },
  { name: 'skills', label: 'Skills used', type: 'list', wide: true, hint: 'Comma-separated. These are matched against job postings.' },
];

const educationFields = [
  { name: 'institution', label: 'School', required: true },
  { name: 'degree_level', label: 'Degree', placeholder: 'BS, MS, ...' },
  { name: 'discipline', label: 'Field of study' },
  { name: 'graduation_date', label: 'Graduated', type: 'date', placeholder: 'YYYY-MM' },
  { name: 'gpa', label: 'GPA', type: 'float' },
  { name: 'honors', label: 'Honors' },
  { name: 'details', label: 'Details', type: 'textarea', wide: true },
];

// remoteNote tells the user when an AI action would send their resume to a remote provider.
const remoteNote = (status) => (status?.active_engine === 'remote'
  ? ` This sends your resume to ${(() => { try { return new URL(status.remote.base_url).host; } catch { return 'your remote provider'; } })()}.` : '');

export async function mount(root, ctx) {
  const page = h('div', { class: 'page' });
  root.append(page);
  let state = null;
  let tab = ctx.params.tab || 'edit';

  async function load() {
    state = await api.resume.get();
    draw();
  }

  const isEmpty = () => !state.profile && !state.experiences.length;

  function draw() {
    clear(page);
    page.append(h('header', { class: 'row between' },
      h('h1', {}, 'Resume'),
      isEmpty() ? null : h('div', { class: 'row' }, h('button', { onclick: () => startImport() }, icon('upload'), 'Import'), h('button', { onclick: pasteDialog }, icon('clipboard'), 'Paste text'))));
    if (isEmpty()) {
      page.append(startCard());
      return;
    }
    page.append(tabs([['edit', 'Edit'], ['preview', 'Preview & export'], ['saved', 'Saved versions']], tab, (t) => { tab = t; draw(); }));
    const body = h('div', { class: 'stack' });
    page.append(body);
    if (tab === 'edit') body.append(profileSection(), listSection('Experience', state.experiences, experienceFields, 'experience'), listSection('Education', state.education, educationFields, 'education'), skillsCard());
    if (tab === 'preview') previewSection(body);
    if (tab === 'saved') body.append(savedSection());
  }

  function startCard() {
    const choice = (glyph, title, text, onclick, primary) => h('button', { class: `choice ${primary ? 'selected' : ''}`, onclick }, icon(glyph, { size: 22 }), h('strong', {}, title), h('span', { class: 'muted' }, text));
    return h('section', { class: 'stack' },
      h('div', { class: 'choice-grid' },
        choice('upload', 'Import a file', 'PDF, text or Markdown. You can also drop it on the window.', () => startImport(), true),
        choice('clipboard', 'Paste text', 'From any document or site.', pasteDialog),
        choice('pencil', 'Start from scratch', 'Fill in your details and export a PDF.', () => { state.profile = null; buildFromScratch(); })),
      h('p', { class: 'hint' }, 'Stored on this computer only.'));
  }

  function buildFromScratch() {
    clear(page);
    page.append(h('header', {}, h('h1', {}, 'New resume'), h('span', { class: 'muted' }, 'Step 1 of 3: about you')),
      h('section', { class: 'card' }, formFor(profileFields.filter((f) => f.name !== 'summary' && f.name !== 'min_desired_pay'), {}, {
        submitLabel: 'Continue',
        onCancel: () => draw(),
        onSubmit: async (v) => {
          await api.resume.saveProfile(v);
          toast('Saved. Now add your most recent job.');
          tab = 'edit';
          await load();
          page.querySelector('[data-add="experience"]')?.click();
        },
      })));
  }

  // ---- import ----
  async function startImport(engine) {
    const out = await run(() => api.resume.importFile(engine));
    if (out) review(out);
  }

  function pasteDialog() {
    const ta = h('textarea', { style: 'min-height:260px', placeholder: 'Paste your resume text here' });
    const dlg = modal('Paste your resume', h('div', { class: 'stack' }, ta, h('div', { class: 'row' },
      h('button', { class: 'primary', onclick: async (e) => {
        if (!ta.value.trim()) return;
        const out = await run(() => api.resume.importText(ta.value), { button: e.target });
        if (out) { dlg.close(); review(out); }
      } }, 'Read resume'), h('button', { onclick: () => dlg.close() }, 'Cancel'))));
    ta.focus();
  }

  async function review(out) {
    const aiStatus = await api.ai.status().catch(() => null);
    const d = out.draft;
    clear(page);
    const profileForm = formFor(profileFields, d.profile, { submitLabel: null });
    const expForms = [];
    const eduForms = [];
    const entryCard = (fields, value, list, label) => {
      const form = formFor(fields, value, { submitLabel: null });
      const card = h('section', { class: 'card' }, h('div', { class: 'row between' }, h('strong', {}, label),
        h('button', { class: 'danger', type: 'button', onclick: () => { list.splice(list.indexOf(form), 1); card.remove(); } }, 'Remove')), form);
      list.push(form);
      return card;
    };
    const replace = h('input', { type: 'checkbox', checked: state.experiences.length > 0 });
    const keepCopy = h('input', { type: 'checkbox', checked: true });
    const err = h('div', { class: 'error-text' });
    const canAI = aiStatus && aiStatus.active_engine !== 'rules' && out.engine === 'rules';
    append(page, [
      h('header', {}, h('h1', {}, 'Review import'), h('span', { class: 'muted' }, out.filename || 'Pasted text')),
      h('p', { class: 'muted' }, 'Correct anything that was read wrong. Nothing is saved until you choose Save.'),
      out.notice ? h('div', { class: 'notice' }, out.notice) : null,
      d.warnings.length ? h('div', { class: 'notice' }, h('ul', { style: 'margin:0;padding-left:18px' }, d.warnings.map((w) => h('li', {}, w)))) : null,
      canAI ? h('div', { class: 'notice info row between' }, h('span', {}, `Fields look off? Your AI model can read the resume instead.${remoteNote(aiStatus)}`),
        h('button', { onclick: async (e) => {
          e.target.replaceWith(spinner('Reading with AI, this can take a minute...'));
          const again = await run(() => api.resume.importText(out.text, 'auto'));
          if (again) review({ ...again, filename: out.filename }); else review(out);
        } }, 'Read with AI')) : null,
      h('section', { class: 'card stack' }, h('h2', {}, 'About you'), profileForm),
      h('h2', { style: 'margin-top:18px' }, `Experience (${d.experiences.length})`),
      d.experiences.map((x) => entryCard(experienceFields, x, expForms, `${x.job_title} · ${x.company_name}`)),
      h('h2', { style: 'margin-top:18px' }, `Education (${d.education.length})`),
      d.education.map((x) => entryCard(educationFields, x, eduForms, x.institution)),
      h('section', { class: 'card stack', style: 'margin-top:18px' }, h('h2', {}, 'Skills found'), d.skills.length ? chips(d.skills) : h('p', { class: 'muted' }, 'None found.'),
        h('p', { class: 'hint' }, 'Skills mentioned in a role are stored on that role. The rest are under Other skills in About you.')),
      h('section', { class: 'card stack' },
        state.experiences.length || state.education.length ? h('label', { class: 'row' }, replace, 'Replace my current experience and education') : null,
        h('label', { class: 'row' }, keepCopy, 'Also keep the original text as a saved version'), err,
        h('div', { class: 'row' }, h('button', { class: 'primary', onclick: async (e) => {
          err.textContent = '';
          const body = { draft: { profile: profileForm.read(), experiences: expForms.map((f) => f.read()), education: eduForms.map((f) => f.read()) }, replace: replace.checked };
          if (keepCopy.checked) body.save_copy = { version_label: out.filename ? `Imported: ${out.filename}` : 'Imported resume', content: out.text };
          if (replace.checked && state.experiences.length && !confirmAction('Replace your current experience and education entries?')) return;
          e.target.disabled = true;
          try {
            await api.resume.applyImport(body);
            toast('Resume saved');
            tab = 'edit';
            await load();
          } catch (ex) {
            const i = ex.details?.experience_index ?? ex.details?.education_index;
            err.textContent = i !== undefined ? `${ex.details.experience_index !== undefined ? 'Experience' : 'Education'} entry ${i + 1}: ${ex.message}` : ex.message;
            e.target.disabled = false;
          }
        } }, 'Save to my resume'), h('button', { onclick: () => draw() }, 'Cancel')))]);
  }

  // ---- edit ----
  function profileSection() {
    const card = h('section', { class: 'card stack' }, h('h2', {}, 'About you'));
    const form = formFor(profileFields, state.profile || {}, {
      submitLabel: 'Save',
      onSubmit: async (v) => { await api.resume.saveProfile(v); toast('Saved'); await load(); },
    });
    const summaryInput = form.inputs.summary;
    const writeBtn = h('button', { type: 'button', class: 'link', disabled: !state.experiences.length, title: state.experiences.length ? '' : 'Add an experience entry first',
      onclick: async (e) => {
        const st = await api.ai.status().catch(() => null);
        if (st?.active_engine === 'remote' && !confirmAction(`Write a summary with your remote AI provider?${remoteNote(st)}`)) return;
        const r = await run(() => api.resume.summary(), { button: e.target });
        if (r) { summaryInput.value = r.summary; toast(r.engine === 'rules' ? 'Draft written from your resume. Edit it, then Save.' : 'Draft written by your AI model. Edit it, then Save.'); }
      } }, 'Write it for me');
    summaryInput.parentElement.after(writeBtn);
    card.append(form);
    return card;
  }

  function listSection(title, items, fields, kind) {
    const card = h('section', { class: 'card' });
    const body = h('div');
    const ops = kind === 'experience'
      ? { add: api.resume.addExperience, update: api.resume.updateExperience, del: api.resume.deleteExperience }
      : { add: api.resume.addEducation, update: api.resume.updateEducation, del: api.resume.deleteEducation };
    card.append(h('div', { class: 'row between' }, h('h2', {}, title),
      h('button', { 'data-add': kind, disabled: !state.profile, title: state.profile ? '' : 'Save the About you section first', onclick: () => openForm(null) }, `Add ${kind}`)), body);

    function openForm(item, slot) {
      const target = slot || h('div', { class: 'card' });
      clear(target);
      target.append(formFor(fields, item || {}, {
        submitLabel: item ? 'Save' : 'Add',
        onCancel: () => (item ? draw() : target.remove()),
        onSubmit: async (v) => {
          if (item) v.sort_order = item.sort_order;
          await (item ? ops.update(item.id, v) : ops.add(v));
          toast('Saved');
          await load();
        },
      }));
      if (!slot) body.prepend(target);
      target.querySelector('input,textarea')?.focus();
    }

    // Order on the resume follows sort_order; moving swaps it with the neighbor.
    async function move(i, delta) {
      const a = items[i];
      const b = items[i + delta];
      if (!b) return;
      const order = items.map((x, n) => x.sort_order ?? n);
      const strip = ({ id, profile_id, ...rest }) => rest;
      await run(async () => {
        await ops.update(a.id, { ...strip(a), sort_order: order[i + delta] === order[i] ? order[i] + delta : order[i + delta] });
        await ops.update(b.id, { ...strip(b), sort_order: order[i] });
      });
      await load();
    }

    if (!items.length) body.append(empty(`No ${kind} yet.`));
    items.forEach((it, i) => {
      const row = h('div', { class: 'list-item' });
      row.append(h('div', { class: 'grow' }, kind === 'experience'
        ? [h('strong', {}, `${it.job_title}, ${it.company_name}`), h('div', { class: 'muted small' }, [fmtMonth(it.start_date), it.end_date ? fmtMonth(it.end_date) : 'Present'].filter(Boolean).join(' – '), it.location ? ` · ${it.location}` : ''),
          it.description ? h('p', { style: 'white-space:pre-line' }, it.description) : null, it.impact ? h('p', { style: 'white-space:pre-line' }, it.impact) : null, it.skills.length ? chips(it.skills) : null]
        : [h('strong', {}, [it.degree_level, it.discipline].filter(Boolean).join(' in ') || it.institution), h('div', { class: 'muted small' }, it.institution, it.graduation_date ? ` · ${fmtMonth(it.graduation_date)}` : '', it.gpa != null ? ` · GPA ${it.gpa}` : ''),
          it.honors ? h('div', { class: 'small' }, it.honors) : null, it.details ? h('p', {}, it.details) : null]),
      h('div', { class: 'row row-actions' },
        h('button', { class: 'icon-btn small', title: 'Move up', 'aria-label': 'Move up', disabled: i === 0, onclick: () => move(i, -1) }, icon('up', { size: 14 })),
        h('button', { class: 'icon-btn small', title: 'Move down', 'aria-label': 'Move down', disabled: i === items.length - 1, onclick: () => move(i, 1) }, icon('down', { size: 14 })),
        h('button', { onclick: () => openForm(it, row) }, 'Edit'),
        h('button', { class: 'danger', onclick: async () => {
          if (!confirmAction(`Delete this ${kind} entry?`)) return;
          if (await run(() => ops.del(it.id)) !== undefined) await load();
        } }, 'Delete')));
      body.append(row);
    });
    return card;
  }

  function skillsCard() {
    return h('section', { class: 'card' }, h('h2', {}, 'Skills'),
      state.skills.length ? chips(state.skills) : h('p', { class: 'muted' }, 'Add skills to your experience entries and they appear here.'));
  }

  // ---- preview and export ----
  async function previewSection(body) {
    body.append(spinner('Rendering...'));
    const pv = await run(() => api.resume.preview());
    clear(body);
    if (!pv) return;
    const exp = (format, label, primary) => h('button', { class: primary ? 'primary' : '', onclick: async (e) => {
      const r = await run(() => api.resume.export(format), { button: e.target });
      if (r?.saved) toast(`Saved to ${r.path}`);
    } }, label);
    body.append(h('div', { class: 'row' }, exp('pdf', 'Download PDF', true), exp('md', 'Download Markdown'),
      state.profile?.summary ? null : h('span', { class: 'muted small' }, 'Tip: add a summary in Edit. "Write it for me" drafts one.')),
    renderMarkdown(pv.markdown));
  }

  // ---- saved versions ----
  function savedSection() {
    const card = h('section', { class: 'card stack' }, h('h2', {}, 'Saved versions'),
      h('p', { class: 'muted' }, 'Snapshots you can tailor from instead of the master copy.'));
    const list = h('div');
    const label = h('input', { type: 'text', placeholder: 'Name, e.g. "Backend v2"' });
    card.append(h('div', { class: 'row' }, label, h('button', { onclick: async () => {
      if (!label.value.trim()) { toast('Enter a name first', 'error'); return; }
      if (await run(() => api.resumes.create({ version_label: label.value.trim(), from_master: true })) !== undefined) { label.value = ''; toast('Saved'); drawList(); }
    } }, 'Save a snapshot')), list);
    async function drawList() {
      const items = await api.resumes.list();
      clear(list);
      if (!items.length) { list.append(empty('No saved versions.')); return; }
      for (const r of items) {
        const row = h('div', { class: 'list-item' });
        row.append(h('div', { class: 'grow' }, h('strong', {}, r.version_label), h('div', { class: 'muted small' }, `Saved ${fmtDate(r.created_at)}`)),
          h('button', { onclick: () => edit(r.id, row) }, 'View / edit'),
          h('button', { class: 'danger', onclick: async () => {
            if (!confirmAction(`Delete "${r.version_label}"?`)) return;
            if (await run(() => api.resumes.remove(r.id)) !== undefined) drawList();
          } }, 'Delete'));
        list.append(row);
      }
    }
    async function edit(id, row) {
      const full = await run(() => api.resumes.get(id));
      if (!full) return;
      const ta = h('textarea', { value: full.content, style: 'min-height:300px;font-family:ui-monospace,Menlo,monospace' });
      clear(row);
      row.style.display = 'block';
      row.append(h('strong', {}, full.version_label), ta, h('div', { class: 'row' },
        h('button', { class: 'primary', onclick: async () => { if (await run(() => api.resumes.update(id, { content: ta.value })) !== undefined) { toast('Saved'); row.style.display = ''; drawList(); } } }, 'Save'),
        h('button', { onclick: () => { row.style.display = ''; drawList(); } }, 'Close')));
    }
    drawList().catch((e) => toast(e.message, 'error'));
    return card;
  }

  await load().catch((e) => page.append(h('div', { class: 'notice error' }, e.message)));

  // Menu commands and file drops arrive through the bus, possibly before this view was showing.
  const handleRequest = () => {
    const req = take('resume');
    if (req?.action === 'review') review(req.data);
    if (req?.action === 'import') startImport();
  };
  const onRequest = (e) => { if (e.detail === 'resume') handleRequest(); };
  window.addEventListener('fistbump:request', onRequest);
  handleRequest();
  return { onShow: handleRequest, cleanup: () => window.removeEventListener('fistbump:request', onRequest) };
}

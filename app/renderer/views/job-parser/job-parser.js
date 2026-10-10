import { api } from '../../js/api.js';
import { icon } from '../../js/icons.js';
import { h, clear, fmtDate } from '../../js/dom.js';
import { badge, chips, confirmAction, empty, field, formFor, modal, run, showPosting, spinner, tabs, toast } from '../../js/ui.js';

const MODES = [['', 'Unknown'], 'Remote', 'Hybrid', 'On-site'];
let lastSearch = null; // kept across tab switches in this session

// archiveJob archives a saved job. When the job is in the tracker, it asks whether to archive that
// application as well. Returns true when archived.
async function archiveJob(job, apps) {
  const app = (apps || []).find((a) => a.job_id === job.id && a.status !== 'Archived');
  let both = false;
  if (app) {
    both = window.confirm(`Also archive your ${app.status.toLowerCase()} application for ${job.position_title}?\n\nOK archives both. Cancel archives only the saved job.`);
  }
  const r = await run(() => api.jobs.archive(job.id, { archiveApplication: both }));
  if (r) toast(both ? 'Job and application archived' : 'Job archived', 'info', { label: 'Undo', onClick: async () => { await run(() => api.jobs.unarchive(job.id)); window.dispatchEvent(new Event('fistbump:jobs-changed')); } });
  return !!r;
}

export async function mount(root, ctx) {
  const page = h('div', { class: 'page' });
  root.append(page);
  let tab = ctx.params.tab || (ctx.params.job ? 'saved' : 'find');
  let selectedId = ctx.params.job ? Number(ctx.params.job) : null;
  const body = h('div');

  async function draw() {
    const [jobs, archivedJobs] = await Promise.all([api.jobs.list({ limit: 500 }).catch(() => []), api.jobs.list({ limit: 500, archived: 'only' }).catch(() => [])]);
    clear(page);
    page.append(h('header', {}, h('h1', {}, 'Jobs')),
      tabs([['find', 'Find jobs'], ['paste', 'Paste a posting'], ['saved', `Saved jobs (${jobs.length})`]], tab, (t) => { tab = t; showArchivedJobs = false; draw(); }), body);
    clear(body);
    if (tab === 'find') findTab();
    if (tab === 'paste') body.append(pasteCard());
    if (tab === 'saved') savedTab(showArchivedJobs ? archivedJobs : jobs, archivedJobs.length);
  }

  let showArchivedJobs = false;
  const openSaved = (id, archived) => { selectedId = id; tab = 'saved'; if (archived) showArchivedJobs = true; draw(); };
  const onJobsChanged = () => { if (tab === 'saved') draw(); };
  window.addEventListener('fistbump:jobs-changed', onJobsChanged);
  // A link to a specific job (from the tracker or home) may point at an archived one: show that list.
  if (selectedId) {
    const requested = await api.jobs.get(selectedId).catch(() => null);
    if (requested?.archived_at) showArchivedJobs = true;
  }

  // ---- find ----
  async function findTab() {
    const [connectors, resume, cats] = await Promise.all([api.connectors.list(), api.resume.get(), api.connectors.categories()]);
    const gh = connectors.find((c) => c.id === 'greenhouse');
    if (!gh?.enabled) {
      body.append(h('section', { class: 'card stack' }, h('p', {}, 'Job search is turned off.'),
        h('button', { class: 'primary', onclick: async () => { if (await run(() => api.connectors.setEnabled('greenhouse', true))) draw(); } }, 'Turn on job search')));
      return;
    }
    const p = resume.profile;
    const defaultKw = (p?.target_positions?.length ? p.target_positions : resume.experiences.slice(0, 1).map((e) => e.job_title)).join(', ');
    const prev = lastSearch?.form;
    const kw = h('input', { type: 'text', value: prev?.kw ?? defaultKw, placeholder: 'Job title, e.g. Data Analyst' });
    const loc = h('input', { type: 'text', value: prev?.loc ?? '', placeholder: 'Any location' });
    const mode = h('select', {}, [['', 'Any'], 'Remote', 'Hybrid', 'On-site'].map((m) => { const [v, l] = Array.isArray(m) ? m : [m, m];
      const def = prev ? prev.mode : (p?.preferred_mode && p.preferred_mode !== 'Any' ? p.preferred_mode : '');
      return h('option', { value: v, selected: v === def }, l); }));
    const industry = h('select', {}, h('option', { value: '' }, 'All industries'),
      cats.categories.map((c) => h('option', { value: c.id, selected: prev?.industry === c.id }, `${c.parent ? '\u00a0\u00a0\u00a0↳ ' : ''}${c.name}`)));
    const out = h('div', { class: 'stack' });
    const go = async (e) => {
      if (!kw.value.trim()) { toast('Enter a job title to search for', 'error'); kw.focus(); return; }
      clear(out);
      out.append(h('div', { class: 'card' }, spinner('Searching company job boards. The first search takes a few seconds; later ones are instant.')));
      const query = { keywords: kw.value.split(',').map((s) => s.trim()).filter(Boolean), location: loc.value.trim(), work_mode: mode.value || 'Any' };
      if (industry.value) query.categories = [industry.value];
      const res = await run(() => api.connectors.fetch('greenhouse', query), { button: e?.target });
      clear(out);
      if (!res) return;
      lastSearch = { form: { kw: kw.value, loc: loc.value, mode: mode.value, industry: industry.value }, res };
      showResults(out, res);
    };
    kw.addEventListener('keydown', (e) => { if (e.key === 'Enter') go(); });
    loc.addEventListener('keydown', (e) => { if (e.key === 'Enter') go(); });
    body.append(h('section', { class: 'card stack' },
      h('div', { class: 'toolbar' }, h('label', { class: 'field grow' }, h('span', {}, 'Job title'), kw), field('Location', loc), field('Work mode', mode), field('Industry', industry),
        h('button', { class: 'primary', onclick: go }, 'Search')),
      h('p', { class: 'hint' }, `Searches hundreds of company job boards and ranks results by fit with your resume${p?.home_location ? ` and nearness to ${p.home_location}` : ''}. `,
        h('span', { title: cats.notice, style: 'text-decoration:underline dotted' }, 'Which companies?'))), out);
    if (lastSearch) showResults(out, lastSearch.res);
    else if (kw.value) go();
  }

  function showResults(out, res) {
    const showArchived = h('input', { type: 'checkbox', checked: !!lastSearch?.showArchived });
    showArchived.addEventListener('change', () => { if (lastSearch) lastSearch.showArchived = showArchived.checked; clear(out); showResults(out, res); });
    const archivedCount = res.postings.filter((p) => p.archived).length;
    const head = h('div', { class: 'row between' }, h('div', { class: 'row' }, h('strong', {}, `${res.matched} matching jobs`),
      archivedCount ? h('label', { class: 'row small muted' }, showArchived, `Show ${archivedCount} archived`) : null),
      h('span', { class: 'muted small' }, `${res.postings_scanned.toLocaleString()} postings at ${res.boards_searched} companies scanned`));
    out.append(head);
    if (res.errors.length) out.append(h('details', { class: 'small muted' }, h('summary', {}, `${res.errors.length} companies could not be searched`),
      h('ul', {}, res.errors.map((e) => h('li', {}, `${e.board}: ${e.message}`)))));
    if (!res.postings.length) {
      out.append(h('div', { class: 'card' }, empty('No matches. Try a broader title (for example "Engineer"), a different work mode, or All industries.')));
      return;
    }
    const list = h('section', { class: 'card' });
    for (const p of res.postings.filter((x) => showArchived.checked || !x.archived)) {
      const pct = Math.round(p.score * 100);
      const saveJob = async (button) => {
        const r = await run(() => api.connectors.import('greenhouse', null, [p.external_id]), { button });
        if (!r) return null;
        const job = r.imported[0] || r.refreshed[0];
        save.replaceWith(h('button', { class: 'primary', onclick: () => openSaved(job.id) }, 'Saved: view match'));
        return job;
      };
      const save = p.saved_job_id
        ? h('button', { class: p.archived ? '' : 'primary', onclick: () => openSaved(p.saved_job_id, p.archived) }, p.archived ? 'Archived: view' : 'Saved: view match')
        : h('button', { onclick: (e) => saveJob(e.target) }, 'Save');
      const view = () => {
        const dlg = showPosting({
          title: p.position_title, company: p.company_name, location: p.location, url: p.listing_url,
          text: p.raw_text || '', load: p.raw_text ? null : () => api.connectors.posting(p.external_id).then((x) => ({ text: x.raw_text })),
          actions: [h('button', { class: 'primary', onclick: async (e) => { const job = await saveJob(e.target); if (job) { dlg.close(); openSaved(job.id); } } }, 'Save and see my match')],
        });
      };
      list.append(h('div', { class: 'result' },
        h('span', { class: `score ${pct >= 50 ? 'hi' : pct >= 30 ? 'mid' : 'lo'}`, title: `Title match ${Math.round(Math.min(p.title_score, 1) * 100)}%, skills match ${Math.round(p.skill_score * 100)}%` }, `${pct}%`),
        h('div', {}, h('button', { class: 'link title', style: 'padding:0;text-align:left;font-size:inherit', onclick: view }, p.position_title),
          h('div', { class: 'row small muted', style: 'gap:6px' }, [p.company_name, p.location || 'Location not listed', p.updated_at ? `Updated ${fmtDate(p.updated_at)}` : null].filter(Boolean).join(' · '),
            p.archived ? badge('Archived', 'archived') : p.saved_job_id ? badge('Saved', 'good') : null, p.work_mode ? badge(p.work_mode) : null)),
        h('div', { class: 'row', style: 'gap:6px' }, h('button', { class: 'icon-btn ghost', title: 'Read the posting', 'aria-label': 'Read the posting', onclick: view }, icon('eye')), save)));
    }
    out.append(list);
    if (res.matched > res.postings.length) out.append(h('p', { class: 'hint' }, `Showing the best ${res.postings.length}. Add a location or a more specific title to narrow it down.`));
  }

  // ---- paste ----
  function pasteCard() {
    const text = h('textarea', { placeholder: 'Paste the full job posting here, from any site', style: 'min-height:220px' });
    requestAnimationFrame(() => text.focus());
    const preview = h('div', { class: 'stack' });
    const fields = { company: h('input', { type: 'text' }), title: h('input', { type: 'text' }), location: h('input', { type: 'text' }),
      mode: h('select', {}, MODES.map((m) => { const [v, l] = Array.isArray(m) ? m : [m, m]; return h('option', { value: v }, l); })),
      url: h('input', { type: 'text', placeholder: 'https://...' }) };
    const read = async (e) => {
      if (!text.value.trim()) { toast('Paste a posting first', 'error'); return; }
      const parsed = await run(() => api.jobs.parse(text.value), { button: e.target });
      if (!parsed) return;
      const i = parsed.inferred;
      fields.company.value = i.company_name; fields.title.value = i.position_title; fields.location.value = i.location; fields.mode.value = i.work_mode;
      clear(preview);
      const req = [...parsed.req_tech_skills, ...parsed.req_soft_skills];
      const pref = [...parsed.pref_tech_skills, ...parsed.pref_soft_skills];
      preview.append(h('p', { class: 'muted' }, 'Check the details we found, then save.'),
        h('div', { class: 'form-grid' }, h('div', {}, field('Company *', fields.company)), h('div', {}, field('Job title *', fields.title)),
          h('div', {}, field('Location', fields.location)), h('div', {}, field('Work mode', fields.mode)), h('div', { class: 'wide' }, field('Link to posting', fields.url))),
        h('div', {}, h('h3', {}, 'Required skills'), req.length ? chips(req) : h('span', { class: 'muted small' }, 'None found')),
        h('div', {}, h('h3', {}, 'Nice to have'), pref.length ? chips(pref, 'warn') : h('span', { class: 'muted small' }, 'None found')),
        h('div', { class: 'row' }, h('button', { class: 'primary', onclick: save }, 'Save job and see match')));
    };
    async function save(e) {
      const job = await run(() => api.jobs.create({ source: 'pasted', raw_text: text.value, company_name: fields.company.value.trim() || undefined, position_title: fields.title.value.trim() || undefined,
        location: fields.location.value.trim() || undefined, work_mode: fields.mode.value || undefined, listing_url: fields.url.value.trim() || undefined }), { button: e.target });
      if (job) { toast('Job saved'); openSaved(job.id); }
    }
    return h('section', { class: 'card stack' }, h('p', { class: 'muted' }, 'Works with any job site: LinkedIn, Indeed, company pages, and more.'), text,
      h('div', { class: 'row' }, h('button', { class: 'primary', onclick: read }, 'Read posting'), h('button', { onclick: manualEntry }, 'Just enter a title and company')), preview);
  }

  function manualEntry() {
    const company = h('input', { type: 'text' });
    const title = h('input', { type: 'text' });
    const dlg = modal('Add a job', h('div', { class: 'stack' }, field('Company', company), field('Job title', title),
      h('div', { class: 'row' }, h('button', { class: 'primary', onclick: async () => {
        const job = await run(() => api.jobs.create({ source: 'manual', company_name: company.value, position_title: title.value }));
        if (job) { dlg.close(); openSaved(job.id); }
      } }, 'Add'), h('button', { onclick: () => dlg.close() }, 'Cancel'))));
    company.focus();
  }

  // ---- saved ----
  function savedTab(jobs, archivedCount) {
    const pick = (archived) => () => { showArchivedJobs = archived; selectedId = null; draw(); };
    const toggle = h('div', { class: 'seg labeled', role: 'radiogroup', 'aria-label': 'Show' },
      h('button', { role: 'radio', 'aria-checked': String(!showArchivedJobs), onclick: pick(false) }, 'Active'),
      h('button', { role: 'radio', 'aria-checked': String(showArchivedJobs), onclick: pick(true) }, `Archived (${archivedCount})`));
    body.append(toggle);
    if (showArchivedJobs && !jobs.length) {
      body.append(h('section', { class: 'card', style: 'margin-top:12px' }, empty('No archived jobs.')));
      return;
    }
    if (!jobs.length) {
      body.append(h('section', { class: 'card' }, empty('No saved jobs yet.'), h('div', { class: 'row', style: 'justify-content:center' },
        h('button', { class: 'primary', onclick: () => { tab = 'find'; draw(); } }, 'Find jobs'), h('button', { onclick: () => { tab = 'paste'; draw(); } }, 'Paste a posting'))));
      return;
    }
    if (!selectedId || !jobs.some((j) => j.id === selectedId)) selectedId = jobs[0].id;
    const search = h('input', { type: 'search', placeholder: showArchivedJobs ? 'Filter archived jobs' : 'Filter saved jobs' });
    const listBox = h('div', { class: 'stack' });
    const detailBox = h('div');
    const drawList = () => {
      clear(listBox);
      const q = search.value.trim().toLowerCase();
      for (const j of jobs.filter((x) => !q || `${x.company_name} ${x.position_title}`.toLowerCase().includes(q))) {
        const quick = j.archived_at
          ? h('button', { class: 'icon-btn small', title: 'Restore', 'aria-label': 'Restore', onclick: async (e) => { e.stopPropagation(); if (await run(() => api.jobs.unarchive(j.id))) { toast('Restored'); draw(); } } }, icon('restore', { size: 14 }))
          : h('button', { class: 'icon-btn small', title: 'Archive', 'aria-label': 'Archive', onclick: async (e) => {
            e.stopPropagation();
            if (await archiveJob(j, await api.applications.list().catch(() => []))) { if (selectedId === j.id) selectedId = null; draw(); }
          } }, icon('archive', { size: 14 }));
        listBox.append(h('div', { class: `card clickable ${j.id === selectedId ? 'selected' : ''}`, tabindex: '0', role: 'button',
          onclick: () => { selectedId = j.id; drawList(); drawDetail(); }, onkeydown: (e) => { if (e.key === 'Enter') { selectedId = j.id; drawList(); drawDetail(); } } },
        h('div', { class: 'row between' }, h('strong', {}, j.position_title), quick),
        h('div', { class: 'small muted' }, `${j.company_name}${j.location ? ` · ${j.location}` : ''}`)));
      }
    };
    search.addEventListener('input', drawList);

    async function drawDetail() {
      clear(detailBox);
      detailBox.append(spinner());
      const [job, analysis, apps] = await Promise.all([api.jobs.get(selectedId), api.jobs.analyze(selectedId).catch(() => null), api.applications.list()]);
      clear(detailBox);
      const tracked = apps.find((a) => a.job_id === job.id);
      detailBox.append(h('section', { class: 'card stack' },
        h('div', { class: 'row between' }, h('div', {}, h('h2', { style: 'margin:0' }, job.position_title),
          h('div', { class: 'muted' }, [job.company_name, job.location, job.work_mode].filter(Boolean).join(' · '))),
          h('div', { class: 'row' },
            h('button', { onclick: () => editJob(job) }, 'Edit details'),
            h('button', { onclick: () => showPosting({ title: job.position_title, company: job.company_name, location: job.location, url: job.listing_url, text: job.description || job.raw_text || 'No posting text was saved for this job.' }) }, 'View posting'))),
        [job.pay_min || job.pay_max ? `Pay ${[job.pay_min, job.pay_max].filter(Boolean).map((n) => `$${Number(n).toLocaleString()}`).join(' to ')}` : null,
          job.employment_type, job.close_date ? `Closes ${job.close_date}` : null].some(Boolean)
          ? h('div', { class: 'row small muted' }, [job.pay_min || job.pay_max ? `Pay ${[job.pay_min, job.pay_max].filter(Boolean).map((n) => `$${Number(n).toLocaleString()}`).join(' to ')}` : null,
            job.employment_type, job.close_date ? `Closes ${job.close_date}` : null].filter(Boolean).join(' · ')) : null,
        h('div', { class: 'row' },
          h('button', { class: 'primary', onclick: async (e) => {
            const rev = await run(() => api.revisions.create({ job_id: job.id, engine: 'auto' }), { button: e.target });
            if (!rev) return;
            window.dispatchEvent(new Event('fistbump:revision-started'));
            ctx.navigate('revisions', { job: job.id, revision: rev.id });
          } }, 'Tailor my resume for this job'),
          tracked ? h('button', { onclick: () => ctx.navigate('tracker') }, `In tracker: ${tracked.status}`)
            : h('button', { onclick: async (e) => { if (await run(() => api.applications.create({ job_id: job.id }), { button: e.target })) { toast('Added to tracker'); drawDetail(); } } }, 'Add to tracker'),
          h('span', { class: 'spacer' }),
          job.archived_at
            ? h('button', { onclick: async () => { if (await run(() => api.jobs.unarchive(job.id))) { toast('Restored'); showArchivedJobs = false; draw(); } } }, 'Restore')
            : h('button', { onclick: async () => { if (await archiveJob(job, apps)) { selectedId = null; draw(); } } }, 'Archive'),
          h('button', { class: 'danger', onclick: async () => {
            if (!confirmAction('Delete this job and its tailored drafts? Archive instead to keep them.')) return;
            if (await run(() => api.jobs.remove(job.id)) !== undefined) { selectedId = null; draw(); }
          } }, 'Delete')),
        analysis ? analysisBlock(analysis) : h('div', { class: 'notice' }, 'Add your resume to see how you match.'),
        job.requirements.length ? h('div', {}, h('h3', {}, 'Requirements'), h('ul', { style: 'margin:0;padding-left:18px' }, job.requirements.map((r) => h('li', {}, r)))) : null));
    }
    body.append(h('div', { class: 'grid-2', style: 'margin-top:12px' }, h('div', { class: 'stack' }, search, listBox), detailBox));
    drawList();
    drawDetail();
  }

  // editJob edits the details a posting may not state clearly. Changing the posting text re-reads its
  // skills; the other fields leave them alone.
  function editJob(job) {
    const fields = [
      { name: 'position_title', label: 'Job title', required: true }, { name: 'company_name', label: 'Company', required: true },
      { name: 'location', label: 'Location' }, { name: 'work_mode', label: 'Work mode', type: 'select', options: [['', 'Unknown'], 'Remote', 'Hybrid', 'On-site'] },
      { name: 'employment_type', label: 'Employment type', placeholder: 'Full-time, Internship, ...' }, { name: 'close_date', label: 'Applications close', type: 'date', placeholder: 'YYYY-MM-DD' },
      { name: 'pay_min', label: 'Pay from', type: 'int' }, { name: 'pay_max', label: 'Pay to', type: 'int' },
      { name: 'listing_url', label: 'Link to posting', wide: true },
    ];
    const dlg = modal('Edit job details', formFor(fields, job, {
      submitLabel: 'Save',
      onCancel: () => dlg.close(),
      onSubmit: async (v) => {
        await api.jobs.update(job.id, v);
        dlg.close();
        toast('Job updated');
        draw();
      },
    }));
    dlg.classList.add('wide');
  }

  function analysisBlock(a) {
    const pct = Math.round(a.score * 100);
    const total = ['tech', 'soft'].reduce((n, k) => n + a.required[k].matched.length + a.required[k].missing.length, 0);
    const group = (title, pair, missingCls) => {
      const matched = [...pair.tech.matched, ...pair.soft.matched];
      const missing = [...pair.tech.missing, ...pair.soft.missing];
      if (!matched.length && !missing.length) return null;
      return h('div', {}, h('h3', {}, title),
        h('div', { class: 'chips' }, matched.map((m) => h('span', { class: 'chip good', title: m.experience_ids.length ? `Shown in ${m.experience_ids.length} of your roles` : 'Listed in your skills or certifications' }, icon('check', { size: 12 }), m.skill)),
          missing.map((s) => h('span', { class: `chip ${missingCls}`, title: 'Not on your resume' }, s))));
    };
    // Postings without listed required skills are compared on their nice-to-have skills instead.
    const prefMatched = a.preferred.tech.matched.length + a.preferred.soft.matched.length;
    const prefTotal = prefMatched + a.preferred.tech.missing.length + a.preferred.soft.missing.length;
    const meter = (p, label) => h('div', {}, h('strong', {}, label),
      h('div', { class: `meter ${p >= 70 ? 'good' : p >= 40 ? 'warn' : 'bad'}`, style: 'margin-top:4px' }, h('div', { style: `width:${p}%` })),
      h('p', { class: 'hint' }, p < 100 ? 'Missing a skill you do have? Add it to the matching role on your resume, then tailor.' : 'Strong match. Tailor your resume to lead with these skills.'));
    const prefPct = prefTotal ? Math.round((prefMatched / prefTotal) * 100) : 0;
    return h('div', { class: 'stack' },
      total ? meter(pct, `You have ${pct}% of the required skills`)
        : prefTotal ? meter(prefPct, `No required skills listed. You have ${prefMatched} of ${prefTotal} nice-to-have skills`)
          : h('div', { class: 'notice info' }, 'No specific skills were found in this posting to compare.'),
      group('Required', a.required, 'bad'), group('Nice to have', a.preferred, 'warn'));
  }

  await draw();
  // Returning to the tab keeps searches and pasted text; the saved list refreshes.
  return {
    onShow: () => { if (tab === 'saved') draw(); },
    cleanup: () => window.removeEventListener('fistbump:jobs-changed', onJobsChanged),
  };
}

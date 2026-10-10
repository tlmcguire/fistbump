import { api } from '../../js/api.js';
import { icon } from '../../js/icons.js';
import { h, append, clear, fmtDate } from '../../js/dom.js';
import { badge, confirmAction, empty, field, poll, run, showPosting, spinner, toast } from '../../js/ui.js';
import { renderWords, wordDiff } from '../../js/worddiff.js';

const SECTION = { summary: 'Summary', experience: 'Experience', education: 'Education', skills: 'Skills' };
const ENGINE = { local: 'Local AI', remote: 'Remote AI', rules: 'Rule-based' };
const STATUS = { queued: 'Starting', running: 'Writing', done: 'Ready', failed: 'Failed', canceled: 'Canceled' };
let lastJobId = null; // the job you were working on, kept across visits

const fmtTime = (s) => {
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
};

export async function mount(root, ctx) {
  const page = h('div', { class: 'page' });
  root.append(page);
  const stepper = h('div', { class: 'stepper' });
  const setStep = (n) => {
    clear(stepper);
    ['Review suggested edits', 'Check the tailored resume', 'Download and track'].forEach((t, i) => {
      if (i) stepper.append(h('span', { class: 'sep' }, '→'));
      stepper.append(h('span', { class: i === n ? 'on' : '' }, `${i + 1}. ${t}`));
    });
  };
  const picker = h('section', { class: 'card toolbar' });
  const roundsBar = h('div', { class: 'rounds' });
  const body = h('div', { class: 'stack' });
  page.append(h('header', {}, h('h1', {}, 'Tailor')),
  stepper, picker, roundsBar, body);
  setStep(0);

  let jobs = await api.jobs.list({ limit: 200 }).catch(() => []);
  if (!jobs.length) {
    page.append(h('section', { class: 'card' }, empty('Save a job first, then tailor your resume for it.'),
      h('div', { class: 'row', style: 'justify-content:center' }, h('button', { class: 'primary', onclick: () => ctx.navigate('jobs') }, 'Find jobs'))));
    // Rebuild once jobs exist, so returning to the tab is not stuck on this message.
    return { onShow: async () => { if ((await api.jobs.list({ limit: 1 }).catch(() => [])).length) { clear(root); mount(root, ctx); } } };
  }
  let jobId = ctx.params.job ? Number(ctx.params.job) : lastJobId;
  let revId = ctx.params.revision ? Number(ctx.params.revision) : null;
  if (!jobId) { // default to the job of the most recent round
    const recent = await api.revisions.list().catch(() => []);
    jobId = recent[0]?.job_id || null;
  }
  if (jobId && !jobs.some((j) => j.id === jobId)) jobId = null;
  let stopPoll = null;
  let rounds = [];
  let drafts = [];
  let view = 'round'; // or 'tailored'
  let tailoredId = null;

  const jobSelect = h('select', { 'aria-label': 'Job' });
  const fillJobs = () => {
    clear(jobSelect);
    append(jobSelect, [h('option', { value: '' }, 'Choose a job'), jobs.map((j) => h('option', { value: j.id }, `${j.position_title} at ${j.company_name}`))]);
    jobSelect.value = jobId ? String(jobId) : ''; // set after the options exist, or the choice is lost
  };
  fillJobs();
  jobSelect.addEventListener('change', () => { selectJob(jobSelect.value ? Number(jobSelect.value) : null); });
  const newRound = h('button', { class: 'primary', onclick: (e) => startRound(e.target) }, 'Suggest edits');
  const viewPosting = h('button', { onclick: async () => {
    const j = await run(() => api.jobs.get(jobId));
    if (j) showPosting({ title: j.position_title, company: j.company_name, location: j.location, url: j.listing_url, text: j.description || j.raw_text || '' });
  } }, 'View posting');
  // Start from the master resume or a saved version (resumes table). Rounds remember their base.
  const versions = await api.resumes.list().catch(() => []);
  const baseSelect = h('select', { 'aria-label': 'Start from' }, h('option', { value: '' }, 'Master resume'),
    versions.map((v) => h('option', { value: v.id }, v.version_label)));
  append(picker, [h('label', { class: 'field grow' }, h('span', {}, 'Job'), jobSelect),
    versions.length ? h('label', { class: 'field' }, h('span', {}, 'Start from'), baseSelect) : null, viewPosting, newRound]);
  const baseName = (id) => (id ? versions.find((v) => v.id === id)?.version_label || 'a saved version' : 'master resume');

  async function startRound(button) {
    if (!jobId) { toast('Choose a job first', 'error'); return; }
    const body = { job_id: jobId, engine: 'auto' };
    if (baseSelect.value) body.base_resume_id = Number(baseSelect.value);
    const rev = await run(() => api.revisions.create(body), { button });
    if (!rev) return;
    window.dispatchEvent(new Event('fistbump:revision-started'));
    revId = rev.id;
    view = 'round';
    await loadRounds();
    await drawBody();
  }

  function selectJob(id) {
    jobId = id;
    lastJobId = id;
    revId = null;
    view = 'round';
    stopPolling();
    loadRounds().then(drawBody);
  }

  function stopPolling() { if (stopPoll) { stopPoll(); stopPoll = null; } }

  // Rounds are numbered per job in the order they were created.
  async function loadRounds() {
    viewPosting.disabled = !jobId;
    newRound.disabled = !jobId;
    if (!jobId) { rounds = []; drafts = []; drawRounds(); return; }
    lastJobId = jobId;
    const [revs, tl] = await Promise.all([api.revisions.list(jobId).catch(() => []), api.tailored.list(jobId).catch(() => [])]);
    rounds = revs.slice().sort((a, b) => a.id - b.id).map((r, i) => ({ ...r, n: i + 1 }));
    drafts = tl;
    if (!revId || !rounds.some((r) => r.id === revId)) revId = rounds.length ? rounds[rounds.length - 1].id : null;
    newRound.textContent = rounds.length ? 'Suggest edits again' : 'Suggest edits';
    drawRounds();
  }

  function drawRounds() {
    clear(roundsBar);
    for (const r of rounds.slice().reverse()) {
      const draft = drafts.find((t) => t.revision_id === r.id);
      const busy = r.status === 'queued' || r.status === 'running';
      roundsBar.append(h('button', { class: 'card round', 'aria-pressed': r.id === revId && view === 'round' ? 'true' : 'false',
        onclick: () => { revId = r.id; view = 'round'; drawRounds(); drawBody(); } },
      h('strong', {}, `Round ${r.n}`, busy ? h('span', { class: 'spinner', style: 'margin-left:6px' }) : null),
      h('span', { class: 'meta' }, `${fmtTime(r.created_at)} · ${STATUS[r.status] || r.status} · ${ENGINE[r.engine] || r.engine}`),
      versions.length ? h('span', { class: 'meta' }, `From ${baseName(r.base_resume_id)}`) : null,
      draft ? h('span', { class: 'meta draft' }, icon('check', { size: 12 }), 'Tailored resume') : null));
    }
  }

  async function drawBody() {
    stopPolling();
    clear(body);
    if (!jobId) { body.append(h('section', { class: 'card' }, empty('Choose a job to tailor your resume for.'))); return; }
    if (view === 'tailored' && tailoredId) return showTailored(tailoredId);
    setStep(0);
    if (!revId) {
      body.append(h('section', { class: 'card' }, empty('No suggestions for this job yet.'),
        h('div', { class: 'row', style: 'justify-content:center' }, h('button', { class: 'primary', onclick: (e) => startRound(e.target) }, 'Suggest edits'))));
      return;
    }
    const rev = await run(() => api.revisions.get(revId));
    if (!rev || rev.id !== revId) return;
    const round = rounds.find((r) => r.id === rev.id);
    const busy = rev.status === 'queued' || rev.status === 'running';
    const draft = drafts.find((t) => t.revision_id === rev.id);
    body.append(h('section', { class: 'card row between' },
      h('div', { class: 'row' }, h('strong', {}, `Round ${round?.n ?? ''}`), badge(STATUS[rev.status] || rev.status, { done: 'good', failed: 'bad', canceled: 'warn' }[rev.status] || ''),
        badge(ENGINE[rev.engine] || rev.engine),
        busy ? spinner(rev.engine === 'local' ? 'Writing suggestions on this computer' : 'Writing suggestions') : null),
      h('div', { class: 'row' },
        draft ? h('button', { class: 'primary', onclick: () => { view = 'tailored'; tailoredId = draft.id; drawRounds(); drawBody(); } }, 'View tailored resume') : null,
        busy ? h('button', { onclick: async () => { await run(() => api.revisions.cancel(rev.id)); await loadRounds(); drawBody(); } }, 'Cancel') : null,
        h('button', { class: 'danger', onclick: async () => {
          if (!confirmAction(`Delete round ${round?.n ?? ''} and its suggestions?`)) return;
          if (await run(() => api.revisions.remove(rev.id)) !== undefined) { revId = null; await loadRounds(); drawBody(); }
        } }, 'Delete'))));
    if (rev.error) body.append(h('div', { class: 'notice error' }, `Could not write suggestions: ${rev.error}`));
    if (busy) {
      // Keeps polling while you are on other tabs; the view stays mounted.
      stopPoll = poll(async () => {
        const cur = await api.revisions.get(rev.id);
        if (cur.status === 'queued' || cur.status === 'running') return false;
        await loadRounds();
        if (revId === rev.id) drawBody();
        return true;
      }, 1200);
      return;
    }
    if (rev.status === 'done') drawSuggestions(rev);
  }

  async function drawSuggestions(rev) {
    const sugs = rev.suggestions || [];
    const pending = sugs.filter((s) => s.state === 'pending').length;
    if (!sugs.length) {
      const job = await api.jobs.get(rev.job_id).catch(() => null);
      const noText = job && !job.description && !job.raw_text;
      const noSkills = job && ![...job.req_tech_skills, ...job.pref_tech_skills, ...job.req_soft_skills, ...job.pref_soft_skills].length;
      body.append(h('section', { class: 'card stack' }, empty(noText
        ? 'This job has no posting text, so there is nothing to tailor against. Paste the posting on the Jobs page, then try again.'
        : noSkills ? 'No skills were found in this posting to tailor toward. Edit the job or paste a fuller posting.'
          : 'No suggested edits this round. Add more detail or skills to your resume, then try again.'),
      noText || noSkills ? h('div', { class: 'row', style: 'justify-content:center' }, h('button', { onclick: () => ctx.navigate('jobs', { tab: 'paste' }) }, 'Paste a posting')) : null));
      return;
    }
    for (const s of sugs) body.append(suggestionCard(rev, s));
    body.append(h('p', { class: 'hint' }, rev.engine === 'rules'
      ? 'Rule-based: reorders and highlights what your resume already says. Add a local model in Settings for rewrites.'
      : 'Suggestions that name skills or claims not on your resume are discarded before you see them.'));
    const create = h('button', { class: 'primary', disabled: pending > 0, onclick: async (e) => {
      const t = await run(() => api.tailored.create(rev.id), { button: e.target });
      if (t) { toast('Tailored resume built'); await loadRounds(); view = 'tailored'; tailoredId = t.id; drawRounds(); drawBody(); }
    } }, pending ? `Review ${pending} more to continue` : 'Build tailored resume');
    body.append(h('section', { class: 'card row between' }, h('span', { class: 'muted' }, `${sugs.length - pending} of ${sugs.length} reviewed`),
      h('div', { class: 'row' }, pending ? h('button', { onclick: async () => {
        for (const s of sugs.filter((x) => x.state === 'pending')) await run(() => api.revisions.accept(rev.id, s.id));
        drawBody();
      } }, 'Accept all remaining') : null, create)));
  }

  function suggestionCard(rev, s) {
    const card = h('section', { class: 'card stack' });
    const stateKind = { accepted: 'good', rejected: 'bad', edited: 'warn' }[s.state] || '';
    const draw = () => {
      const final = s.state === 'edited' ? s.edited_text : s.proposed_text;
      const words = wordDiff(s.original_text, final);
      clear(card);
      card.append(h('div', { class: 'row between' }, h('strong', {}, SECTION[s.section] || s.section), badge(s.state === 'pending' ? 'to review' : s.state, stateKind)),
        h('div', { class: 'diff-cols' },
          h('div', { class: 'diff-col' }, h('h4', {}, 'Current'), s.original_text ? renderWords(words, 'before') : h('span', { class: 'muted' }, 'Nothing yet (new text)')),
          h('div', { class: 'diff-col' }, h('h4', {}, s.state === 'edited' ? 'Your edit' : 'Suggested'), renderWords(words, 'after'))),
        h('div', { class: 'row' },
          h('button', { class: s.state === 'accepted' ? 'primary' : '', onclick: (e) => act(e, () => api.revisions.accept(rev.id, s.id)) }, s.state === 'accepted' ? 'Accepted' : 'Accept'),
          h('button', { onclick: (e) => act(e, () => api.revisions.reject(rev.id, s.id)) }, s.state === 'rejected' ? 'Rejected' : 'Reject'),
          h('button', { onclick: edit }, 'Edit')));
    };
    async function act(e, fn) {
      const next = await run(fn, { button: e.target });
      if (next) { Object.assign(s, next); drawBody(); }
    }
    function edit() {
      const ta = h('textarea', { value: s.state === 'edited' ? s.edited_text : s.proposed_text });
      clear(card);
      card.append(h('strong', {}, `Edit ${SECTION[s.section] || s.section}`), ta, h('div', { class: 'row' },
        h('button', { class: 'primary', onclick: async (e) => { if (await run(() => api.revisions.edit(rev.id, s.id, ta.value), { button: e.target })) drawBody(); } }, 'Save edit'),
        h('button', { onclick: draw }, 'Cancel')));
      ta.focus();
    }
    draw();
    return card;
  }

  async function showTailored(id) {
    clear(body);
    setStep(1);
    const [t, diff, apps] = await Promise.all([run(() => api.tailored.get(id)), run(() => api.tailored.diff(id)), run(() => api.applications.list())]);
    if (!t || !diff) return;
    const app = (apps || []).find((a) => a.tailored_resume_id === t.id);
    const jobApp = app || (apps || []).find((a) => a.job_id === t.job_id);
    const round = rounds.find((r) => r.id === t.revision_id);
    const hunksBox = h('div');
    const showAll = h('input', { type: 'checkbox' });
    const drawHunks = () => {
      clear(hunksBox);
      for (const hk of diff.hunks) {
        if (hk.status === 'unchanged' && !showAll.checked) {
          const lines = hk.before.split('\n').filter(Boolean);
          if (lines.length) hunksBox.append(h('div', { class: 'hunk unchanged collapsed' }, `${lines.length} unchanged line${lines.length === 1 ? '' : 's'}`));
          continue;
        }
        const label = hk.status === 'unchanged' ? null : h('div', { class: 'small muted' }, `${SECTION[hk.section] || hk.section} · ${hk.status}`);
        const content = hk.status === 'changed' ? h('div', {}, renderWords(hk.words, 'after'), h('div', { class: 'small muted', style: 'margin-top:4px' }, 'was: ', renderWords(hk.words, 'before')))
          : hk.status === 'removed' ? h('del', { class: 'w' }, hk.before) : hk.status === 'added' ? h('ins', { class: 'w' }, hk.after) : hk.after;
        hunksBox.append(h('div', { class: `hunk ${hk.status}` }, label, content));
      }
    };
    showAll.addEventListener('change', drawHunks);
    drawHunks();
    const exportBtn = (format, label) => h('button', { class: format === 'pdf' ? 'primary' : '', onclick: async (e) => {
      const r = await run(() => api.tailored.export({ id: t.id, format, applicationId: (app || jobApp)?.id }), { button: e.target });
      if (r?.saved) { toast(`Saved to ${r.path}`); setStep(2); }
    } }, label);
    body.append(h('section', { class: 'card row between' },
      h('div', {}, h('strong', {}, `Tailored resume${round ? ` from round ${round.n}` : ''}`), h('span', { class: 'muted' }, ` · ${fmtDate(t.created_at)}`)),
      h('div', { class: 'row' }, h('button', { onclick: () => { view = 'round'; drawRounds(); drawBody(); } }, 'Back to suggestions'),
        app ? badge(`In tracker: ${app.status}`, 'good')
          : jobApp ? h('button', { onclick: async (e) => {
            if (await run(() => api.applications.update(jobApp.id, { tailored_resume_id: t.id }), { button: e.target })) { toast('Tracker now uses this resume'); showTailored(t.id); }
          } }, 'Use this resume in the tracker')
            : h('button', { onclick: async (e) => {
              if (await run(() => api.applications.create({ job_id: t.job_id, tailored_resume_id: t.id }), { button: e.target })) { toast('Added to tracker'); showTailored(t.id); }
            } }, 'Add to tracker'),
        exportBtn('md', 'Download Markdown'), exportBtn('pdf', 'Download PDF'),
        h('button', { class: 'danger', onclick: async () => {
          if (!confirmAction('Delete this tailored resume?')) return;
          if (await run(() => api.tailored.remove(t.id)) !== undefined) { view = 'round'; await loadRounds(); drawBody(); }
        } }, 'Delete'))),
    h('section', { class: 'card' }, h('div', { class: 'row between' }, h('h2', {}, 'What changed from your resume'),
      h('label', { class: 'row small' }, showAll, 'Show unchanged text')), hunksBox));
  }

  await loadRounds();
  await drawBody();
  return {
    cleanup: stopPolling,
    // Returning to the tab: pick up jobs saved and rounds started elsewhere, without losing your place.
    onShow: async () => {
      jobs = await api.jobs.list({ limit: 200 }).catch(() => jobs);
      fillJobs();
      const before = JSON.stringify(rounds.map((r) => [r.id, r.status]));
      await loadRounds();
      if (JSON.stringify(rounds.map((r) => [r.id, r.status])) !== before && view === 'round' && !stopPoll) drawBody();
    },
  };
}

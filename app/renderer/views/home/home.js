import { api } from '../../js/api.js';
import { icon } from '../../js/icons.js';
import { h, fmtDate } from '../../js/dom.js';
import { badge, empty } from '../../js/ui.js';

export async function mount(root, ctx) {
  const page = h('div', { class: 'page stack' });
  root.append(page);
  const [resume, jobs, apps, drafts] = await Promise.all([
    api.resume.get(), api.jobs.list({ limit: 200 }), api.applications.list(), api.tailored.list(),
  ]);
  const hasResume = !!resume.profile && resume.experiences.length > 0;
  const tailored = drafts.length; // tailored resumes built, not suggestion rounds
  const steps = [
    { done: hasResume, title: 'Resume', body: hasResume ? `${resume.experiences.length} roles · ${resume.skills.length} skills` : 'Import a PDF or start from scratch.',
      action: hasResume ? ['Edit resume', () => ctx.navigate('resume')] : ['Import or build', () => ctx.navigate('resume')] },
    { done: jobs.length > 0, title: 'Jobs', body: jobs.length ? `${jobs.length} saved` : 'Search company job boards or paste a posting.',
      action: ['Find jobs', () => ctx.navigate('jobs')] },
    { done: tailored > 0, title: 'Tailor', body: tailored ? `${tailored} tailored ${tailored === 1 ? 'resume' : 'resumes'}` : 'Match your resume to a posting.',
      action: ['Tailor', () => ctx.navigate('jobs', jobs.length ? { tab: 'saved' } : {})] },
    { done: apps.length > 0, title: 'Applications', body: apps.length ? `${apps.length} tracked` : 'Dates, status and next steps.',
      action: ['Open tracker', () => ctx.navigate('tracker')] },
  ];
  const current = steps.findIndex((s) => !s.done);
  const name = resume.profile?.full_name?.split(' ')[0];
  page.append(
    h('header', {}, h('h1', {}, 'Overview'), name ? h('span', { class: 'muted' }, resume.profile.full_name) : null),
    h('div', { class: 'steps' }, steps.map((s, i) => h('section', { class: `card step ${s.done ? 'done' : ''} ${i === current ? 'current' : ''}` },
      h('div', { class: 'row' }, h('span', { class: 'num' }, s.done ? icon('check', { size: 13 }) : String(i + 1)), h('strong', {}, s.title)),
      h('p', { class: 'muted small grow' }, s.body),
      h('div', {}, h('button', { class: i === current ? 'primary' : '', onclick: s.action[1] }, s.action[0]))))),
  );

  const counts = Object.fromEntries(['Saved', 'Applied', 'Interviewing', 'Offered', 'Rejected'].map((st) => [st, apps.filter((a) => a.status === st).length]));
  const upcoming = apps.filter((a) => a.next_step_date && a.status !== 'Archived' && a.status !== 'Rejected').sort((a, b) => a.next_step_date.localeCompare(b.next_step_date)).slice(0, 6);
  // Local date, not UTC: toISOString() would mark today's steps overdue in the evening west of UTC.
  const now = new Date();
  const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
  page.append(h('div', { class: 'grid-2' },
    h('section', { class: 'card stack' }, h('h2', {}, 'Pipeline'),
      h('div', { class: 'row', style: 'gap:24px' }, Object.entries(counts).map(([k, v]) => h('div', {}, h('div', { class: 'stat' }, v), h('div', { class: 'muted small' }, k)))),
      h('h3', { style: 'margin-top:8px' }, 'Next steps'),
      upcoming.length ? upcoming.map((a) => h('div', { class: 'list-item' }, h('div', { class: 'grow' }, h('strong', {}, a.position_title), h('div', { class: 'small muted' }, a.company_name)),
        badge(a.next_step_date < today ? `Overdue ${a.next_step_date}` : a.next_step_date, a.next_step_date < today ? 'bad' : 'warn')))
        : h('p', { class: 'muted small' }, 'No upcoming steps.')),
    h('section', { class: 'card stack' }, h('h2', {}, 'Recent jobs'),
      jobs.length ? jobs.slice(0, 6).map((j) => h('div', { class: 'list-item' }, h('div', { class: 'grow' }, h('strong', {}, j.position_title), h('div', { class: 'small muted' }, `${j.company_name} · added ${fmtDate(j.created_at)}`)),
        h('button', { onclick: () => ctx.navigate('jobs', { tab: 'saved', job: j.id }) }, 'Open')))
        : empty('No jobs yet.'))));
}

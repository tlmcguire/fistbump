const { test, expect } = require('./fixtures');

test('opens the app with the expected title and bottom bar', async ({ window }) => {
  await expect(window).toHaveTitle('Fistbump');
  await expect(window.locator('.bottombar #nav-home')).toBeVisible();
  await expect(window.locator('.sidebar')).toHaveCount(0);
});

test('the Go backend answers through the bridge API', async ({ window }) => {
  const info = await window.evaluate(() => window.api.app.info());
  expect(info.version).toMatch(/^\d+\.\d+\.\d+/);
  const connectors = await window.evaluate(() => window.api.connectors.list());
  expect(connectors.error).toBeUndefined();
  const resume = await window.evaluate(() => window.api.resume.get());
  expect(resume.error).toBeUndefined();
});

test('uses a fresh data folder, not the real one', async ({ window }) => {
  const info = await window.evaluate(() => window.api.app.info());
  expect(info.dataDir).toContain('fistbump-e2e-');
});

test('the renderer is isolated from Node', async ({ window }) => {
  const exposed = await window.evaluate(() => ({
    require: typeof require,
    process: typeof process,
    module: typeof module,
    api: typeof window.api,
  }));
  expect(exposed).toEqual({ require: 'undefined', process: 'undefined', module: 'undefined', api: 'object' });
});

test('the page has a Content-Security-Policy that blocks remote scripts', async ({ window }) => {
  const csp = await window.locator('meta[http-equiv="Content-Security-Policy"]').getAttribute('content');
  expect(csp).toContain("default-src 'self'");
  expect(csp).not.toContain('unsafe-eval');
  expect(csp).not.toMatch(/script-src[^;]*unsafe-inline/);
});

test('starts without console errors or page errors', async ({ window }) => {
  const problems = [];
  window.on('pageerror', (e) => problems.push(String(e)));
  window.on('console', (m) => m.type() === 'error' && problems.push(m.text()));
  await window.reload();
  await window.locator('#nav-home').waitFor();
  expect(problems).toEqual([]);
});

test('navigates between the main views', async ({ window }) => {
  await window.locator('#tab-settings').click();
  await expect(window.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible();
  await expect(window.locator('#settings-pane')).toBeVisible();

  await window.evaluate(() => window.Omnibox.route('Data Analyst\nTidewater Logistics\n\nRequirements:\n- SQL'));
  await expect(window.getByRole('heading', { name: 'Paste a job', level: 1 })).toBeVisible();

  await window.locator('#nav-resume').click();
  await expect(window.locator('#settings-pane')).toBeHidden();

  await window.evaluate(() => window.App.openTab('tab-find'));
  await expect(window.locator('#search-form')).toBeVisible();
});

test('master resume saves profile and renders PDF preview without stuck overlay on unmount', async ({ window }) => {
  await window.locator('#nav-resume').click();
  const nameInput = window.locator('input[id$="-full_name"]');
  await nameInput.fill('Ada Lovelace');
  await window.getByRole('button', { name: /Create profile|Save profile/ }).click();
  await expect(window.locator('.resume-doc')).toContainText('Ada Lovelace');

  await window.locator('#resume-tab-pdf').click();
  const preview = window.locator('.pdf-preview');
  await expect(preview).toBeVisible();
  await preview.locator('.pdf-expand').click();
  await expect(window.locator('.pdf-preview.expanded')).toBeVisible();
  await window.keyboard.press('Escape');
  await expect(window.locator('.pdf-preview.expanded')).toHaveCount(0);

  // Expanding again and switching routes programmatically must dispose the overlay cleanly.
  await preview.locator('.pdf-expand').click();
  await expect(window.locator('.pdf-preview.expanded')).toBeVisible();
  await window.evaluate(() => window.App.go('home'));
  await expect(window.locator('.pdf-preview.expanded')).toHaveCount(0);
  await expect(window.locator('.pdf-backdrop')).toHaveCount(0);
});

test('open link page is merged into search bar and detects pasted Greenhouse URLs', async ({ window }) => {
  await window.evaluate(() => window.App.openTab('tab-find'));
  await expect(window.locator('#tab-link')).toHaveCount(0);
  await expect(window.locator('#job-form')).toHaveCount(0);

  const searchInput = window.locator('#search-keywords');
  const submitBtn = window.locator('#search-form button[type="submit"]');
  await expect(submitBtn).toHaveText('Search');

  await searchInput.fill('https://boards.greenhouse.io/acme/jobs/12345');
  await expect(submitBtn).toHaveText('Open link');

  await searchInput.fill('data analyst');
  await expect(submitBtn).toHaveText('Search');
});

test('experience supports date range not applicable and resume preview supports PDF styles', async ({ window }) => {
  await window.locator('#nav-resume').click();
  const nameInput = window.locator('input[id$="-full_name"]');
  await nameInput.fill('Grace Hopper');
  await window.getByRole('button', { name: /Create profile|Save profile/ }).click();
  await expect(window.locator('.resume-doc')).toContainText('Grace Hopper');

  // Switch PDF styles in preview
  const styleSelect = window.locator('#resume-pdf-style');
  await expect(styleSelect).toBeVisible();
  await styleSelect.selectOption('modern');
  await expect(styleSelect).toHaveValue('modern');
  await styleSelect.selectOption('compact');
  await expect(styleSelect).toHaveValue('compact');

  // Add an experience with Date range not applicable checked
  await window.getByRole('button', { name: 'Add experience' }).click();
  await window.locator('input[id$="-job_title"]').fill('Open Source Maintainer');
  await window.locator('input[id$="-company_name"]').fill('Community Project');
  await window.locator('input[id$="-date_na"]').check();
  await window.locator('.entry-form button[type="submit"]').click();

  await expect(window.locator('.resume-doc')).toContainText('Open Source Maintainer');
  await expect(window.locator('.resume-doc')).not.toContainText('to Present');
});

test('cost analysis returns cited sources and 30-day refresh metadata', async ({ window }) => {
  const result = await window.evaluate(() => window.api.cost.analyze({
    title: 'Software Engineer',
    location: 'Austin, TX',
    work_mode: 'Hybrid',
    pay_min: 130000,
    pay_max: 150000,
    text: 'We offer health insurance, dental, 401(k) match, and paid time off.',
  }));
  expect(result.supported).toBe(true);
  expect(result.refresh_days).toBe(30);
  expect(result.data_updated_at).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  expect(result.next_update_at).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  expect(Array.isArray(result.sources)).toBe(true);
  expect(result.sources.length).toBeGreaterThanOrEqual(5);
});

test('themes and accessibility preferences apply cleanly in Settings', async ({ window }) => {
  await window.locator('#tab-settings').click();
  const swatch = theme => window.locator(`.st3-swatch[data-theme="${theme}"]`);
  await expect(swatch('classic')).toHaveAttribute('aria-checked', 'true');

  for (const theme of ['bubblegum', 'cyber', 'retro', 'december']) {
    await swatch(theme).click();
    await expect(window.locator('html')).toHaveAttribute('data-theme', theme);
    await expect(swatch(theme)).toHaveAttribute('aria-checked', 'true');
  }
  await swatch('auto-season').click();
  await expect(window.locator('html')).toHaveAttribute('data-theme', /^(january|february|march|april|may|june|july|august|september|october|november|december)$/);

  await window.locator('#pref-high-contrast').check();
  await expect(window.locator('html')).toHaveClass(/high-contrast/);
  await window.locator('#pref-underline-links').check();
  await expect(window.locator('html')).toHaveClass(/underline-links/);

  await swatch('classic').click();
  await expect(window.locator('html')).not.toHaveAttribute('data-theme', /.+/);
});

test('the bottom bar is the only navigation, and Jobs always opens the list', async ({ window }) => {
  await expect(window.locator('#nav-home')).toHaveAttribute('aria-current', 'page');
  await expect(window.locator('.section-switches')).toBeHidden();
  await window.locator('#nav-jobs').click();
  await expect(window.locator('#nav-jobs')).toHaveAttribute('aria-current', 'page');
  await expect(window.locator('.app')).toHaveAttribute('data-route', 'list');

  await window.keyboard.press('ControlOrMeta+4');
  await expect(window.locator('.app')).toHaveAttribute('data-route', 'tracker');
  await expect(window.locator('#nav-jobs')).toHaveAttribute('aria-current', 'page');

  await window.evaluate(() => window.App.openTab('tab-find'));
  await window.locator('#nav-jobs').click();
  await expect(window.locator('.app')).toHaveAttribute('data-route', 'list');

  await window.keyboard.press('ControlOrMeta+3');
  await expect(window.locator('#nav-resume')).toHaveAttribute('aria-current', 'page');
});

test('the omnibox searches, and sends a pasted posting to Paste a job', async ({ window }) => {
  await window.keyboard.press('ControlOrMeta+k');
  await expect(window.locator('#omnibox-input')).toBeFocused();
  await window.keyboard.type('data analyst');
  await window.keyboard.press('Enter');
  await expect(window.locator('#search-keywords')).toHaveValue('data analyst');
  await expect(window.locator('#tab-find')).toHaveAttribute('aria-selected', 'true');
  await expect(window.locator('#omnibox-input')).toHaveValue('');

  const posting = 'Data Analyst\nTidewater Logistics\nSavannah, GA\n\nBuild dashboards in Tableau.';
  await window.evaluate(text => window.Omnibox.route(text), posting);
  await expect(window.locator('#paste-pane')).toBeVisible();
  await expect(window.locator('#paste-text')).toHaveValue(posting);
});

test('Home shows the next step, the week, the pipeline and Pico\'s tip from real data', async ({ window }) => {
  const day = n => {
    const d = new Date();
    d.setDate(d.getDate() + n);
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  };
  await window.evaluate(async ({ today, soon }) => {
    const api = window.api;
    await api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com', summary: 'Analyst.' });
    await api.resume.createExperience({ job_title: 'Research Assistant', company_name: 'Coastal University', start_date: '2022-01-01', skills: ['sql', 'excel'] });
    const job = (title, company, skills) => api.jobs.create({ source: 'pasted', position_title: title, company_name: company,
      raw_text: `${title}\n${company}\n\nRequirements:\n${skills.map(s => `- Experience with ${s}`).join('\n')}` });
    const a = await job('Data Analyst', 'Tidewater Logistics', ['sql', 'kubernetes']);
    const b = await job('Backend Engineer', 'Oakline Software', ['sql', 'kubernetes']);
    const c = await job('BI Developer', 'Northgate Energy', ['excel']);
    await api.applications.create({ job_id: b.id, status: 'Applied', date_applied: today, next_step_date: today });
    await api.applications.create({ job_id: c.id, status: 'Interviewing', date_applied: today, next_step_date: soon });
    return a.id;
  }, { today: day(0), soon: day(3) });
  await window.evaluate(() => window.Home.refresh());

  await expect(window.locator('.home3-next h2')).toHaveText('Backend Engineer at Oakline Software');
  await expect(window.locator('.home3-next .btn-primary')).toHaveText('Open');
  await expect(window.locator('.home3-day.today .home3-ev')).toContainText('Follow up');
  await expect(window.locator('.home3-ev.good')).toContainText('Interview');
  await expect(window.locator('#home-pipe-h')).toHaveText('Your 3 jobs');
  await expect(window.locator('.home3-stage').nth(2).locator('button.tile')).toHaveCount(1);
  await expect(window.locator('.home3-scene .bubble')).toContainText('Kubernetes shows up in 2 of your 3 jobs');

  await window.locator('.home3-scene .bubble').getByRole('button', { name: 'Not now' }).click();
  await expect(window.locator('.home3-scene .bubble')).toContainText('Interview');
});

test('a new user sees the welcome, how it works, and Pico', async ({ window }) => {
  await expect(window.locator('#home-greeting')).toContainText('Welcome to');
  await expect(window.locator('.home3-how .home3-step')).toHaveCount(3);
  await expect(window.locator('.home3-scene .pico')).toHaveCount(1);
  await window.locator('.home3-next .btn-primary').click();
  await expect(window.locator('#nav-resume')).toHaveAttribute('aria-current', 'page');
});

test('a saved job opens on its own page with its stage, fit, notes and next step', async ({ window }) => {
  const id = await window.evaluate(async () => {
    const api = window.api;
    await api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com' });
    await api.resume.createExperience({ job_title: 'Research Assistant', company_name: 'Coastal University', start_date: '2022-01-01', skills: ['sql', 'excel'] });
    const job = await api.jobs.create({ source: 'pasted', company_name: 'Tidewater Logistics', position_title: 'Data Analyst',
      raw_text: 'Data Analyst\nTidewater Logistics\n\nRequirements:\n- Experience with sql\n- Experience with kubernetes' });
    return job.id;
  });

  await window.locator('#nav-jobs').click();
  await window.locator('.jl3-open', { hasText: 'Data Analyst' }).click();
  await expect(window).toHaveURL(new RegExp(`#/job/${id}$`));
  await expect(window.locator('#job3-title')).toContainText('Data Analyst');
  await expect(window.locator('.job3-go')).toHaveText('Tailor resume');
  await expect(window.locator('.job3-stop.now b')).toHaveText('Saved');
  await expect(window.locator('.job3-ri .ink-open').first()).toBeVisible();
  await expect(window.locator('.job3-req .aside')).toHaveText('1 of 2 skills on your resume');
  await expect(window.locator('#nav-jobs')).toHaveAttribute('aria-current', 'page');

  await window.locator('.job3-notes-area').fill('Recruiter is Dana Ruiz.');
  await expect(window.locator('.job3-saved')).toHaveText('Saved');
  const notes = await window.evaluate(async jobId => (await window.api.applications.list({})).find(a => a.job_id === jobId)?.notes, id);
  expect(notes).toBe('Recruiter is Dana Ruiz.');

  await window.locator('.job3-stage-select').selectOption('Applied');
  await expect(window.locator('.job3-stop.now b')).toHaveText('Applied');
  await expect(window.locator('.job3-go')).toHaveText('Set a follow-up date');

  await expect(window.locator('.job3-back')).toHaveText('\u2190 Your jobs');
  await window.locator('.job3-back').click();
  await expect(window.locator('.app')).toHaveAttribute('data-route', 'list');
});

test('Tailor shows each change in place on the resume and builds once all are decided', async ({ window }) => {
  const jobId = await window.evaluate(async () => {
    const api = window.api;
    await api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com', summary: 'Analyst with five years of experience turning messy data into decisions.' });
    await api.resume.createExperience({ job_title: 'Research Assistant', company_name: 'Coastal University', start_date: '2022-01-01',
      skills: ['sql', 'excel', 'tableau', 'python'], description: 'Built Tableau dashboards' });
    const job = await api.jobs.create({ source: 'pasted', company_name: 'Tidewater Logistics', position_title: 'Data Analyst',
      raw_text: 'Data Analyst\nTidewater Logistics\n\nRequirements:\n- Experience with sql\n- Experience with excel\n- Experience with tableau' });
    await api.revisions.create({ job_id: job.id, engine: 'rules' });
    return job.id;
  });
  await window.evaluate(id => window.Tailor.open(id), jobId);

  const notes = window.locator('.t3-note');
  await expect(notes.first()).toBeVisible();
  const count = await notes.count();
  expect(count).toBeGreaterThan(0);
  await expect(window.locator('.t3-sheet .t3-name')).toHaveText('Ada Okafor');
  await expect(window.locator('.t3-change').first().locator('ins, del').first()).toBeVisible();
  await expect(window.locator('.t3-priv')).toContainText('Built-in rules');
  await expect(window.locator('.t3-bar-acts .btn-mark')).toBeDisabled();

  for (let i = 0; i < count; i++) await window.keyboard.press('a');
  await expect(window.locator('.t3-count')).toHaveText(`${count} of ${count} decided`);
  await expect(window.locator('.t3-change ins')).toHaveCount(0);
  await expect(window.locator('.t3-bar-acts .btn-mark')).toBeEnabled();
  await expect(window.locator('.t3-finish')).toBeVisible();
});

test('the Jobs list groups what needs you, filters by stage, and moves jobs in bulk', async ({ window }) => {
  await window.evaluate(async () => {
    const api = window.api;
    await api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com' });
    const job = (title, company, extra = {}) => api.jobs.create({ source: 'pasted', position_title: title, company_name: company,
      raw_text: `${title}\n${company}\n\nRequirements:\n- Experience with sql`, ...extra });
    const a = await job('Backend Engineer', 'Oakline Software', { location: 'Remote, US' });
    await job('Product Analyst', 'Fernway Health');
    await job('Financial Analyst', 'Cobalt Rivers');
    const today = new Date();
    const iso = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`;
    await api.applications.create({ job_id: a.id, status: 'Applied', date_applied: iso, next_step_date: iso });
  });
  await window.locator('#nav-jobs').click();

  await expect(window.locator('.jl3-group').first().locator('h2')).toHaveText('Needs you');
  await expect(window.locator('.jl3-group').first().locator('.jl3-next')).toHaveText('Follow up today');
  await expect(window.locator('.jl3-stages [aria-selected="true"]')).toContainText('All');
  await expect(window.locator('.jl3-row')).toHaveCount(3);

  await window.locator('.jl3-quick', { hasText: 'Remote' }).click();
  await expect(window.locator('.jl3-row')).toHaveCount(1);
  await window.locator('.jl3-quick', { hasText: 'Remote' }).click();

  await window.locator('.jl3-row', { hasText: 'Product Analyst' }).locator('.jl3-check').check();
  await window.locator('.jl3-row', { hasText: 'Financial Analyst' }).locator('.jl3-check').check();
  await expect(window.locator('.jl3-bulk b')).toHaveText('2 selected');
  await window.locator('.jl3-move').selectOption('Applied');
  await expect(window.locator('.jl3-stages button', { hasText: 'Applied' }).locator('span')).toHaveText('3');

  await window.locator('.jl3-stages button', { hasText: 'Applied' }).click();
  await expect(window.locator('.jl3-row')).toHaveCount(3);
});

test('Resume shows how ready it is, and each gap opens its field', async ({ window }) => {
  await window.evaluate(async () => {
    await window.api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com' });
  });
  await window.locator('#nav-resume').click();
  await expect(window.locator('.rs3-ready h2')).toHaveText('Ready to tailor?');
  await expect(window.locator('.rs3-check').first()).toContainText('Name and contact');
  await expect(window.locator('.rs3-check', { hasText: 'Write a short summary' })).toBeVisible();
  await expect(window.locator('.rs3-big b')).not.toHaveText('100%');
});

test('the Jobs board shows a column per stage and moves a job with buttons, no dragging', async ({ window }) => {
  await window.evaluate(async () => {
    const api = window.api;
    await api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com' });
    await api.jobs.create({ source: 'pasted', position_title: 'Product Analyst', company_name: 'Fernway Health',
      raw_text: 'Product Analyst\nFernway Health\n\nRequirements:\n- Experience with sql' });
  });
  await window.locator('#nav-jobs').click();
  await window.locator('.jl3-view button', { hasText: 'Board' }).click();
  await expect(window.locator('.jl3-col')).toHaveCount(5);
  await expect(window.locator('.jl3-col').nth(0).locator('.jl3-card')).toHaveCount(1);
  await window.locator('.jl3-col').nth(0).getByRole('button', { name: 'Move on to Applied' }).click();
  await expect(window.locator('.jl3-col').nth(2).locator('.jl3-card')).toHaveCount(1);
  await expect(window.locator('.jl3-col').nth(0).locator('.jl3-card')).toHaveCount(0);
  await window.locator('.jl3-col').nth(2).getByRole('button', { name: 'Move back to Saved' }).click();
  await expect(window.locator('.jl3-col').nth(0).locator('.jl3-card')).toHaveCount(1);
});

test('resume sections reorder by dragging, by the arrows, and by Alt+arrow keys', async ({ window }) => {
  await window.evaluate(async () => { await window.api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com' }); });
  await window.locator('#nav-resume').click();
  const labels = window.locator('.sortlist-label');
  await expect(labels).toHaveText(['Summary', 'Experience', 'Education', 'Skills', 'Custom sections']);

  const rows = window.locator('.sortlist-row');
  await rows.nth(3).scrollIntoViewIfNeeded();
  const from = await rows.nth(3).locator('.sortlist-grip').boundingBox();
  const to = await rows.nth(0).boundingBox();
  await window.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await window.mouse.down();
  for (let i = 1; i <= 10; i++) await window.mouse.move(from.x + from.width / 2, from.y + from.height / 2 - (from.y - to.y + 5) * i / 10);
  await window.mouse.up();
  await expect(labels).toHaveText(['Skills', 'Summary', 'Experience', 'Education', 'Custom sections']);

  await rows.nth(0).focus();
  await window.keyboard.press('Alt+ArrowDown');
  await expect(labels.nth(1)).toHaveText('Skills');

  await window.getByRole('button', { name: 'Move Education up' }).click();
  await expect(labels).toHaveText(['Summary', 'Skills', 'Education', 'Experience', 'Custom sections']);
  await expect.poll(async () => (await window.evaluate(() => window.api.settings.get()))['resume.section_order'])
    .toEqual(['summary', 'skills', 'education', 'experience', 'custom']);
});

test('a running engine test says so on its button, then gives the button back', async ({ window }) => {
  await window.evaluate(() => {
    //a test that takes a moment, so the button can be seen while it runs
    window.api.ai.test = () => new Promise(resolve => setTimeout(() => resolve({ ok: true, latency_ms: 1200 }), 1200));
    window.api.ai.status = async () => ({ mode: 'auto', active_engine: 'rules', local: { available: true }, remote: { configured: false } });
  });
  await window.locator('#tab-settings').click();
  const button = window.locator('#ai-test-local');
  await button.evaluate(b => { b.disabled = false; });
  await button.click();
  await expect(button).toHaveText(/^Testing/);
  await expect(button).toHaveAttribute('aria-busy', 'true');
  await expect(button).toHaveText('Test', { timeout: 5000 });
  await expect(button).not.toHaveAttribute('aria-busy', 'true');
});

test('the search box suggests before anything is typed, and the Search button opens Find', async ({ window }) => {
  await window.evaluate(async () => {
    await window.api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com' });
    await window.api.jobs.create({ source: 'pasted', position_title: 'Data Analyst', company_name: 'Tidewater Logistics',
      raw_text: 'Data Analyst\nTidewater Logistics\n\nRequirements:\n- sql' });
  });
  await window.locator('#omnibox-input').click();
  const panel = window.locator('#omnibox-panel');
  await expect(panel).toBeVisible();
  await expect(panel.getByRole('option', { name: /Browse job boards/ })).toBeVisible();
  await expect(window.locator('#omnibox-input')).toHaveAttribute('aria-expanded', 'true');

  await window.keyboard.type('tidewater');
  await expect(panel.getByRole('option', { name: /Data Analyst/ })).toBeVisible();
  await window.keyboard.press('ArrowDown');
  await window.keyboard.press('Enter');
  await expect(window.locator('.app')).toHaveAttribute('data-route', 'job');

  await window.locator('#omnibox-input').click();
  await window.keyboard.press('Escape');
  await expect(panel).toBeHidden();

  await window.locator('#nav-search').click();
  await expect(window.locator('#search-form')).toBeVisible();
  await expect(window.locator('#nav-search')).toHaveAttribute('aria-current', 'page');
});

test('themes bring their own accent, and Settings can pick another for every theme', async ({ window }) => {
  await window.locator('#tab-settings').click();
  await window.locator('.st3-swatch[data-theme="bubblegum"]').click();
  const mark = () => window.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--mark').trim());
  expect(await mark()).toBe('#ffd3e6');
  await window.locator('.st3-accent[data-accent="mint"]').click();
  await expect(window.locator('html')).toHaveAttribute('data-accent', 'mint');
  expect(await mark()).toBe('#6fdcac');
  await expect(window.locator('.st3-accent[data-accent="mint"]')).toHaveAttribute('aria-checked', 'true');
  await window.locator('.st3-accent[data-accent="theme"]').click();
  await expect(window.locator('html')).not.toHaveAttribute('data-accent', /.+/);
  await window.locator('.st3-swatch[data-theme="classic"]').click();
});

test('the resume target length sets what the page-count badge checks', async ({ window }) => {
  await window.evaluate(async () => {
    await window.api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com', summary: 'Analyst.' });
  });
  await window.locator('#nav-resume').click();
  await expect(window.locator('.rs3-target [aria-checked="true"]')).toHaveText('1 page');
  await expect(window.locator('.fit-pill')).toContainText('1 page');
  await window.locator('.rs3-target button', { hasText: '2 pages' }).click();
  await expect(window.locator('.rs3-target [aria-checked="true"]')).toHaveText('2 pages');
  expect(await window.evaluate(() => window.Prefs.get('resumePages'))).toBe(2);
  await window.locator('.rs3-target button', { hasText: 'Custom' }).click();
  await expect(window.locator('.rs3-target [aria-checked="true"]')).toHaveText('Custom');
  await window.locator('.rs3-target-custom input').fill('4');
  await window.locator('.rs3-target-custom input').press('Enter');
  await window.locator('.rs3-target-custom input').blur();
  await expect.poll(() => window.evaluate(() => window.Prefs.get('resumePages'))).toBe(4);
  await window.locator('.rs3-target button', { hasText: '1 page' }).click();
  await expect(window.locator('.rs3-target-custom')).toBeHidden();
});

test('the navigation bar can move to the top, where it is also the drag area', async ({ window }) => {
  await window.locator('#tab-settings').click();
  await window.locator('.st3-barPosition-row button[data-value="top"]').click();
  await expect(window.locator('html')).toHaveClass(/bar-top/);
  await expect(window.locator('.titlebar')).toBeHidden();
  const bar = await window.locator('.bottombar').boundingBox();
  expect(bar.y).toBeLessThan(5);
  await expect(window.locator('.bottombar')).toHaveAttribute('data-tauri-drag-region', '');
  await window.locator('#omnibox-input').click();
  const panel = await window.locator('#omnibox-panel').boundingBox();
  expect(panel.y).toBeGreaterThan(bar.y + bar.height - 1);
  await window.keyboard.press('Escape');
  await window.locator('#tab-settings').click();
  await window.locator('.st3-barPosition-row button[data-value="bottom"]').click();
  await expect(window.locator('html')).not.toHaveClass(/bar-top/);
  await expect(window.locator('.bottombar')).not.toHaveAttribute('data-tauri-drag-region', '');
});

test('fistbump:// links open a saved job or run a search', async ({ window }) => {
  const id = await window.evaluate(async () => {
    await window.api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com' });
    const job = await window.api.jobs.create({ source: 'pasted', position_title: 'Data Analyst', company_name: 'Tidewater Logistics',
      raw_text: 'Data Analyst\nTidewater Logistics\n\nRequirements:\n- sql' });
    return job.id;
  });
  await window.evaluate(jobId => window.Desktop.openLink(`fistbump://job/${jobId}`), id);
  await expect(window).toHaveURL(new RegExp(`#/job/${id}$`));
  await window.evaluate(() => window.Desktop.openLink('fistbump://search?q=data%20analyst'));
  await expect(window.locator('#search-keywords')).toHaveValue('data analyst');
  await window.evaluate(() => window.Desktop.openLink('https://example.com/not-ours'));
  await expect(window.locator('#search-keywords')).toHaveValue('data analyst');
});

test('a posting file dropped on the window opens in Paste a job', async ({ window }) => {
  await window.evaluate(() => {
    const file = new File(['Data Analyst\nTidewater Logistics\n\nRequirements:\n- SQL'], 'posting.txt', { type: 'text/plain' });
    const data = new DataTransfer();
    data.items.add(file);
    document.querySelector('#view-home').dispatchEvent(new DragEvent('drop', { dataTransfer: data, bubbles: true, cancelable: true }));
  });
  await expect(window.locator('#paste-pane')).toBeVisible();
  await expect(window.locator('#paste-text')).toHaveValue(/Tidewater Logistics/);
});

test('the desktop switches in Settings are off until turned on, and are remembered', async ({ window }) => {
  await window.locator('#tab-settings').click();
  for (const [id, key] of [['#pref-global-shortcut', 'globalShortcut'], ['#pref-menu-bar', 'menuBarItem'], ['#pref-notify-due', 'notifyDue']]) {
    await expect(window.locator(id)).not.toBeChecked();
    await window.locator(id).check();
    expect(await window.evaluate(k => window.Prefs.get(k), key)).toBe(true);
    await window.locator(id).uncheck();
  }
});

test('every theme has a day version, chosen in Settings or following the system', async ({ window }) => {
  await window.locator('#tab-settings').click();
  await window.locator('.st3-mode-row button[data-value="day"]').click();
  await expect(window.locator('html')).toHaveClass(/\bday\b/);
  const bg = await window.evaluate(() => getComputedStyle(document.body).getPropertyValue('--bg'));
  expect(bg).toBeTruthy();
  const canvas = await window.evaluate(() => {
    //painted onto a canvas, whatever colour syntax the page computed comes back as plain RGB
    const ctx = document.createElement('canvas').getContext('2d');
    ctx.fillStyle = getComputedStyle(document.querySelector('.settings-pane')).backgroundColor;
    ctx.fillRect(0, 0, 1, 1);
    const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
    return (r + g + b) / 3;
  });
  expect(canvas).toBeGreaterThan(180);
  await window.locator('.st3-paper-row button[data-value="white"]').click();
  await expect(window.locator('html')).toHaveAttribute('data-paper', 'white');
  await window.locator('.st3-inkMarks-row button[data-value="false"]').click();
  await expect(window.locator('html')).toHaveClass(/no-marks/);
  await window.locator('.st3-mode-row button[data-value="night"]').click();
  await window.locator('.st3-paper-row button[data-value="cream"]').click();
  await window.locator('.st3-inkMarks-row button[data-value="true"]').click();
  await expect(window.locator('html')).not.toHaveClass(/\bday\b/);
});

test('the Tracker is one sheet: dates edit in place and empty ones say what to add', async ({ window }) => {
  await window.evaluate(() => window.api.resume.putProfile({ full_name: 'Ada Okafor', email: 'ada@example.com' }));
  const job = await window.evaluate(() => window.api.jobs.create({ source: 'pasted', raw_text: 'Analyst at Tracker Co\nRemote\nWe need SQL.' }));
  const id = job.id ?? job.data?.id;
  await window.evaluate(jobId => window.api.applications.create({ job_id: jobId, status: 'Saved' }), id);
  await window.evaluate(() => window.App.go('tracker'));
  const row = window.locator('.tr3-table tbody tr').first();
  await expect(row).toBeVisible();
  await expect(row.locator('.tr3-date.blank .tr3-date-empty').first()).toBeVisible();
  await row.locator('.tr3-stage-select').selectOption('Applied');
  await expect(row.locator('.tr3-stage-select')).toHaveAttribute('data-stage', 'Applied');
  await expect(window.locator('.jl3-sum')).toContainText('tracked');
});

test('Report an issue offers similar reports and sends only what it shows', async ({ window }) => {
  await window.evaluate(() => window.Desktop.menu('report'));
  const dialog = window.locator('dialog.report-dialog');
  await expect(dialog).toBeVisible();
  await dialog.locator('#report-what').fill('Search results never load');
  await dialog.locator('#report-details').fill('The list stays empty. Write to ada@example.com');
  await expect(dialog.locator('.report-alike')).toContainText('Search results never load');
  await expect(dialog.locator('.report-preview')).toContainText('[email removed]');
  await dialog.getByRole('button', { name: 'Send report' }).click();
  await expect(dialog).toContainText("Thank you, it's filed");
  const sent = await window.evaluate(() => window.__report);
  expect(sent.details).not.toContain('ada@example.com');
  await dialog.getByRole('button', { name: 'Done' }).click();
});

test('the menu bar runs the same actions as the buttons', async ({ window }) => {
  await window.evaluate(() => window.Desktop.menu('go-resume'));
  await expect(window.locator('.app')).toHaveAttribute('data-route', 'resume');
  await window.evaluate(() => window.Desktop.menu('mode-day'));
  await expect(window.locator('html')).toHaveClass(/\bday\b/);
  await window.evaluate(() => window.Desktop.menu('mode-night'));
  await window.evaluate(() => window.Desktop.menu('paste-job'));
  await expect(window.locator('#paste-text')).toBeFocused();
});

test('boards added to the watchlist are shared only when sharing is on', async ({ window }) => {
  await window.locator('#tab-settings').click();
  await window.locator('#watchlist-input').fill('lever:examplecorp');
  await window.locator('#watchlist-add').click();
  await expect(window.locator('#watchlist-status')).toContainText('Added');
  expect(await window.evaluate(() => window.__shared)).toBeUndefined();
  await window.locator('#pref-community-share').check();
  await expect.poll(() => window.evaluate(() => window.__shared?.boards)).toEqual(['lever:examplecorp']);
  await window.locator('#pref-community-share').uncheck();
});

test('the calendar plans an interview, flags a clash with a busy time, and adds it to a calendar app', async ({ window }) => {
  await window.evaluate(async () => {
    const at = (days, h) => { const d = new Date(); d.setDate(d.getDate() + days); d.setHours(h, 0, 0, 0); return d; };
    const u = d => d.toISOString().replace(/[-:]/g, '').replace(/\.\d+/, '');
    window.__feeds = true;
    window.__busyIcs = ['BEGIN:VCALENDAR', 'BEGIN:VEVENT', `DTSTART:${u(at(1, 10))}`, `DTEND:${u(at(1, 11))}`, 'SUMMARY:Private', 'END:VEVENT', 'END:VCALENDAR'].join('\r\n');
    window.Prefs.set('calendarView', 'week');
    await window.CalendarView.refreshBusy();
  });
  await window.locator('#nav-calendar').click();
  await expect(window.locator('.app')).toHaveAttribute('data-route', 'calendar');
  await expect(window.locator('.cal3-sync')).toContainText('Checking 1 calendar');
  await window.getByRole('button', { name: 'Plan an interview' }).click();
  const dialog = window.locator('dialog.plan-dialog');
  await expect(dialog).toBeVisible();
  const day = await window.evaluate(() => { const d = new Date(); d.setDate(d.getDate() + 1); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`; });
  await dialog.locator('#plan-date').fill(day);
  await dialog.locator('#plan-time').fill('10:30');
  await dialog.locator('#plan-title').fill('Interview, round 2');
  await expect(dialog.locator('.plan-clash')).toContainText('Overlaps something on your calendar');
  await dialog.getByRole('button', { name: 'Add to your calendar' }).click();
  await expect(dialog).toBeHidden();
  const plan = window.locator('.cal3-plan', { hasText: 'Interview, round 2' });
  await expect(plan).toHaveClass(/clash/);
  await plan.click();
  await window.locator('dialog.plan-dialog').getByRole('button', { name: 'Calendar app' }).click();
  const opened = await window.evaluate(() => window.__openedIcs);
  expect(opened.text).toContain('BEGIN:VEVENT');
  expect(opened.text).toContain('SUMMARY:Interview\\, round 2');
  expect(opened.text).not.toContain('Private');
});

test('an available update is offered once, and can be put off or skipped', async ({ window }) => {
  await window.locator('#tab-settings').click();
  await window.evaluate(() => { window.api.app.checkForUpdates = async () => ({ status: 'available', latest: '9.9.9', current: '0.5.0', downloadable: true, url: 'x' }); });
  await window.evaluate(() => { window.Settings.promptUpdate(); });
  const dialog = window.locator('dialog.update-notice');
  await expect(dialog).toContainText('9.9.9 is available');
  await dialog.locator('#skip-update').check();
  await dialog.getByRole('button', { name: 'Not now' }).click();
  await expect(dialog).toBeHidden();
  expect(await window.evaluate(() => window.Prefs.get('skippedUpdate'))).toBe('9.9.9');
  await window.evaluate(() => window.Settings.promptUpdate());
  await expect(window.locator('dialog.update-notice')).toHaveCount(0);
});

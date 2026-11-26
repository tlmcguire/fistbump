// The Home view (v3, "Paper on Night"): a greeting with where things stand, the one thing to do next, this
// month's Pico with a tip, the next seven days, the pipeline as company tiles, and new matches.
// Every panel opens the place it names. It reads the same data the other views keep, so it is drawn again
// every time it is shown. See DESIGN.md for the visual rules and docs/mockups/home-v3.html for the target.
(() => {
  const api = () => window.FistBumpAPI;
  const {el, fill} = window.ResumeBuilder;

  const HISTORY_KEY = "fistbump.search-history";
  const LISTING_KEY = "fistbump.last-listing";

  const readJSON = key => {
    try {
      return JSON.parse(localStorage.getItem(key) || "null");
    } catch {
      return null;
    }
  };

  //dates follow the app's language, not only the system's
  const locale = () => window.I18n?.locale() || undefined;

  //"Today", "Yesterday" or "Oct 24"
  const day = iso => {
    const then = new Date(iso);
    if (!iso || Number.isNaN(then.getTime())) return "";
    const start = d => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
    const days = Math.round((start(new Date()) - start(then)) / 86400000);
    if (days === 0) return "Today";
    if (days === 1) return "Yesterday";
    return then.toLocaleDateString(locale(), {month: "short", day: "numeric"});
  };

  const greeting = () => {
    const hour = new Date().getHours();
    return hour < 12 ? "Good morning" : hour < 18 ? "Good afternoon" : "Good evening";
  };

  // ---------- Time away ----------

  //When Fistbump was last open. It is read once, when the app starts, so Home can welcome someone back
  //for the rest of that run, and then kept current while the app is open.
  const SEEN_KEY = "fistbump.last-seen";
  const AWAY = [
    {days: 180, text: "It's been over six months. Pico dusted itself off for you."},
    {days: 90, text: "It's been about three months since you were here."},
    {days: 30, text: "It's been about a month since you were here."},
    {days: 14, text: "It's been a couple of weeks since you were here."},
    {days: 7, text: "It's been a week since you were here."},
  ];
  const away = (() => {
    const last = Date.parse(readJSON(SEEN_KEY) || "");
    if (Number.isNaN(last)) return null;
    const days = (Date.now() - last) / 86400000;
    return AWAY.find(a => days >= a.days) || null;
  })();
  const seen = () => {
    try {
      localStorage.setItem(SEEN_KEY, JSON.stringify(new Date().toISOString()));
    } catch {}
  };
  seen();
  setInterval(seen, 10 * 60 * 1000);
  window.addEventListener("beforeunload", seen);

  //Pico, the mascot. Home shows this month's pose, the same art all month, as a season marker.
  const MASCOTS = "views/graphics/mascots";
  const MONTHS = ["01-january", "02-february", "03-march", "04-april", "05-may", "06-june",
    "07-july", "08-august", "09-september", "10-october", "11-november", "12-december"];
  const monthPose = () => `${MASCOTS}/monthly/${MONTHS[new Date().getMonth()]}.png`;

  //A hand-drawn mark from the sprite (#ink-*), drawn in currentColor.
  const ink = (id, cls = "") => {
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("class", `ink ink-${id} ${cls}`.trim());
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS(ns, "use");
    use.setAttribute("href", `#ink-${id}`);
    svg.append(use);
    return svg;
  };

  // ---------- Where each row goes ----------

  const openResume = target => {
    window.App.go("resume");
    window.ResumeBuilder.reveal?.(target);
  };
  const openJobs = () => window.App.go("jobs");

  // ---------- What the master resume still lacks ----------

  //Each gap is a row that opens the Resume view at the field or entry to fill in.
  function resumeGaps(master) {
    const p = master.profile;
    if (!p) return [];
    const gaps = [];
    if (!p.full_name) gaps.push({title: "Add your name", detail: "It heads every resume you send", target: {field: "full_name"}});
    if (!p.email && !p.phone) gaps.push({title: "Add how to reach you", detail: "An email address or phone number", target: {field: "email"}});
    if (!p.summary) gaps.push({title: "Write a short summary", detail: "Two or three sentences about what you do best", target: {field: "summary"}});
    if (!master.experiences.length) gaps.push({title: "Add your work experience", detail: "Jobs, internships or volunteer roles", target: {add: "experience"}});
    for (const e of master.experiences.filter(e => !e.impact).slice(0, 2)) {
      gaps.push({title: `Add impact to ${e.job_title}`, detail: e.company_name, target: {kind: "experience", id: e.id, field: "impact"}});
    }
    if (!master.education.length) gaps.push({title: "Add your education", detail: "Schools, degrees or certificates", target: {add: "education"}});
    return gaps;
  }

  //"Done: Name · Contact details · 2 jobs". Each part is its own phrase, so it translates on its own.
  function resumeDone(master) {
    const p = master.profile;
    const done = [];
    if (p.full_name) done.push("Name");
    if (p.email || p.phone) done.push("Contact details");
    if (p.summary) done.push("Summary");
    if (master.experiences.length) done.push(master.experiences.length === 1 ? "1 job" : `${master.experiences.length} jobs`);
    if (master.education.length) done.push(master.education.length === 1 ? "1 school" : `${master.education.length} schools`);
    return done.length ? `Done: ${done.join(" \u00b7 ")}` : "";
  }

  // ---------- What needs you ----------

  //Steps due or overdue, suggestions still to decide, and saved jobs with no tailored resume yet: the work in
  //flight, most urgent first. Kept to a few rows; the Tracker and Tailor hold the rest.
  async function pipeline(limit = 4, preloadedApps = null, preloadedJobs = null) {
    const today = window.Tracker?.localToday?.() || new Date().toISOString().slice(0, 10);
    const [apps, jobs] = await Promise.all([
      preloadedApps !== null ? Promise.resolve(preloadedApps) : api().applications.list({}).catch(() => []),
      preloadedJobs !== null ? Promise.resolve(preloadedJobs) : api().jobs.list("").catch(() => []),
    ]);
    const items = [];
    for (const a of (apps || []).filter(a => a.next_step_date && a.next_step_date <= today && !a.archived_at)
      .sort((x, y) => x.next_step_date < y.next_step_date ? -1 : 1)) {
      items.push({
        kind: "due",
        title: [a.position_title, a.company_name || "Unknown company"].filter(Boolean).join(" at "),
        detail: a.next_step_date < today ? "Next step is overdue" : "Next step is due today",
        aside: "Open", onclick: () => window.JobPage.open(a.job_id),
      });
    }
    const tracked = new Set((apps || []).map(a => a.job_id));
    for (const job of (jobs || []).slice(0, 6)) {
      if (items.length >= limit) break;
      const rounds = await api().revisions.list(job.id).catch(() => []);
      const name = [job.position_title, job.company_name].filter(Boolean).join(" at ");
      const latest = rounds?.[0];
      if (latest?.status === "done") {
        const full = await api().revisions.get(latest.id).catch(() => null);
        const left = full ? full.suggestions.filter(s => s.state === "pending").length : 0;
        if (left) {
          items.push({kind: "review", title: name, detail: left === 1 ? "1 suggestion to decide" : `${left} suggestions to decide`,
            aside: "Review", onclick: () => window.Tailor.open(job.id)});
          continue;
        }
      }
      if (!rounds?.length && !tracked.has(job.id)) {
        items.push({kind: "tailor", title: name, detail: "Saved, not tailored yet", aside: "Tailor", onclick: () => window.Tailor.open(job.id)});
      }
    }
    return items.slice(0, limit);
  }

  // ---------- Jobs that fit you ----------

  //Matches come from the same Greenhouse search as Find jobs, run with what the profile says: the target
  //positions, the home location and the preferred work mode. They are rated against the resume, so the best
  //fits come first. A search scans many boards, so the answer is kept for half an hour and drawn after Home.
  const MATCH_TTL = 30 * 60 * 1000;
  const MATCH_SHOWN = 5;
  let matchCache = null; // {key, at, items}
  let matchLoad = null; // {key, promise}
  const invalidateMatches = () => {
    matchCache = null;
    matchLoad = null;
  };
  const clearLastListing = () => {
    try { localStorage.removeItem(LISTING_KEY); } catch {}
  };

  const modeOf = p => ["Remote", "Hybrid", "On-site"].includes(p.preferred_mode) ? p.preferred_mode : "Any";
  const matchQuery = p => ({
    keywords: (p.target_positions || []).map(t => String(t).trim()).filter(Boolean).slice(0, 5),
    location: (p.home_location || "").trim(),
    work_mode: modeOf(p),
    use_resume: true,
    sort: "relevance",
    limit: 30,
    //only boards a search has already listed: showing Home must not send a request to every board
    cached_only: true,
  });
  const matchKey = p => JSON.stringify([matchQuery(p), p.updated_at]);

  const payLine = job => {
    const k = n => `$${Math.round(n / 1000)}k`;
    if (job.pay_min && job.pay_max) return job.pay_min === job.pay_max ? k(job.pay_min) : `${k(job.pay_min)}–${k(job.pay_max)}`;
    return job.pay_min || job.pay_max ? k(job.pay_min || job.pay_max) : "";
  };

  async function findMatches(p) {
    const [data, saved] = await Promise.all([
      api().suggest(matchQuery(p)),
      api().jobs.list("", {archived: "include"}).catch(() => []),
    ]);
    const have = new Set((saved || []).filter(j => j.board && j.external_id).map(j => `${j.board}/${j.external_id}`));
    const items = (data.postings || [])
      //a posting that shows less than two in five of its required skills is a stretch, not a match
      .filter(j => !have.has(`${j.board}/${j.external_id}`) && (!j.fit || j.fit.percent >= 40))
      .slice(0, MATCH_SHOWN);
    items.rateLimited = Boolean(data.rate_limited); //Greenhouse skipped some boards or descriptions
    items.nothingCached = !data.boards_searched; //no search has listed any board yet
    return items;
  }

  //"Data Analyst, Security Analyst · Remote · Austin, TX"
  const matchScope = p => [
    (p.target_positions || []).slice(0, 3).join(", "),
    modeOf(p) === "Any" ? "" : modeOf(p),
    p.home_location,
  ].filter(Boolean).join(" · ");

  // ---------- Recent searches ----------

  //The search history the Jobs view keeps, described in a line: what, then where and how.
  function recentSearches(limit = 4) {
    const list = readJSON(HISTORY_KEY);
    if (!Array.isArray(list)) return [];
    const seen = new Set();
    const out = [];
    for (const entry of list.filter(entry => entry?.fields && typeof entry.fields === "object")) {
      const f = entry.fields;
      //keywords as a clean line: no stray commas, so a title never reads "data analyst,"
      const text = id => id === "search-keywords"
        ? String(f[id] ?? "").split(",").map(w => w.trim()).filter(Boolean).join(", ")
        : String(f[id] ?? "").trim();
      const title = text("search-keywords") || (text("search-company") && `Jobs at ${text("search-company")}`) || entry.industry || "All jobs";
      const detail = [
        text("search-location"),
        text("search-keywords") && text("search-company") && `at ${text("search-company")}`,
        text("search-work-mode") && text("search-work-mode") !== "Any" && text("search-work-mode"),
        text("search-keywords") && entry.industry,
      ].filter(Boolean).join(" \u00b7 ") || "Anywhere";
      //the same search run twice is one line
      const key = `${title}|${detail}`.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      out.push({title, detail, aside: day(entry.at), onclick: () => {
        openJobs();
        window.JobParser.runSearch(f);
      }});
      if (out.length === limit) break;
    }
    return out;
  }

  // ---------- Dates for the next seven days ----------

  const pad = n => String(n).padStart(2, "0");
  const isoDay = d => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  const addDays = (d, n) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);

  //US public holidays and a few days most offices close, by date. The cost figures are US-only too.
  function holidays(year) {
    const nth = (month, weekday, n) => { //the nth weekday of a month; n = -1 is the last
      if (n > 0) {
        const first = new Date(year, month, 1);
        return new Date(year, month, 1 + ((weekday - first.getDay() + 7) % 7) + (n - 1) * 7);
      }
      const last = new Date(year, month + 1, 0);
      return new Date(year, month, last.getDate() - ((last.getDay() - weekday + 7) % 7));
    };
    const list = [
      [new Date(year, 0, 1), "New Year's Day"], [nth(0, 1, 3), "Martin Luther King Jr. Day"], [nth(1, 1, 3), "Presidents' Day"],
      [nth(4, 1, -1), "Memorial Day"], [new Date(year, 5, 19), "Juneteenth"], [new Date(year, 6, 4), "Independence Day"],
      [nth(8, 1, 1), "Labor Day"], [new Date(year, 10, 11), "Veterans Day"], [nth(10, 4, 4), "Thanksgiving"],
      [new Date(year, 11, 24), "Christmas Eve"], [new Date(year, 11, 25), "Christmas Day"], [new Date(year, 11, 31), "New Year's Eve"],
    ];
    return new Map(list.map(([d, name]) => [isoDay(d), name]));
  }

  //What a next step is called, from where the application stands. The tracker keeps a date, not a label.
  const STEP = {
    Interviewing: {label: "Interview", verb: "Prep", tone: "good"},
    Applied: {label: "Follow up", verb: "Open", tone: "urgent"},
    Offered: {label: "Reply to the offer", verb: "Open", tone: "mark"},
    Saved: {label: "Next step", verb: "Open", tone: ""},
  };

  //The next seven days as columns. Overdue steps land on today. Empty Saturday and Sunday share one column.
  function weekColumns(apps, jobs) {
    const now = new Date();
    const today = isoDay(now);
    const days = Array.from({length: 7}, (_, i) => addDays(now, i));
    const last = isoDay(days[6]);
    const off = new Map([...holidays(now.getFullYear()), ...holidays(now.getFullYear() + 1)]);
    const events = new Map(days.map(d => [isoDay(d), []]));
    for (const a of apps) {
      if (!a.next_step_date || a.archived_at || a.status === "Rejected" || a.next_step_date > last) continue;
      const step = STEP[a.status] || STEP.Saved;
      const overdue = a.next_step_date < today;
      events.get(overdue ? today : a.next_step_date)?.push({
        title: overdue ? `${step.label}, overdue` : step.label,
        who: a.company_name || a.position_title || "",
        verb: step.verb,
        tone: overdue ? "urgent" : step.tone,
        open: () => a.status === "Interviewing" ? window.Tracker.open(a.id) : openSavedJob(a.job_id),
      });
    }
    const tracked = new Set(apps.map(a => a.job_id));
    for (const j of jobs) {
      const close = (j.close_date || "").slice(0, 10);
      if (!close || close < today || close > last) continue;
      events.get(close)?.push({
        title: "Posting closes",
        who: j.company_name || j.position_title || "",
        verb: tracked.has(j.id) ? "Open" : "Tailor",
        tone: "mark",
        open: () => tracked.has(j.id) ? openSavedJob(j.id) : window.Tailor.open(j.id),
      });
    }
    const columns = [];
    for (let i = 0; i < days.length; i++) {
      const d = days[i];
      const key = isoDay(d);
      const next = days[i + 1];
      //an empty weekend is one column, so the week spends its width on days that hold something
      if (d.getDay() === 6 && next && !events.get(key).length && !events.get(isoDay(next)).length && !off.has(key) && !off.has(isoDay(next))) {
        columns.push({date: d, weekend: true, events: []});
        i++;
        continue;
      }
      columns.push({date: d, today: key === today, holiday: off.get(key), events: events.get(key)});
    }
    return columns;
  }

  // ---------- Stages, for the pipeline ----------

  const STAGES = [
    {key: "saved", label: "Saved"}, {key: "tailored", label: "Tailored"}, {key: "applied", label: "Applied"},
    {key: "interview", label: "Interview"}, {key: "offer", label: "Offer"},
  ];
  const APP_STAGE = {Applied: "applied", Interviewing: "interview", Offered: "offer"};

  //Each active job in one stage: the tracker's status once it has applied; before that, Tailored when a round of
  //suggestions has finished, else Saved. Rejected and archived jobs are left out.
  async function stagesOf(apps, jobs) {
    const appOf = new Map(apps.filter(a => !a.archived_at).map(a => [a.job_id, a]));
    const list = jobs.slice(0, 60);
    const rounds = await Promise.all(list.map(j => {
      const a = appOf.get(j.id);
      return a && APP_STAGE[a.status] ? null : api().revisions.list(j.id).catch(() => []);
    }));
    const byStage = Object.fromEntries(STAGES.map(s => [s.key, []]));
    list.forEach((j, i) => {
      const a = appOf.get(j.id);
      if (a?.status === "Rejected") return;
      const stage = a && APP_STAGE[a.status] ? APP_STAGE[a.status] : rounds[i]?.some(r => r.status === "done") ? "tailored" : "saved";
      byStage[stage].push({job: j, app: a});
    });
    return byStage;
  }

  // ---------- Pico's tip: the skill your jobs ask for most that the resume lacks ----------

  const TIP_KEY = "fistbump.home-tip-dismissed";
  async function gapTip(byStage) {
    const active = ["saved", "tailored", "applied", "interview"].flatMap(k => byStage[k]).map(e => e.job).slice(0, 25);
    if (active.length < 2) return null;
    const results = await Promise.all(active.map(j => api().jobs.analyze(j.id).catch(() => null)));
    const count = new Map();
    for (const r of results) {
      for (const skill of new Set(r?.required?.tech?.missing || [])) count.set(skill, (count.get(skill) || 0) + 1);
    }
    const dismissed = readJSON(TIP_KEY) || [];
    const [skill, n] = [...count].filter(([k]) => !dismissed.includes(k)).sort((a, b) => b[1] - a[1])[0] || [];
    return skill && n >= 2 ? {skill, n, of: active.length} : null;
  }
  const dismissTip = skill => {
    const list = readJSON(TIP_KEY) || [];
    try { localStorage.setItem(TIP_KEY, JSON.stringify([...new Set([...list, skill])].slice(-40))); } catch {}
  };

  const openSavedJob = id => window.JobPage.open(id);

  // ---------- The view ----------

  function mount(container) {
    container.classList.add("v3", "home3");
    const scroll = el("div", {class: "home3-scroll"});
    container.replaceChildren(scroll);

    let seq = 0;
    async function refresh() {
      const mine = ++seq;
      const [master, apps, jobs] = await Promise.all([
        api().resume.get().catch(() => null),
        api().applications.list({}).catch(() => []),
        api().jobs.list("").catch(() => []),
      ]);
      const [needs, byStage] = await Promise.all([
        pipeline(4, apps || [], jobs || []).catch(() => []),
        stagesOf(apps || [], jobs || []).catch(() => Object.fromEntries(STAGES.map(s => [s.key, []]))),
      ]);
      if (mine !== seq) return;
      // Drop the remembered "Keep reading" listing if it was a saved job that was deleted, trashed, or archived.
      const kept = readJSON(LISTING_KEY);
      if (kept?.id && !(jobs || []).some(j => j.id === kept.id)) clearLastListing();
      const state = {master: master || {profile: null, experiences: [], education: []}, apps: apps || [], jobs: jobs || [], needs, byStage, tip: null};
      render(state);
      //the tip runs the skill analysis over every active job, so it arrives after the rest of Home
      const tip = await gapTip(byStage).catch(() => null);
      if (mine !== seq || !tip) return;
      state.tip = tip;
      fillScene(state, true);
    }

    let parts = {};

    function render(state) {
      const p = state.master.profile;
      parts = {
        hero: el("section", {class: "home3-hero", "aria-labelledby": "home-greeting"}),
        scene: el("section", {class: "home3-scene", "aria-label": "Pico"}),
        week: el("section", {class: "paper home3-week", "aria-labelledby": "home-week-h"}),
        pipe: el("section", {class: "paper home3-pipe", "aria-labelledby": "home-pipe-h"}),
        matches: el("section", {class: "paper home3-matches", "aria-labelledby": "home-matches-h"}),
      };
      scroll.replaceChildren(el("div", {class: "home3-grid"}, parts.hero, parts.scene,
        p ? [parts.week, parts.pipe, parts.matches] : howItWorks()));
      fillHero(state);
      fillScene(state);
      if (!p) return;
      fillWeek(state);
      fillPipe(state);
      renderMatches(p);
      window.UI?.stagger?.(scroll.querySelector(".home3-grid"), 45, 6);
    }

    // ----- Greeting and Up next -----
    function fillHero(state) {
      const {master, needs} = state;
      const p = master.profile;
      const gaps = resumeGaps(master);
      const kept = readJSON(LISTING_KEY);
      const listing = typeof kept?.raw_text === "string" ? kept : null;
      const today = new Date().toLocaleDateString(locale(), {weekday: "long", month: "long", day: "numeric"});
      const firstName = (p?.full_name || "").split(/\s+/)[0];

      if (!p) {
        fill(parts.hero,
          el("h1", {id: "home-greeting", class: "display-xl"}, "Welcome to ", el("span", {class: "underlined", translate: "no"}, "Fistbump", ink("underline"))),
          el("p", {class: "home3-sub"}, "Start with your master resume. Every version you tailor for a job begins from it, and you can import one you already have."),
          upNext({kicker: "Start here", title: "Your master resume", detail: "Import a file, paste text or fill it in", action: "Begin",
            onclick: () => openResume({field: "full_name"})}));
        return;
      }

      const hello = away ? "Welcome back" : greeting();
      const due = needs.filter(n => n.kind === "due").length;
      const review = needs.filter(n => n.kind === "review").length;
      const waiting = due + review;
      const sub = el("p", {class: "home3-sub"}, el("span", {translate: "no", text: today}));
      const add = (...nodes) => sub.append(" · ", ...nodes);
      if (away) add(away.text);
      else if (waiting) add(el("b", {text: waiting === 1 ? "1 thing" : `${waiting} things`}), " need you today");
      else add("Nothing is due today");
      if (gaps.length) {
        add("resume: ", el("a", {href: "#", text: gaps[0].title.charAt(0).toLowerCase() + gaps[0].title.slice(1),
          onclick: event => { event.preventDefault(); openResume(gaps[0].target); }}));
      } else add("resume ready to tailor");

      const next = needs[0];
      const card = listing && !next ? upNext({
        kicker: "Keep reading", title: [listing.position_title || "Unknown position", listing.company_name].filter(Boolean).join(" at "),
        detail: [listing.location, listing.work_mode].filter(Boolean).join(" · "), action: "Open",
        onclick: () => { openJobs(); window.JobParser.openListing(listing); },
      }) : next ? upNext({
        kicker: next.kind === "due" ? "Due now" : next.kind === "review" ? "Ready for your review" : "Up next",
        urgent: next.kind === "due",
        title: next.title, detail: next.detail,
        action: next.kind === "review" ? "Review changes" : next.kind === "tailor" ? "Tailor resume" : "Open",
        onclick: next.onclick,
      }) : upNext({
        kicker: "Up next", title: "Find your next role", detail: "Search Greenhouse, Lever and Ashby boards by title, place or company",
        action: "Search", onclick: () => window.Omnibox?.focus(),
      });

      fill(parts.hero,
        el("h1", {id: "home-greeting", class: "display-xl"}, `${hello}, `,
          el("span", {class: "underlined", translate: "no"}, firstName ? `${firstName}.` : "friend.", ink("underline"))),
        sub, card);
    }

    //First run: the three steps, so a new user sees the whole path before starting it.
    function howItWorks() {
      const steps = [
        ["Your master resume", "Everything you have done, in one place. Each tailored version starts from it."],
        ["Add a job", "Search the job boards, paste a link, or paste the posting into the bar below."],
        ["Tailor and review", "See which skills the job asks for, then accept, reject or edit each suggested change."],
      ];
      return el("section", {class: "paper home3-how", "aria-labelledby": "home-how-h"},
        el("div", {class: "paper-head"}, el("h2", {id: "home-how-h", text: "How Fistbump works"})),
        el("div", {class: "home3-steps"}, steps.map(([title, text], i) => el("div", {class: "home3-step"},
          el("span", {class: "num-mark", text: String(i + 1)}), el("b", {text: title}), el("p", {text})))));
    }

    function upNext({kicker, title, detail, action, onclick, urgent = false}) {
      return el("div", {class: "paper home3-next"},
        el("div", {class: "home3-next-text"},
          el("span", {class: urgent ? "home3-kicker urgent" : "home3-kicker", text: kicker}),
          el("h2", {class: "display-l", text: title}),
          detail ? el("p", {class: "ink-2", text: detail}) : null),
        el("button", {type: "button", class: "btn btn-primary", text: action, onclick}));
    }

    // ----- Pico and its one bubble -----
    function fillScene(state, animate = false) {
      const p = state.master.profile;
      let bubble = null;
      if (!p) {
        fill(parts.scene,
          el("div", {class: "bubble tail-right"},
            el("h2", {class: "display-m", text: "Hi, I'm Pico."}),
            el("p", {text: "Your job-search sidekick. Everything stays on this computer, and you approve every word."})),
          el("img", {class: "pico", src: monthPose(), alt: ""}));
        return;
      }
      const interview = state.apps.find(a => a.status === "Interviewing" && a.next_step_date
        && a.next_step_date >= isoDay(new Date()) && a.next_step_date <= isoDay(addDays(new Date(), 6)));
      if (state.tip) {
        const {skill, n, of} = state.tip;
        bubble = el("div", {class: "bubble tail-right"},
          el("h2", {class: "display-m"}, el("span", {translate: "no", text: skill.charAt(0).toUpperCase() + skill.slice(1)}), ` shows up in ${n} of your ${of} jobs.`),
          el("p", {text: n === 2 ? "Used it? Add it to your resume and your fit goes up on both." : `Used it? Add it to your resume and your fit goes up on all ${n}.`}),
          el("div", {class: "home3-bubble-acts"},
            el("button", {type: "button", class: "btn btn-primary", text: "Add to resume", onclick: () => openResume({add: "experience"})}),
            el("button", {type: "button", class: "btn", text: "Not now", onclick: () => { dismissTip(skill); state.tip = null; fillScene(state); }})));
      } else if (interview) {
        const when = new Date(`${interview.next_step_date}T12:00`).toLocaleDateString(locale(), {weekday: "long"});
        bubble = el("div", {class: "bubble tail-right"},
          el("h2", {class: "display-m"}, `Interview ${when}`, el("span", {translate: "no", text: interview.company_name ? ` with ${interview.company_name}` : ""}), "."),
          el("p", {text: "Want to prep? The tracker has practice questions for it."}),
          el("div", {class: "home3-bubble-acts"},
            el("button", {type: "button", class: "btn btn-primary", text: "Prep", onclick: () => window.Tracker.open(interview.id)})));
      } else if (away) {
        bubble = el("div", {class: "bubble tail-right"}, el("p", {class: "home3-bubble-line", text: away.text}));
      }
      fill(parts.scene, bubble, el("img", {class: "pico", src: monthPose(), alt: ""}));
      if (animate && bubble) window.UI?.play?.(bubble, "enter", 320);
    }

    // ----- Next seven days -----
    function fillWeek(state) {
      const cols = weekColumns(state.apps, state.jobs);
      const anything = cols.some(c => c.events.length);
      fill(parts.week,
        el("div", {class: "paper-head"}, el("h2", {id: "home-week-h", text: "Next 7 days"}),
          el("a", {href: "#/tracker", text: "Open tracker"})),
        el("div", {class: "home3-days"}, cols.map(c => {
          const num = el("b", {class: "num", translate: "no", text: String(c.date.getDate())});
          const label = c.weekend ? "Sat · Sun" : c.today ? "Today"
            : c.date.getDate() === 1 ? c.date.toLocaleDateString(locale(), {month: "short"})
            : c.date.toLocaleDateString(locale(), {weekday: "short"});
          return el("div", {class: ["home3-day", c.today && "today", (c.holiday || c.weekend) && "off", !c.events.length && "empty"].filter(Boolean).join(" ")},
            el("div", {class: "home3-date"}, c.today ? el("span", {class: "circled"}, num, ink("circle")) : num, el("span", {text: label})),
            c.holiday ? el("span", {class: "home3-free", text: c.holiday}) : null,
            c.weekend ? el("span", {class: "home3-free", text: anything ? "Free. A good time to prep." : "Free"}) : null,
            c.events.map(e => el("button", {type: "button", class: `home3-ev ${e.tone}`, onclick: e.open},
              el("span", {}, el("b", {text: e.title}), el("span", {translate: "no", text: e.who})),
              el("span", {class: "home3-verb", text: e.verb}))));
        })),
        anything ? null : el("p", {class: "home3-week-note ink-2", text: "Nothing scheduled this week. Next steps you set in the tracker, and closing dates of saved jobs, show up here."}));
    }

    // ----- Pipeline as company tiles -----
    function fillPipe(state) {
      const {byStage, apps} = state;
      const total = STAGES.reduce((n, s) => n + byStage[s.key].length, 0);
      const month = isoDay(new Date()).slice(0, 7);
      const sent = apps.filter(a => (a.date_applied || "").startsWith(month)).length;
      const interviews = apps.filter(a => a.status === "Interviewing" || a.status === "Offered").length;
      const tile = (entry, stage) => el("button", {
        type: "button", class: stage === "interview" ? "tile tile-mark" : "tile",
        title: [entry.job.position_title, entry.job.company_name].filter(Boolean).join(", "),
        "aria-label": [entry.job.position_title, entry.job.company_name].filter(Boolean).join(", "),
        text: (entry.job.company_name || entry.job.position_title || "?").trim().charAt(0).toUpperCase(),
        onclick: () => openSavedJob(entry.job.id),
      });
      fill(parts.pipe,
        el("div", {class: "paper-head"}, el("h2", {id: "home-pipe-h", text: total === 1 ? "Your 1 job" : `Your ${total} jobs`}),
          el("a", {href: "#", text: "All jobs", onclick: event => { event.preventDefault(); document.getElementById("nav-jobs").click(); }})),
        total ? el("div", {class: "home3-stages"}, STAGES.map(s => {
          const list = byStage[s.key];
          const shown = list.slice(0, list.length > 4 ? 3 : 4);
          return el("div", {class: "home3-stage"},
            el("b", {class: "num", text: String(list.length)}),
            el("div", {class: "home3-stack"},
              list.length ? shown.map(e => tile(e, s.key)) : el("span", {class: "tile tile-slot", title: s.key === "offer" ? "Your first offer goes here" : `Nothing ${s.label.toLowerCase()} yet`},
                s.key === "offer" ? ink("star") : null),
              list.length > shown.length ? el("span", {class: "tile home3-more", text: `+${list.length - shown.length}`}) : null),
            el("span", {class: "home3-stage-label", text: s.label}));
        })) : el("p", {class: "ink-2 home3-empty", text: "Save a job from a search or a pasted posting, and it starts here."}),
        el("div", {class: "home3-pipe-foot ruled-top"},
          el("span", {class: "ink-2", text: sent === 1 ? "1 application sent this month" : `${sent} applications sent this month`}),
          interviews ? el("span", {class: "ink-2"}, el("b", {text: String(interviews)}), interviews === 1 ? " reached an interview" : " reached interviews") : null));
    }

    // ----- New matches, or recent searches when there are no target roles -----
    let matchSeq = 0;
    function renderMatches(p, force = false) {
      const mine = ++matchSeq;
      const section = parts.matches;
      const targets = (p?.target_positions || []).filter(t => String(t).trim());
      const searches = recentSearches(3);
      const head = aside => el("div", {class: "paper-head"}, el("h2", {id: "home-matches-h", text: targets.length ? "New for you" : "Search again"}),
        aside ? el("span", {class: "aside", text: aside}) : null);
      const searchRows = () => searches.length ? el("div", {class: "ruled"}, searches.map(s => el("button", {type: "button", class: "home3-row", onclick: s.onclick},
        el("span", {}, el("b", {text: s.title}), el("span", {class: "ink-2", text: s.detail})), el("span", {class: "ink-2 num", text: s.aside}))))
        : el("p", {class: "ink-2 home3-empty", text: "Searches you run show up here, so you can run them again."});
      if (!targets.length) {
        fill(section, head(""), searchRows(),
          el("p", {class: "ink-2 home3-note"}, "Add target roles to your ", el("a", {href: "#", text: "profile", onclick: e => { e.preventDefault(); openResume({field: "target_positions"}); }}), " and new matches show up here."));
        return;
      }
      const key = matchKey(p);
      const retry = (waitSeconds = 0) => {
        const button = el("button", {type: "button", class: "btn btn-text", text: "Search again", onclick: () => renderMatches(p, true)});
        if (waitSeconds > 0) {
          let left = waitSeconds;
          button.disabled = true;
          button.textContent = `Search again in ${left}s`;
          const tick = setInterval(() => {
            left--;
            if (!button.isConnected || left <= 0) {
              clearInterval(tick);
              button.disabled = false;
              button.textContent = "Search again";
              return;
            }
            button.textContent = `Search again in ${left}s`;
          }, 1000);
        }
        return button;
      };
      const RATE_WAIT = 30;
      const matchRow = job => el("button", {type: "button", class: "home3-row home3-match", title: window.Places.summary(job.location).full,
        onclick: () => { openJobs(); window.JobParser.openListing(job); }},
        el("span", {class: "tile", "aria-hidden": "true", text: (job.company_name || "?").charAt(0).toUpperCase()}),
        el("span", {},
          el("b", {text: job.position_title || "Unknown position"}),
          job.fit?.missing?.[0] ? el("span", {class: "tag tag-urgent"}, "missing: ", el("span", {translate: "no", text: job.fit.missing[0]})) : null,
          el("span", {class: "ink-2", text: [job.company_name, window.Places.summary(job.location).short, payLine(job)].filter(Boolean).join(" · ")})),
        job.fit ? el("span", {class: "home3-pc num", text: `${job.fit.percent}%`}) : null);
      const show = (items, animate = false) => {
        if (mine !== matchSeq) return;
        const note = items.length ? null
          : items.nothingCached ? "Run a search and matches show up here. Home reuses what that search loaded, so it never adds load on the job boards."
          : items.rateLimited ? "Job boards are limiting requests right now. Try again in a minute or two."
          : `No strong matches for ${matchScope(p)} right now.`;
        fill(section, head(matchScope(p)),
          items.length ? el("div", {class: "ruled"}, items.slice(0, 3).map(matchRow)) : el("p", {class: "ink-2 home3-empty", text: note}),
          items.length ? null : retry(items.rateLimited && !items.length ? RATE_WAIT : 0));
        if (animate) window.UI?.stagger?.(section.querySelector(".ruled") || section, 55, 6);
      };
      if (!force && matchCache && matchCache.key === key && Date.now() - matchCache.at < MATCH_TTL) return show(matchCache.items);
      fill(section, head(matchScope(p)), el("p", {class: "ink-2 home3-empty", text: "Looking for roles that match…"}));
      if (!matchLoad || matchLoad.key !== key || force) matchLoad = {key, promise: findMatches(p)};
      const load = matchLoad;
      load.promise.then(items => {
        if (items.length || !(items.rateLimited || items.nothingCached)) matchCache = {key, at: Date.now(), items};
        else matchLoad = null;
        show(items, true);
      }, error => {
        if (matchLoad === load) matchLoad = null;
        if (mine !== matchSeq) return;
        const limited = /limiting/i.test(error?.message || "");
        fill(section, head(matchScope(p)), el("p", {class: "ink-2 home3-empty", text: limited
          ? "Job boards are limiting requests right now. Search again when the countdown ends."
          : `Couldn't look for matches just now. ${error?.message || ""}`.trim()}), retry(limited ? RATE_WAIT : 0));
      });
    }

    const refreshIfVisible = () => {
      if (!container.hidden) refresh();
    };
    const onDataChange = event => {
      const detail = event?.detail || {};
      if (detail.invalidateMatches !== false) invalidateMatches();
      if (detail.clearListing) clearLastListing();
      refreshIfVisible();
    };
    window.addEventListener("fistbump:history", () => {
      invalidateMatches();
      refreshIfVisible();
    });
    window.addEventListener("fistbump:versions", refreshIfVisible);
    window.addEventListener("fistbump:jobs", onDataChange);
    window.addEventListener("fistbump:applications", onDataChange);
    window.addEventListener("fistbump:storage-cleaned", onDataChange);
    window.addEventListener("storage", event => {
      if (!event.key || event.key === HISTORY_KEY || event.key === LISTING_KEY) {
        invalidateMatches();
        refreshIfVisible();
      }
    });

    window.Home.refresh = refresh;
    refresh();
    return refresh;
  }

  window.Home = {mount, recentSearches, openResume, resumeGaps, invalidate: invalidateMatches};
})();

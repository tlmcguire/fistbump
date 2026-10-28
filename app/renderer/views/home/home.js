// The Home view: a greeting, the listing that was open last, what the master resume still lacks,
// saved versions and recent searches. Each row opens the place it names in the Jobs or Resume view.
// It reads the same data those views keep, so it is drawn again every time it is shown.
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

  //"Today", "Yesterday" or "Oct 24"
  const day = iso => {
    const then = new Date(iso);
    if (!iso || Number.isNaN(then.getTime())) return "";
    const start = d => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
    const days = Math.round((start(new Date()) - start(then)) / 86400000);
    if (days === 0) return "Today";
    if (days === 1) return "Yesterday";
    return then.toLocaleDateString(undefined, {month: "short", day: "numeric"});
  };

  const greeting = () => {
    const hour = new Date().getHours();
    return hour < 12 ? "Good morning" : hour < 18 ? "Good afternoon" : "Good evening";
  };

  //A row is one button: what it is, a detail line, something short on the right, and an arrow.
  const chevron = () => {
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("viewBox", "0 0 16 16");
    svg.setAttribute("aria-hidden", "true");
    const path = document.createElementNS(ns, "path");
    path.setAttribute("d", "M6 3.5 10.5 8 6 12.5");
    svg.append(path);
    return svg;
  };
  const row = ({title, detail, aside, onclick}) =>
    el("button", {type: "button", class: "home-row", onclick},
      el("div", {}, el("strong", {text: title}), detail ? el("span", {text: detail}) : null),
      el("small", {text: aside || ""}),
      chevron());
  const rows = items => el("ul", {class: "home-rows"}, items.map(item => el("li", {}, row(item))));

  // ---------- Time away ----------

  //When Fistbump was last open. It is read once, when the app starts, so Home can welcome someone back
  //for the rest of that run, and then kept current while the app is open.
  const SEEN_KEY = "fistbump.last-seen";
  const AWAY = [
    {days: 180, pose: "05-six-months-plus", text: "It's been over six months. Pico dusted itself off for you."},
    {days: 90, pose: "04-three-months", text: "It's been about three months since you were here."},
    {days: 30, pose: "03-one-month", text: "It's been about a month since you were here."},
    {days: 14, pose: "02-two-weeks", text: "It's been a couple of weeks since you were here."},
    {days: 7, pose: "01-one-week", text: "It's been a week since you were here."},
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

  //Pico, the mascot. Home shows still poses only.
  const MASCOTS = "views/graphics/mascots";
  const MONTHS = ["01-january", "02-february", "03-march", "04-april", "05-may", "06-june",
    "07-july", "08-august", "09-september", "10-october", "11-november", "12-december"];
  //The away and monthly poses keep the art's full canvas, so they are drawn a little larger to match.
  const pico = src => el("img", {class: src.startsWith("home/") ? "mascot" : "mascot full", src: `${MASCOTS}/${src}.png`, alt: ""});
  //A small pose that cheers on the last step or two of the resume.
  const nudge = () => el("img", {class: "mascot home-nudge", src: `${MASCOTS}/home/03-youve-got-this.png`, alt: ""});
  //New users meet Pico first. The picture carries its own words, so they are its alt text.
  const intro = () => el("img", {
    class: "home-intro",
    src: `${MASCOTS}/intro/meet-pico.png`,
    alt: "Meet Pico, your job-search sidekick. Here to help you keep going.",
  });

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

  //"Your name, contact details and 2 jobs are in."
  function resumeDone(master) {
    const p = master.profile;
    const done = [];
    if (p.full_name) done.push("your name");
    if (p.email || p.phone) done.push("contact details");
    if (p.summary) done.push("a summary");
    if (master.experiences.length) done.push(master.experiences.length === 1 ? "1 job" : `${master.experiences.length} jobs`);
    if (master.education.length) done.push(master.education.length === 1 ? "a school" : `${master.education.length} schools`);
    if (!done.length) return "";
    const list = done.length === 1 ? done[0] : `${done.slice(0, -1).join(", ")} and ${done.at(-1)}`;
    const verb = done.length > 1 || done[0].endsWith("s") ? "are" : "is";
    return `${list[0].toUpperCase()}${list.slice(1)} ${verb} in.`;
  }

  // ---------- Recent searches ----------

  //The search history the Jobs view keeps, described in a line: what, then where and how.
  function recentSearches(limit = 4) {
    const list = readJSON(HISTORY_KEY);
    if (!Array.isArray(list)) return [];
    return list.filter(entry => entry?.fields && typeof entry.fields === "object").slice(0, limit).map(entry => {
      const f = entry.fields;
      const text = id => String(f[id] ?? "").trim();
      const title = text("search-keywords") || (text("search-company") && `Jobs at ${text("search-company")}`) || entry.industry || "All jobs";
      const detail = [
        text("search-location"),
        text("search-keywords") && text("search-company") && `at ${text("search-company")}`,
        text("search-work-mode") && text("search-work-mode") !== "Any" && text("search-work-mode"),
        text("search-keywords") && entry.industry,
      ].filter(Boolean).join(" · ") || "Anywhere";
      return {title, detail, aside: day(entry.at), onclick: () => {
        openJobs();
        window.JobParser.runSearch(f);
      }};
    });
  }

  // ---------- The view ----------

  function mount(container) {
    const introSlot = el("div", {class: "home-intro-slot"});
    const welcome = el("section", {class: "home-welcome", "aria-labelledby": "home-greeting"});
    const resumeSection = el("section", {class: "home-section", "aria-labelledby": "home-resume-h"});
    const versionsSection = el("section", {class: "home-section", "aria-labelledby": "home-versions-h"});
    const searchSection = el("section", {class: "home-section", "aria-labelledby": "home-search-h"});
    const home = el("div", {class: "home"}, introSlot, welcome,
      el("div", {class: "home-columns"}, resumeSection, versionsSection), searchSection);

    container.replaceChildren(
      el("header", {class: "toolbar drag"}, el("div", {class: "toolbar-form"}, el("h1", {class: "view-title", text: "Home"}))),
      el("div", {class: "pane home-pane"}, home));

    let seq = 0;
    async function refresh() {
      const mine = ++seq;
      const [master, versions] = await Promise.all([
        api().resume.get().catch(() => null),
        api().resumes.list().catch(() => []),
      ]);
      if (mine !== seq) return;
      render(master || {profile: null, experiences: [], education: []}, versions || []);
    }

    function render(master, versions) {
      const p = master.profile;
      const gaps = resumeGaps(master);
      //A listing remembered before its text was kept cannot be shown without Greenhouse, so it is not offered.
      const kept = readJSON(LISTING_KEY);
      const listing = typeof kept?.raw_text === "string" ? kept : null;
      const today = new Date().toLocaleDateString(undefined, {weekday: "long", month: "long", day: "numeric"});
      const firstName = (p?.full_name || "").split(/\s+/)[0];

      // Welcome: the greeting, where things stand, and the one thing worth doing next.
      // A new user meets Pico above it; after that Pico sits beside the greeting.
      if (!p) {
        introSlot.hidden = false;
        if (!introSlot.firstChild) fill(introSlot, intro());
        fill(welcome,
          el("div", {class: "home-welcome-text"},
            el("h2", {id: "home-greeting", text: "Welcome to Fistbump"}),
            el("p", {text: "Start with your master resume. Every version you tailor for a job begins from it, and you can import one you already have."}),
            row({title: "Start your master resume", detail: "Import a file, paste text or fill it in", aside: "Begin", onclick: () => openResume({field: "full_name"})})));
      } else {
        introSlot.hidden = true;
        const status = gaps.length === 0 ? "Your resume is ready to tailor."
          : gaps.length <= 2 ? "Your resume is nearly there."
          : "Your resume has a few gaps left to fill.";
        const then = listing ? " The listing you were reading is right where you left it." : "";
        const hello = away ? "Welcome back" : greeting();
        fill(welcome,
          el("div", {class: "home-welcome-text"},
            el("h2", {id: "home-greeting", text: firstName ? `${hello}, ${firstName}` : hello}),
            el("p", {text: `${today}. ${away ? `${away.text} ` : ""}${status}${then}`}),
            listing ? row({
              title: [listing.position_title || "Unknown position", listing.company_name].filter(Boolean).join(", "),
              detail: [listing.location, listing.work_mode].filter(Boolean).join(" · "),
              aside: "Keep reading",
              onclick: () => {
                openJobs();
                window.JobParser.openListing(listing);
              },
            }) : row({title: "Find jobs", detail: "Search Greenhouse boards by title, place or company", aside: "Search", onclick: () => window.App.openTab("tab-find")})),
          //After time away, a pose that shows how long it has been; then a small win once the resume is
          //ready; otherwise a cozy break in the evening, or the pose for this month.
          pico(away ? `away/${away.pose}`
            : gaps.length === 0 ? "home/04-little-victory"
            : new Date().getHours() >= 18 ? "home/02-cozy-break"
            : `monthly/${MONTHS[new Date().getMonth()]}`));
      }

      // Finish your resume
      const done = p ? resumeDone(master) : "";
      fill(resumeSection,
        p && gaps.length > 0 && gaps.length <= 2 ? nudge() : null,
        el("h3", {id: "home-resume-h", text: gaps.length || !p ? "Finish your resume" : "Your resume"}),
        !p ? el("p", {text: "Nothing here yet. Your profile, jobs and schools will show up as you add them."})
          : gaps.length ? el("p", {text: `${done ? `${done} ` : ""}${gaps.length === 1 ? "One thing left:" : `${gaps.length} things left:`}`})
          : el("p", {text: `${done} Nothing is missing.`}),
        gaps.length ? rows(gaps.map(g => ({...g, onclick: () => openResume(g.target)}))) : null);

      // Saved versions
      fill(versionsSection,
        el("h3", {id: "home-versions-h", text: "Saved versions"}),
        el("p", {text: versions.length ? "Copies of your resume you can come back to." : "None yet. Save a version from the Resume view to keep it as it is now."}),
        versions.length ? rows(versions.slice(0, 3).map(v => ({
          title: v.version_label,
          detail: v.target_position ? `For ${v.target_position} roles` : "",
          aside: day(v.created_at),
          onclick: () => openResume({version: v.id}),
        }))) : null);

      // Search again
      const searches = recentSearches();
      fill(searchSection,
        el("h3", {id: "home-search-h", text: "Search again"}),
        el("p", {text: searches.length ? "Your recent searches, with their filters." : "Searches you run in Find jobs show up here, so you can run them again."}),
        searches.length ? rows(searches) : null);
    }

    refresh();
    return refresh;
  }

  //The sidebar lists the same searches and opens versions the same way.
  window.Home = {mount, recentSearches, openResume};
})();

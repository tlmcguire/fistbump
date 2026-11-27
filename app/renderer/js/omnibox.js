// The omnibox in the bottom bar: one field for every way a job comes in.
//  - A job link (Greenhouse, Lever, Ashby or any https page) opens it, as the Find search bar does.
//  - Posting text (several lines, or a long paste) goes to Paste a job, ready to preview.
//  - Anything else is a search: it runs in Find jobs with the words typed.
// Cmd/Ctrl+K focuses it from anywhere. Text dropped on it is treated as a paste.
// Focusing it opens suggestions at once, with nothing typed: recent searches and the main ways in. Typing adds
// matching saved jobs. The panel opens above the box while the bar is at the bottom, below it when it is at the top.
(() => {
  const form = document.getElementById("omnibox");
  const input = document.getElementById("omnibox-input");
  const $ = id => document.getElementById(id);

  const isMac = window.api?.platform === "darwin";
  $("omnibox-key").textContent = isMac ? "⌘K" : "Ctrl K";

  const isDirectATSLink = text => {
    const t = text.trim();
    if (!t || /\s/.test(t)) return false;
    return /^(?:https?:\/\/)?(?:(?:job-)?boards\.greenhouse\.io\/|grnh\.se\/|jobs\.(?:eu\.)?lever\.co\/|jobs\.ashbyhq\.com\/)/i.test(t);
  };
  const looksLikeLink = text => {
    const t = text.trim();
    if (!t || /\s/.test(t)) return false;
    return /^https?:\/\//i.test(t) || isDirectATSLink(t)
      || /^(?:www\.)?(?:linkedin\.com|indeed\.com|glassdoor\.com|wellfound\.com|ziprecruiter\.com)\//i.test(t);
  };
  const siteNameFromURL = text => {
    const t = text.trim();
    try {
      const host = new URL(/^https?:\/\//i.test(t) ? t : `https://${t}`).hostname.replace(/^www\./i, "").toLowerCase();
      if (host === "linkedin.com" || host.endsWith(".linkedin.com")) return "LinkedIn";
      if (host === "indeed.com" || host.endsWith(".indeed.com")) return "Indeed";
      if (host === "glassdoor.com" || host.endsWith(".glassdoor.com")) return "Glassdoor";
      if (host.includes("workday")) return "Workday";
      if (host === "wellfound.com" || host === "angel.co") return "Wellfound";
      if (host === "ziprecruiter.com" || host.endsWith(".ziprecruiter.com")) return "ZipRecruiter";
      return host;
    } catch {
      return "this site";
    }
  };
  //A posting is long or spans lines; a search is a few words.
  const looksLikePosting = text => /\n/.test(text.trim()) || text.trim().length > 200;

  //Sets a field's value the way typing would, so the view's own listeners see it.
  const fill = (field, value) => {
    field.value = value;
    field.dispatchEvent(new Event("input", {bubbles: true}));
  };

  function runSearch(text) {
    $("tab-find").click();
    fill($("search-keywords"), text);
    $("search-form").requestSubmit();
  }

  function openPosting(text) {
    $("tab-paste").click();
    const urlInput = $("paste-url");
    if (urlInput) fill(urlInput, "");
    const area = $("paste-text");
    fill(area, text);
    area.focus();
    area.setSelectionRange(0, 0);
    area.scrollTop = 0;
  }

  function route(text) {
    const value = String(text || "");
    if (!value.trim()) return;
    if (looksLikePosting(value)) openPosting(value);
    else runSearch(value.trim()); //a link runs through the search bar too, which opens it
    input.value = "";
    close();
    input.blur();
  }

  // ---------- Suggestions ----------
  const panel = $("omnibox-panel");
  let items = [];
  let active = -1;
  let saved = null;      //saved jobs, read when the box is focused and kept for a short while
  let savedAt = 0;
  const loadSaved = async () => {
    if (saved && Date.now() - savedAt < 20000) return saved;
    try { saved = await window.FistBumpAPI.jobs.list(""); } catch { saved = saved || []; }
    savedAt = Date.now();
    return saved;
  };
  const svg = id => {
    const ns = "http://www.w3.org/2000/svg";
    const s = document.createElementNS(ns, "svg");
    s.setAttribute("class", "icon");
    s.setAttribute("aria-hidden", "true");
    const use = document.createElementNS(ns, "use");
    use.setAttribute("href", `#i-${id}`);
    s.append(use);
    return s;
  };
  const go = fn => () => { input.value = ""; close(); input.blur(); fn(); };

  function build(query) {
    const q = query.trim();
    const low = q.toLowerCase();
    const groups = [];
    if (q) {
      groups.push({title: "", rows: [looksLikeLink(q)
        ? (isDirectATSLink(q)
          ? {icon: "link", label: "Open this job link", detail: q, run: () => route(q)}
          : {icon: "paste", label: `Paste a job from ${siteNameFromURL(q)}`, detail: "Paste its content to save and analyze", run: () => route(q)})
        : {icon: "search", label: `Search job boards for \u201c${q}\u201d`, detail: "Greenhouse, Lever and Ashby", run: () => route(q)}]});
      const jobs = (saved || []).filter(j => `${j.position_title} ${j.company_name}`.toLowerCase().includes(low)).slice(0, 5);
      if (jobs.length) groups.push({title: "Your jobs", rows: jobs.map(j => ({icon: "bookmark", label: j.position_title, detail: j.company_name, own: true,
        run: go(() => window.JobPage.open(j.id))}))});
    }
    const recent = (window.Home?.recentSearches?.(6) || []).filter(r => !low || `${r.title} ${r.detail}`.toLowerCase().includes(low)).slice(0, q ? 3 : 5);
    if (recent.length) groups.push({title: "Recent searches", rows: recent.map(r => ({icon: "clock", label: r.title, detail: r.detail, run: go(r.onclick)}))});
    if (!q) groups.push({title: "Go to", rows: [
      {icon: "search", label: "Browse job boards", detail: "Search with filters for place, pay and fit", run: go(() => openFind())},
      {icon: "paste", label: "Paste a job posting", detail: "Save a posting to your jobs", run: go(() => window.App.openTab("tab-paste"))},
      {icon: "list", label: "Your jobs", detail: "Everything you have saved", run: go(() => { location.hash = "#/list"; })},
    ]});
    return groups;
  }

  function draw() {
    const groups = build(input.value);
    items = groups.flatMap(g => g.rows);
    active = input.value.trim() ? 0 : -1;
    panel.replaceChildren();
    let n = 0;
    for (const g of groups) {
      const section = document.createElement("div");
      section.className = "omnibox-group";
      section.setAttribute("role", "group");
      if (g.title) {
        const h = document.createElement("p");
        h.className = "omnibox-group-title";
        h.textContent = g.title;
        section.setAttribute("aria-label", g.title);
        section.append(h);
      }
      for (const row of g.rows) {
        const i = n++;
        const opt = document.createElement("div");
        opt.className = "omnibox-option";
        opt.id = `omnibox-option-${i}`;
        opt.setAttribute("role", "option");
        const text = document.createElement("span");
        const b = document.createElement("b");
        b.textContent = row.label;
        if (row.own) b.setAttribute("translate", "no");
        const d = document.createElement("span");
        d.textContent = row.detail || "";
        if (row.own) d.setAttribute("translate", "no");
        text.append(b, d);
        opt.append(svg(row.icon), text);
        opt.addEventListener("pointerdown", event => event.preventDefault()); //keeps focus in the box
        opt.addEventListener("click", () => row.run());
        opt.addEventListener("pointermove", () => mark(i));
        section.append(opt);
      }
      panel.append(section);
    }
    mark(active);
    panel.hidden = !items.length;
    input.setAttribute("aria-expanded", String(!panel.hidden));
  }

  function mark(i) {
    active = i;
    panel.querySelectorAll(".omnibox-option").forEach((o, k) => o.setAttribute("aria-selected", String(k === i)));
    if (i >= 0) {
      input.setAttribute("aria-activedescendant", `omnibox-option-${i}`);
      panel.querySelector(`#omnibox-option-${i}`)?.scrollIntoView({block: "nearest"});
    } else input.removeAttribute("aria-activedescendant");
  }

  function close() {
    panel.hidden = true;
    input.setAttribute("aria-expanded", "false");
    input.removeAttribute("aria-activedescendant");
  }

  input.addEventListener("focus", async () => {
    draw();
    await loadSaved();
    if (document.activeElement === input) draw();
  });
  input.addEventListener("input", draw);
  input.addEventListener("blur", close);
  //closed with Esc while the box keeps focus: a click opens the suggestions again (the arrow keys do too, below)
  input.addEventListener("click", () => { if (panel.hidden) draw(); });

  form.addEventListener("submit", event => {
    event.preventDefault();
    if (active >= 0 && items[active] && !panel.hidden) items[active].run();
    else route(input.value);
  });

  //A search field drops line breaks, so a pasted posting is caught before it is flattened.
  input.addEventListener("paste", event => {
    const text = event.clipboardData?.getData("text/plain") || "";
    if (looksLikePosting(text)) {
      event.preventDefault();
      route(text);
    }
  });

  form.addEventListener("dragover", event => {
    if ([...(event.dataTransfer?.types || [])].includes("text/plain")) form.classList.add("dropping");
  });
  form.addEventListener("dragleave", () => form.classList.remove("dropping"));
  form.addEventListener("drop", event => {
    form.classList.remove("dropping");
    const text = event.dataTransfer?.getData("text/plain") || "";
    if (text) route(text);
  });

  input.addEventListener("keydown", event => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      if (panel.hidden) draw();
      if (!items.length) return;
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      mark(active < 0 ? (step > 0 ? 0 : items.length - 1) : (active + step + items.length) % items.length);
    } else if (event.key === "Escape") {
      event.preventDefault();
      if (!panel.hidden) close(); //first Esc closes the suggestions, a second clears and leaves the box
      else { input.value = ""; input.blur(); }
    }
  });

  //The full search page: Find with its filters, the keywords box ready to type in.
  function openFind() {
    window.App.openTab("tab-find");
    setTimeout(() => $("search-keywords")?.focus(), 0);
  }
  $("nav-search").addEventListener("click", openFind);

  document.addEventListener("keydown", event => {
    if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && event.key.toLowerCase() === "k") {
      event.preventDefault();
      input.focus();
      input.select();
    }
  });

  window.Omnibox = {route, looksLikeLink, looksLikePosting, openFind, focus: () => input.focus()};
})();

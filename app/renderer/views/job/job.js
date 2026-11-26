// The Job page (v3): one saved job, everything about it in one place. Where it stands (a stage path), the one
// next step (a primary button that follows the stage), what the posting asks for against the resume (with the
// experience that shows each skill), pay and cost of living, notes, and the posting itself.
// Opened with window.JobPage.open(id); its route is #/job/<id>. See docs/mockups/job-v3.html and DESIGN.md.
(() => {
  const api = () => window.FistBumpAPI;

  const STAGES = [
    {key: "saved", label: "Saved"}, {key: "tailored", label: "Tailored"}, {key: "applied", label: "Applied"},
    {key: "interview", label: "Interview"}, {key: "offer", label: "Offer"},
  ];
  const STATUS_STAGE = {Applied: "applied", Interviewing: "interview", Offered: "offer"};
  const STATUSES = ["Saved", "Applied", "Interviewing", "Offered", "Rejected"];
  const STATUS_LABEL = {Saved: "Saved", Applied: "Applied", Interviewing: "Interviewing", Offered: "Offer", Rejected: "Not moving forward"};
  const ROUTE_BACK = {home: "Home", list: "Your jobs", jobs: "Search results", tracker: "Tracker", tailor: "Tailor"};

  const locale = () => window.I18n?.locale() || undefined;
  const shortDate = iso => {
    if (!iso) return "";
    const d = new Date(/^\d{4}-\d{2}-\d{2}$/.test(iso) ? `${iso}T12:00` : iso);
    return Number.isNaN(d.getTime()) ? "" : d.toLocaleDateString(locale(), {month: "short", day: "numeric"});
  };
  const ago = iso => {
    const then = new Date(iso);
    if (!iso || Number.isNaN(then.getTime())) return "";
    const days = Math.floor((Date.now() - then.getTime()) / 86400000);
    return days <= 0 ? "today" : days === 1 ? "yesterday" : days < 30 ? `${days} days ago` : `on ${shortDate(iso)}`;
  };
  const money = n => `$${Math.round(n / 1000)}k`;
  const payLine = j => j.pay_min && j.pay_max ? (j.pay_min === j.pay_max ? money(j.pay_min) : `${money(j.pay_min)}–${money(j.pay_max)}`)
    : j.pay_min || j.pay_max ? money(j.pay_min || j.pay_max) : "";
  const safeURL = url => {
    try {
      const u = new URL(url);
      return u.protocol === "https:" || u.protocol === "http:" ? u.href : "";
    } catch {
      return "";
    }
  };

  const ink = id => {
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("class", `ink ink-${id}`);
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS(ns, "use");
    use.setAttribute("href", `#ink-${id}`);
    svg.append(use);
    return svg;
  };

  //The page's CSP blocks style attributes, so positions are set through the CSSOM instead.
  const styled = (node, props) => {
    for (const [k, v] of Object.entries(props)) node.style.setProperty(k, v);
    return node;
  };

  const notify = (kind, detail = {}) => window.dispatchEvent(new CustomEvent(`fistbump:${kind}`, {detail}));

  window.JobPage = {
    jobId: null,
    from: "jobs",

    open(id) {
      const route = document.querySelector(".app")?.dataset.route;
      if (route && route !== "job") this.from = route;
      this.jobId = Number(id);
      const hash = `#/job/${this.jobId}`;
      if (location.hash !== hash) location.hash = hash;
      else window.App.go("job");
    },

    mount(container) {
      const {el, fill} = window.ResumeBuilder;
      container.classList.add("v3", "job3");
      const scroll = el("div", {class: "job3-scroll"});
      container.replaceChildren(scroll);

      let seq = 0;
      let current = null; //{job, app, rounds, pending, analysis, master}

      const idFromHash = () => Number((location.hash.match(/^#\/job\/(\d+)/) || [])[1]) || window.JobPage.jobId;

      async function refresh() {
        const id = idFromHash();
        window.JobPage.jobId = id;
        const mine = ++seq;
        if (!id) return fill(scroll, el("p", {class: "job3-missing", text: "No job is open."}));
        try {
          const [job, apps, rounds, master] = await Promise.all([
            api().jobs.get(id),
            api().applications.list({archived: "include"}).catch(() => []),
            api().revisions.list(id).catch(() => []),
            api().resume.get().catch(() => null),
          ]);
          const app = (apps || []).find(a => a.job_id === id) || null;
          const latest = (rounds || [])[0];
          let pending = 0;
          if (latest?.status === "done") {
            const full = await api().revisions.get(latest.id).catch(() => null);
            pending = full ? full.suggestions.filter(s => s.state === "pending").length : 0;
          }
          if (mine !== seq) return;
          current = {job, app, rounds: rounds || [], latest, pending, master};
          render();
          loadAnalysis(mine);
          loadCost(mine);
        } catch (error) {
          if (mine !== seq) return;
          fill(scroll, el("div", {class: "job3-grid"}, el("section", {class: "job3-hero"},
            backLink(), el("h1", {class: "display-xl", text: "This job could not be opened"}),
            el("p", {class: "on-canvas-2", text: error.message}))));
        }
      }

      function backLink() {
        const from = window.JobPage.from;
        const label = ROUTE_BACK[from] || "Back";
        return el("button", {type: "button", class: "btn btn-text job3-back", onclick: () => {
          if (from && from !== "job") window.App.go(from);
          else history.back();
        }}, `← ${label}`);
      }

      // The job's stage: the tracker's status once there is one, else Tailored when a round has finished.
      const stageOf = ({app, rounds}) => app && STATUS_STAGE[app.status] ? STATUS_STAGE[app.status]
        : rounds.some(r => r.status === "done") ? "tailored" : "saved";

      // ---------- Layout ----------
      let parts = {};
      function render() {
        parts = {
          hero: el("section", {class: "job3-hero", "aria-labelledby": "job3-title"}),
          decide: el("section", {class: "paper job3-decide", "aria-label": "Next step"}),
          path: el("section", {class: "paper job3-path", "aria-labelledby": "job3-path-h"}),
          req: el("section", {class: "paper job3-req", "aria-labelledby": "job3-req-h"}),
          side: el("div", {class: "job3-side"}),
          pay: el("section", {class: "paper job3-pay", "aria-labelledby": "job3-pay-h", hidden: true}),
          notes: el("section", {class: "paper job3-notes", "aria-labelledby": "job3-notes-h"}),
          posting: el("section", {class: "paper job3-posting", "aria-labelledby": "job3-posting-h"}),
        };
        parts.side.append(parts.pay, parts.notes);
        fill(scroll, el("div", {class: "job3-grid"}, parts.hero, parts.decide, parts.path, parts.req, parts.side, parts.posting));
        fillHero();
        fillDecide();
        fillPath();
        fill(parts.req, el("div", {class: "paper-head"}, el("h2", {id: "job3-req-h", text: "What they're asking for"})),
          el("p", {class: "ink-2", text: "Comparing with your resume…"}));
        fillNotes();
        fillPosting();
        window.UI?.stagger?.(scroll.querySelector(".job3-grid"), 40, 6);
      }

      // ---------- Title and facts ----------
      function fillHero() {
        const {job} = current;
        const words = (job.position_title || "Untitled job").trim().split(/\s+/);
        const lastWord = words.pop();
        const facts = [
          el("span", {class: "fact", translate: "no"}, el("b", {text: job.company_name || "Unknown company"})),
          [window.Places?.summary(job.location).short || job.location, job.work_mode].filter(Boolean).length
            ? el("span", {class: "fact", text: [window.Places?.summary(job.location).short || job.location, job.work_mode].filter(Boolean).join(" · ")}) : null,
          payLine(job) ? el("span", {class: "fact"}, el("b", {text: payLine(job)})) : null,
          job.created_at ? el("span", {class: "fact", text: `Saved ${ago(job.created_at)}`}) : null,
          job.close_date ? el("span", {class: "fact", text: `Closes ${shortDate(job.close_date)}`}) : null,
        ];
        const state = job.trashed_at ? "This job is in the Trash. Restore it to keep it."
          : job.archived_at ? "This job is archived: hidden from Saved jobs, nothing deleted." : "";
        fill(parts.hero,
          backLink(),
          el("h1", {id: "job3-title", class: "display-xl", translate: "no"},
            words.length ? `${words.join(" ")} ` : "", el("span", {class: "underlined"}, lastWord, ink("underline"))),
          el("div", {class: "job3-facts"}, facts),
          state ? el("p", {class: "job3-state", text: state}) : null);
      }

      // ---------- The one next step ----------
      function nextStep() {
        const {job, app, latest, pending} = current;
        const stage = stageOf(current);
        if (job.trashed_at) return {label: "Restore job", run: () => act(() => api().jobs.restore(job.id))};
        if (app?.status === "Rejected") return {label: job.archived_at ? "Unarchive" : "Archive job",
          run: () => act(() => job.archived_at ? api().jobs.unarchive(job.id) : api().jobs.archive(job.id, {archive_application: true}))};
        if (stage === "offer") return {label: "Compare offers", run: () => window.Tracker.open(app.id)};
        if (stage === "interview") return {label: "Prep for the interview", run: () => window.Tracker.open(app.id)};
        if (stage === "applied") return {label: app.next_step_date ? "Change follow-up date" : "Set a follow-up date", run: () => editNextStep()};
        if (latest && ["queued", "running"].includes(latest.status)) return {label: "See suggestions as they arrive", run: () => window.Tailor.open(job.id)};
        if (pending) return {label: pending === 1 ? "Review 1 change" : `Review ${pending} changes`, run: () => window.Tailor.open(job.id)};
        if (stage === "tailored") return {label: "Mark as applied", run: () => setStatus("Applied")};
        return {label: "Tailor resume", run: () => window.Tailor.open(job.id)};
      }

      function fillDecide(fit = null) {
        const {job, app, pending} = current;
        const step = nextStep();
        const stage = stageOf(current);
        const sub = fit === null ? "Comparing with your resume…"
          : fit.total ? `${fit.have} of ${fit.total} required skills are on your resume` : "This posting lists no required skills to compare";
        const actions = [
          stage === "tailored" && !pending ? el("button", {type: "button", class: "btn", text: "Open tailored resume", onclick: () => window.Tailor.open(job.id)}) : null,
          app ? el("button", {type: "button", class: "btn", text: "Open in tracker", onclick: () => window.Tracker.open(app.id)}) : null,
          safeURL(job.listing_url || job.company_url) ? el("a", {class: "btn", href: safeURL(job.listing_url || job.company_url), target: "_blank", rel: "noopener noreferrer", text: "Open the listing"}) : null,
          job.trashed_at ? null : job.archived_at
            ? el("button", {type: "button", class: "btn", text: "Unarchive", onclick: () => act(() => api().jobs.unarchive(job.id))})
            : el("button", {type: "button", class: "btn", text: "Archive", onclick: () => act(() => api().jobs.archive(job.id, {archive_application: Boolean(app)}), {leftList: true})}),
          job.trashed_at ? null : el("button", {type: "button", class: "btn job3-danger", text: app ? "Move to Trash" : "Move to Trash", onclick: trash}),
          el("button", {type: "button", class: "btn job3-danger", text: "Delete now", onclick: remove}),
        ].filter(Boolean);
        fill(parts.decide,
          el("div", {class: "job3-fit"},
            el("span", {class: "job3-fit-n num", text: fit === null ? "—" : fit.total ? `${fit.percent}%` : "n/a"}),
            el("span", {class: "job3-fit-l", text: "fit"})),
          el("p", {class: "ink-2 job3-fit-sub", text: sub}),
          el("button", {type: "button", class: "btn btn-primary job3-go", text: step.label, onclick: step.run}),
          el("details", {class: "job3-more"}, el("summary", {class: "btn btn-text", text: "More actions"}),
            el("div", {class: "job3-more-list"}, actions)));
      }

      // ---------- Where it stands ----------
      function fillPath() {
        const {job, app} = current;
        const stage = stageOf(current);
        const at = STAGES.findIndex(s => s.key === stage);
        const rejected = app?.status === "Rejected";
        const note = key => key === "saved" ? shortDate(job.created_at)
          : key === "tailored" ? (current.pending ? (current.pending === 1 ? "1 change to decide" : `${current.pending} changes to decide`) : current.latest?.status === "done" ? "Ready" : "")
          : key === "applied" ? (app?.date_applied ? shortDate(app.date_applied) : stage === "tailored" ? "After you send it" : "")
          : key === "interview" && stage === "interview" && app?.next_step_date ? shortDate(app.next_step_date) : "";
        const select = el("select", {class: "job3-stage-select", "aria-label": "Change stage", onchange: e => setStatus(e.target.value)},
          !app ? el("option", {value: "", text: "Not tracked"}) : null,
          STATUSES.map(s => el("option", {value: s, text: STATUS_LABEL[s], selected: app?.status === s ? true : undefined})));
        fill(parts.path,
          el("div", {class: "paper-head"}, el("h2", {id: "job3-path-h", text: "Where this job stands"}),
            el("label", {class: "job3-stage-change"}, el("span", {class: "ink-2", text: "Stage"}), select)),
          styled(el("ol", {class: rejected ? "job3-stops closed" : "job3-stops"},
            STAGES.map((s, i) => {
              //a job can be applied to without a tailored version; that stop is passed, not done
              const skipped = s.key === "tailored" && i < at && !current.rounds.some(r => r.status === "done");
              return el("li", {class: ["job3-stop", i < at && !skipped && "done", skipped && "skipped", i === at && !rejected && "now"].filter(Boolean).join(" "), "aria-current": i === at ? "step" : undefined},
                el("span", {class: "job3-dot", "aria-hidden": "true"}, i < at && !skipped ? ink("check") : null),
                el("b", {text: s.label}),
                el("span", {class: "ink-2", text: skipped ? "Skipped" : note(s.key) || "—"}));
            })), {"--done": String(Math.max(at, 0) / (STAGES.length - 1))}),
          rejected ? el("p", {class: "job3-closed-note"}, el("span", {class: "tag", text: "Not moving forward"}), " Archive it from More actions when you're ready.") : null);
      }

      // ---------- Requirements against the resume ----------
      async function loadAnalysis(mine) {
        const {job, master} = current;
        try {
          const a = await api().jobs.analyze(job.id);
          if (mine !== seq) return;
          const titles = Object.fromEntries((master?.experiences || []).map(x => [x.id, [x.job_title, x.company_name].filter(Boolean).join(", ")]));
          const req = a.required || {}, pref = a.preferred || {};
          const have = (req.tech?.matched || []).length;
          const likely = (req.tech?.likely || []).length;
          const total = have + likely + (req.tech?.missing || []).length;
          fillDecide({have, total, percent: Math.round((a.score || 0) * 100)});
          const row = (mark, name, detail, action) => el("div", {class: "job3-ri"}, ink(mark),
            el("div", {}, el("b", {translate: "no", text: name}), detail ? el("span", {class: "ink-2", text: detail}) : null),
            action || null);
          const evidence = m => {
            const where = (m.experience_ids || []).map(id => titles[id]).filter(Boolean);
            return where.length ? `On your resume: ${where.slice(0, 2).join("; ")}` : "Listed in your skills";
          };
          const add = () => el("button", {type: "button", class: "btn", text: "I've used it", onclick: () => {
            window.App.go("resume");
            window.ResumeBuilder.reveal?.({add: "experience"});
          }});
          const group = g => [
            ...(g.tech?.matched || []).map(m => row("check", m.skill, evidence(m))),
            ...(g.tech?.likely || []).map(m => row("half", m.skill || m, "Close: your resume shows something like it")),
            ...(g.tech?.missing || []).map(s => row("open", s, "Not on your resume", add())),
            ...(g.soft?.matched || []).map(m => row("check", m.skill, evidence(m))),
          ];
          const soft = [...(req.soft?.unmentioned || []), ...(req.soft?.missing || [])];
          const required = group(req);
          const nice = group(pref);
          fill(parts.req,
            el("div", {class: "paper-head"}, el("h2", {id: "job3-req-h", text: "What they're asking for"}),
              total ? el("span", {class: "aside", text: `${have} of ${total} on your resume`}) : null),
            required.length ? el("div", {class: "ruled job3-ris"}, required) : el("p", {class: "ink-2", text: "No required skills were found in this posting."}),
            soft.length ? el("p", {class: "job3-soft ink-2"}, "They also mention ", el("span", {translate: "no", text: soft.join(", ")}),
              ". Resumes rarely list these, so show them through results instead.") : null,
            nice.length ? [el("h3", {class: "job3-sub", text: "Nice to have"}), el("div", {class: "ruled job3-ris"}, nice)] : null);
        } catch (error) {
          if (mine !== seq) return;
          fillDecide({have: 0, total: 0, percent: 0});
          fill(parts.req, el("div", {class: "paper-head"}, el("h2", {id: "job3-req-h", text: "What they're asking for"})),
            el("p", {class: "ink-2", text: error.message}),
            error.code === "conflict" ? el("button", {type: "button", class: "btn", text: "Start your resume", onclick: () => window.App.go("resume")}) : null);
        }
      }

      // ---------- Pay and place ----------
      const PAY = {well_above: "Well above market", above: "Above market", in_line: "At market", below: "Below market", well_below: "Well below market"};
      const TONE = {well_above: "good", above: "good", in_line: "mark", below: "urgent", well_below: "urgent",
        comfortable: "good", okay: "mark", tight: "urgent", short: "urgent"};
      const LIFE = {comfortable: "Comfortable", okay: "Workable", tight: "Tight", short: "Short"};
      async function loadCost(mine) {
        const {job} = current;
        if (!job.raw_text && !job.pay_min && !job.position_title) return;
        try {
          const c = await api().cost.analyze({title: job.position_title || "", location: job.location || "", work_mode: job.work_mode || "",
            pay_min: job.pay_min || 0, pay_max: job.pay_max || 0, text: job.raw_text || ""});
          if (mine !== seq || !c) return;
          const p = c.pay;
          let scale = null;
          if (p?.market_low && p?.market_high) {
            const lo = Math.min(p.market_low, p.min || p.market_low) * 0.85;
            const hi = Math.max(p.market_high, p.max || p.market_high) * 1.1;
            const pos = n => `${((n - lo) / (hi - lo)) * 100}%`;
            scale = el("div", {class: "job3-scale-wrap"},
              el("div", {class: "job3-scale", "aria-hidden": "true"},
                styled(el("i"), {left: pos(p.market_low), width: `calc(${pos(p.market_high)} - ${pos(p.market_low)})`}),
                p.listed ? styled(el("em"), {left: pos(((p.min || 0) + (p.max || p.min || 0)) / 2)}) : null),
              el("p", {class: "ink-2 job3-scale-l"}, `Typical here: ${money(p.market_low)} to ${money(p.market_high)}`,
                p.listed ? el("span", {}, " · posted midpoint in gold") : null));
          }
          fill(parts.pay,
            el("div", {class: "paper-head"}, el("h2", {id: "job3-pay-h", text: "Pay and place"}),
              p?.verdict && PAY[p.verdict] ? el("span", {class: `tag tag-${TONE[p.verdict]}`, text: PAY[p.verdict]}) : null),
            payLine(job) ? el("div", {class: "job3-amt display", text: payLine(job)}) : null,
            scale,
            el("p", {class: "job3-pay-sum", text: c.summary}),
            c.lifestyle ? el("p", {class: "ink-2 job3-life"}, `Living in ${c.location?.label || "this area"}: `,
              el("span", {class: `tag tag-${TONE[c.lifestyle.verdict] || "mark"}`, text: LIFE[c.lifestyle.verdict] || c.lifestyle.verdict}),
              ` ${(c.lifestyle.headline || "").replace(/^[A-Za-z]+:\s*/, "")}`) : null, //the tag already says the verdict
            el("p", {class: "ink-2 job3-est", text: "Estimates from average figures, not live prices."}));
          parts.pay.hidden = false;
        } catch {
          //the rest of the page stands without it
        }
      }

      // ---------- Notes ----------
      function fillNotes() {
        const {app, job} = current;
        const area = el("textarea", {class: "job3-notes-area", rows: "5", "aria-labelledby": "job3-notes-h",
          placeholder: "Recruiter names, what to ask, how it went. Saved as you type."});
        area.value = app?.notes || "";
        const status = el("span", {class: "ink-2 job3-saved", "aria-live": "polite"});
        let timer = null;
        area.addEventListener("input", () => {
          clearTimeout(timer);
          status.textContent = "";
          timer = setTimeout(async () => {
            try {
              if (current.app) current.app = await api().applications.update(current.app.id, {notes: area.value});
              else {
                current.app = await api().applications.create({job_id: job.id, notes: area.value});
                notify("applications");
                fillPath();
                fillDecide(null);
                loadAnalysis(seq);
              }
              status.textContent = "Saved";
            } catch (error) {
              status.textContent = error.message;
            }
          }, 600);
        });
        fill(parts.notes,
          el("div", {class: "paper-head"}, el("h2", {id: "job3-notes-h", text: "Notes"}), status),
          area,
          !app ? el("p", {class: "ink-2 job3-notes-hint", text: "Writing a note starts tracking this job."}) : null);
      }

      // ---------- The posting ----------
      function fillPosting() {
        const {job} = current;
        const reqs = Array.isArray(job.requirements) ? job.requirements.filter(Boolean) : [];
        const text = (job.description || job.raw_text || "").trim();
        const body = el("div", {class: "job3-posting-text", text});
        const toggle = el("button", {type: "button", class: "btn btn-text", text: "Show the full posting", onclick: () => {
          const open = body.classList.toggle("open");
          toggle.textContent = open ? "Show less" : "Show the full posting";
        }});
        fill(parts.posting,
          el("div", {class: "paper-head"}, el("h2", {id: "job3-posting-h", text: "The posting"})),
          reqs.length ? el("ul", {class: "job3-reqs"}, reqs.slice(0, 8).map(r => el("li", {text: r}))) : null,
          text ? [body, text.length > 600 ? toggle : null] : el("p", {class: "ink-2", text: "No posting text was saved for this job."}));
      }

      // ---------- Actions ----------
      async function act(run, {leftList = false} = {}) {
        try {
          await run();
          notify("jobs");
          notify("applications");
          if (leftList) forgetListing();
          await refresh();
        } catch (error) {
          window.UI.alert(error.message);
        }
      }

      const forgetListing = () => {
        try {
          const kept = JSON.parse(localStorage.getItem("fistbump.last-listing") || "null");
          if (kept?.id === current.job.id) localStorage.removeItem("fistbump.last-listing");
        } catch {}
      };

      async function setStatus(status) {
        const {job, app} = current;
        if (!status) return;
        try {
          current.app = app ? await api().applications.update(app.id, {status}) : await api().applications.create({job_id: job.id, status});
          notify("applications");
          await refresh();
        } catch (error) {
          const id = error.details?.application_id;
          if (id) return window.Tracker.open(id);
          window.UI.alert(error.message);
        }
      }

      function editNextStep() {
        const {app} = current;
        const input = el("input", {type: "date", class: "job3-date", value: app?.next_step_date || "", "aria-label": "Follow-up date"});
        const save = el("button", {type: "button", class: "btn btn-primary", text: "Save date", onclick: async () => {
          try {
            current.app = await api().applications.update(app.id, {next_step_date: input.value || null});
            notify("applications");
            await refresh();
          } catch (error) {
            window.UI.alert(error.message);
          }
        }});
        const go = parts.decide.querySelector(".job3-go");
        go.replaceWith(el("div", {class: "job3-date-row"}, input, save));
        input.focus();
      }

      async function trash() {
        const {job, app} = current;
        if (app) {
          if (!await window.UI.ask("This job has an application in the Tracker, so it cannot go to the Trash.\n\nArchive keeps the job hidden without deleting it, and can be undone. Archive it instead?",
            {confirm: "Archive it", title: "Cannot trash — job has an application"})) return;
          return act(() => api().jobs.archive(job.id, {archive_application: true}), {leftList: true});
        }
        if (!await window.UI.ask(`Move "${job.position_title}" at ${job.company_name} to the Trash?\n\nIt leaves Saved jobs now and is deleted for good later, unless you restore it before then.`,
          {confirm: "Move to Trash", danger: true, title: "Move to Trash"})) return;
        return act(() => api().jobs.trash(job.id), {leftList: true});
      }

      async function remove() {
        const {job} = current;
        if (!await window.UI.ask(`Delete "${job.position_title}" at ${job.company_name} now?\n\nThis cannot be undone. To keep the job for a while and delete it later, move it to the Trash instead.`,
          {confirm: "Delete now", danger: true, title: "Delete job"})) return;
        try {
          await api().jobs.delete(job.id);
          forgetListing();
          notify("jobs");
          window.App.go("list");
        } catch (error) {
          window.UI.alert(error.message);
        }
      }

      refresh();
      return refresh;
    },
  };
})();

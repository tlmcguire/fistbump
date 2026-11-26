// The Jobs list (v3): every job in one list, grouped by what needs you, filtered by stage. It replaces the Saved
// jobs list and the Tracker's table as the place to see everything; a row opens the Job page. Route: #/list.
// See docs/mockups/phase5/jobs-v3.html and DESIGN.md.
(() => {
  const api = () => window.FistBumpAPI;

  const STAGES = [
    {key: "all", label: "All"}, {key: "saved", label: "Saved"}, {key: "tailored", label: "Tailored"},
    {key: "applied", label: "Applied"}, {key: "interview", label: "Interview"}, {key: "offer", label: "Offer"},
    {key: "closed", label: "Closed"},
  ];
  const STAGE_LABEL = {saved: "Saved", tailored: "Tailored", applied: "Applied", interview: "Interview", offer: "Offer", closed: "Closed"};
  const APP_STAGE = {Applied: "applied", Interviewing: "interview", Offered: "offer"};
  const MOVE_TO = [["Saved", "Saved"], ["Applied", "Applied"], ["Interviewing", "Interview"], ["Offered", "Offer"], ["Rejected", "Not moving forward"]];
  const VIEW_KEY = "fistbump.jobs-list";

  const locale = () => window.I18n?.locale() || undefined;
  const pad = n => String(n).padStart(2, "0");
  const isoDay = d => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  const dayName = iso => new Date(`${iso}T12:00`).toLocaleDateString(locale(), {weekday: "short"});
  const shortDate = iso => new Date(`${iso}T12:00`).toLocaleDateString(locale(), {month: "short", day: "numeric"});
  const money = n => `$${Math.round(n / 1000)}k`;
  const payLine = j => j.pay_min && j.pay_max ? (j.pay_min === j.pay_max ? money(j.pay_min) : `${money(j.pay_min)}–${money(j.pay_max)}`)
    : j.pay_min || j.pay_max ? money(j.pay_min || j.pay_max) : "";
  const remote = j => /remote/i.test(`${j.work_mode || ""} ${j.location || ""}`);

  const readPrefs = () => {
    try { return JSON.parse(localStorage.getItem(VIEW_KEY) || "null") || {}; } catch { return {}; }
  };
  const writePrefs = v => {
    try { localStorage.setItem(VIEW_KEY, JSON.stringify(v)); } catch {}
  };

  window.JobsList = {
    mount(container) {
      const {el, fill} = window.ResumeBuilder;
      container.classList.add("v3", "jl3");
      const scroll = el("div", {class: "jl3-scroll"});
      container.replaceChildren(scroll);

      const prefs = Object.assign({stage: "all", remote: false, pay: false, fit: false, view: "list"}, readPrefs());
      let query = "";
      let entries = [];      //[{job, app, stage, pending, closed, fit}]
      const picked = new Set();
      let seq = 0;

      async function refresh() {
        const mine = ++seq;
        const [active, archived, trashed, apps] = await Promise.all([
          api().jobs.list("").catch(() => []),
          api().jobs.list("", {archived: "only"}).catch(() => []),
          api().jobs.list("", {trashed: "only"}).catch(() => []),
          api().applications.list({archived: "include"}).catch(() => []),
        ]);
        const appOf = new Map((apps || []).map(a => [a.job_id, a]));
        const all = [...(active || []).map(j => ({job: j, gone: ""})), ...(archived || []).map(j => ({job: j, gone: "archived"})),
          ...(trashed || []).map(j => ({job: j, gone: "trash"}))];
        //Saved vs Tailored, and changes waiting, need the job's rounds; only jobs not yet applied to are asked
        const rounds = await Promise.all(all.map(({job, gone}) => {
          const a = appOf.get(job.id);
          return gone || (a && (APP_STAGE[a.status] || a.status === "Rejected")) ? null : api().revisions.list(job.id).catch(() => []);
        }));
        const pendingOf = await Promise.all(rounds.map(async list => {
          const latest = list?.[0];
          if (latest?.status !== "done") return 0;
          const full = await api().revisions.get(latest.id).catch(() => null);
          return full ? full.suggestions.filter(s => s.state === "pending").length : 0;
        }));
        if (mine !== seq) return;
        entries = all.map(({job, gone}, i) => {
          const app = appOf.get(job.id) || null;
          const closed = Boolean(gone) || app?.status === "Rejected" || Boolean(app?.archived_at);
          const stage = closed ? "closed" : app && APP_STAGE[app.status] ? APP_STAGE[app.status]
            : rounds[i]?.some(r => r.status === "done") ? "tailored" : "saved";
          return {job, app, stage, gone, closed, pending: pendingOf[i] || 0, fit: null};
        });
        for (const id of [...picked]) if (!entries.some(e => e.job.id === id)) picked.delete(id);
        draw();
        loadFits(mine);
      }

      //fit is the same analysis the Job page shows; it fills in after the list is drawn
      async function loadFits(mine) {
        const open = entries.filter(e => !e.closed).slice(0, 60);
        const results = await Promise.all(open.map(e => api().jobs.analyze(e.job.id).catch(() => null)));
        if (mine !== seq) return;
        open.forEach((e, i) => { if (results[i]) e.fit = Math.round((results[i].score || 0) * 100); });
        for (const e of open) {
          const cell = scroll.querySelector(`[data-fit="${e.job.id}"]`);
          if (cell && e.fit !== null) cell.textContent = `${e.fit}%`;
        }
        if (prefs.fit) draw();
      }

      // What comes next for a job, in words, with how urgent it is.
      function nextOf(e) {
        const {job, app, stage, pending} = e;
        const today = isoDay(new Date());
        if (e.gone === "trash") return {text: "In the Trash", tone: ""};
        if (e.gone === "archived" || app?.archived_at) return {text: "Archived", tone: ""};
        if (app?.status === "Rejected") return {text: "Not moving forward", tone: ""};
        if (pending) return {text: pending === 1 ? "Review 1 change" : `Review ${pending} changes`, tone: "need", needs: true};
        if (app?.next_step_date && app.next_step_date <= today) {
          return {text: app.next_step_date < today ? "Next step overdue" : stage === "interview" ? "Interview today" : "Follow up today", tone: "urgent", needs: true};
        }
        if (stage === "interview") return {text: app?.next_step_date ? `Interview ${dayName(app.next_step_date)}` : "Interviewing", tone: "good"};
        if (stage === "offer") return {text: "Reply to the offer", tone: "need", needs: true};
        if (stage === "applied") {
          if (app?.next_step_date) return {text: `Follow up ${shortDate(app.next_step_date)}`, tone: ""};
          const days = app?.date_applied ? Math.floor((Date.now() - new Date(`${app.date_applied}T12:00`).getTime()) / 86400000) : 0;
          return {text: days > 0 ? `Waiting · ${days} day${days === 1 ? "" : "s"}` : "Applied", tone: ""};
        }
        const closes = (job.close_date || "").slice(0, 10);
        const soon = closes && closes >= today ? ` · closes ${shortDate(closes)}` : "";
        if (stage === "tailored") return {text: `Mark as applied${soon}`, tone: ""};
        return {text: `Tailor resume${soon}`, tone: ""};
      }

      function visible() {
        const q = query.trim().toLowerCase();
        return entries.filter(e => {
          if (prefs.view === "board" ? e.closed : prefs.stage === "all" ? e.closed : e.stage !== prefs.stage) return false;
          if (prefs.remote && !remote(e.job)) return false;
          if (prefs.pay && !(e.job.pay_min || e.job.pay_max)) return false;
          if (prefs.fit && !(e.fit !== null && e.fit >= 70)) return false;
          if (q && !`${e.job.position_title} ${e.job.company_name}`.toLowerCase().includes(q)) return false;
          return true;
        });
      }

      // ---------- Drawing ----------
      function draw() {
        const open = entries.filter(e => !e.closed);
        const counts = Object.fromEntries(STAGES.map(s => [s.key, s.key === "all" ? open.length : entries.filter(e => e.stage === s.key).length]));
        const list = visible();
        const withNext = list.map(e => ({...e, next: nextOf(e)}));
        const needs = withNext.filter(e => e.next.needs);
        const rest = withNext.filter(e => !e.next.needs);
        const month = isoDay(new Date()).slice(0, 7);
        const sent = entries.filter(e => (e.app?.date_applied || "").startsWith(month)).length;
        const interviews = open.filter(e => e.stage === "interview").length;
        const needCount = open.map(nextOf).filter(n => n.needs).length;

        const summary = el("p", {class: "jl3-sum"}, el("b", {text: open.length === 1 ? "1 active" : `${open.length} active`}),
          //each part is its own text, so each one matches its translation
          " · ", el("span", {text: needCount === 1 ? "1 needs you today" : `${needCount} need you today`}),
          " · ", el("span", {text: interviews === 1 ? "1 interview" : `${interviews} interviews`}),
          " · ", el("span", {text: sent === 1 ? "1 application this month" : `${sent} applications this month`}));

        const stageTabs = el("div", {class: "jl3-stages", role: "tablist", "aria-label": "Stage"}, STAGES.map(s => el("button", {
          type: "button", role: "tab", "aria-selected": String(prefs.stage === s.key),
          onclick: () => { prefs.stage = s.key; picked.clear(); writePrefs(prefs); draw(); },
        }, s.label, el("span", {class: "num", text: String(counts[s.key])}))));
        const quick = (key, label) => el("button", {type: "button", class: "jl3-quick", "aria-pressed": String(prefs[key]),
          onclick: () => { prefs[key] = !prefs[key]; writePrefs(prefs); draw(); }}, label);
        const filter = el("input", {type: "search", class: "jl3-filter", placeholder: "Filter by company or title", "aria-label": "Filter by company or title", value: query});
        filter.addEventListener("input", () => {
          query = filter.value;
          const at = filter.selectionStart;
          draw();
          const again = scroll.querySelector(".jl3-filter");
          again.focus();
          again.setSelectionRange(at, at);
        });

        const groups = [];
        if (!entries.length) {
          groups.push(el("div", {class: "jl3-empty"},
            el("b", {text: "No jobs yet"}),
            el("p", {class: "ink-2", text: "Search the job boards, paste a link, or paste a posting into the bar below. Every job you save shows up here."}),
            el("button", {type: "button", class: "btn btn-primary", text: "Find jobs", onclick: () => window.Omnibox?.focus()})));
        } else if (!list.length) {
          groups.push(el("p", {class: "jl3-none ink-2", text: prefs.stage === "closed" ? "Nothing closed. Archived jobs, the Trash and jobs that did not move forward show up here." : "No jobs match these filters."}));
        } else {
          if (needs.length && prefs.stage !== "closed") groups.push(group("Needs you", "Due first", needs));
          if (rest.length) groups.push(group(needs.length && prefs.stage !== "closed" ? "Everything else" : STAGES.find(s => s.key === prefs.stage).label, "", rest));
        }

        const viewSwitch = el("div", {class: "jl3-view", role: "radiogroup", "aria-label": "View"}, [["list", "List"], ["board", "Board"]].map(([v, label]) =>
          el("button", {type: "button", role: "radio", "aria-checked": String(prefs.view === v),
            onclick: () => { prefs.view = v; picked.clear(); writePrefs(prefs); draw(); }}, label)));
        const board = prefs.view === "board";

        fill(scroll, el("div", {class: "jl3-grid"},
          el("header", {class: "jl3-head"},
            el("h1", {class: "display-xl"}, "Your ", el("span", {class: "underlined"}, "jobs", ink("underline"))),
            summary),
          el("div", {class: "jl3-tools"}, board ? null : stageTabs,
            el("div", {class: "jl3-quicks"}, quick("remote", "Remote"), quick("pay", "Has pay"), quick("fit", "70%+ fit")),
            el("label", {class: "jl3-search"}, icon("search"), filter),
            el("button", {type: "button", class: "btn btn-mark jl3-add", text: "Add listing",
              title: "Submit a job listing from LinkedIn, Indeed, Workday, or another site",
              onclick: () => window.SubmitListing?.open()}),
            viewSwitch),
          board ? boardView(withNext) : el("section", {class: "paper jl3-list", "aria-label": "Jobs"}, groups,
            el("div", {class: "jl3-foot ruled-top"},
              el("span", {class: "ink-2", text: "Archived jobs, the Trash and jobs that did not move forward are under Closed."}),
              el("span", {}, el("a", {href: "#/tracker", text: "Tracker table, export and offer comparison"}))))));
      }

      // ---------- Board: one column per stage, moved with buttons, never by dragging ----------
      //Moving a card changes the tracker status. Tailored is not a status, so it moves on to Applied.
      const MOVES = {
        saved: {next: "Applied"}, tailored: {next: "Applied"},
        applied: {prev: "Saved", next: "Interviewing"}, interview: {prev: "Applied", next: "Offered"}, offer: {prev: "Interviewing"},
      };
      const STAGE_OF_STATUS = {Saved: "Saved", Applied: "Applied", Interviewing: "Interview", Offered: "Offer"};
      function boardView(list) {
        const cols = STAGES.filter(s => !["all", "closed"].includes(s.key));
        return el("section", {class: "jl3-board", "aria-label": "Jobs by stage"}, cols.map(s => {
          const cards = list.filter(e => e.stage === s.key);
          return el("div", {class: "jl3-col", "aria-labelledby": `jl3-col-${s.key}`},
            el("div", {class: "jl3-col-head"}, el("h2", {id: `jl3-col-${s.key}`, text: s.label}), el("span", {class: "num ink-2", text: String(cards.length)})),
            cards.length ? cards.map(card) : el("p", {class: "jl3-col-empty ink-2", text: s.key === "offer" ? "Your first offer goes here" : "Nothing here yet"}));
        }));
      }
      function card(e) {
        const {job, next} = e;
        const move = MOVES[e.stage] || {};
        const go = status => run([e], x => x.app ? api().applications.update(x.app.id, {status}) : api().applications.create({job_id: x.job.id, status}));
        return el("article", {class: "jl3-card"},
          el("button", {type: "button", class: "jl3-card-open", onclick: () => window.JobPage.open(job.id), "aria-label": `${job.position_title} at ${job.company_name}`},
            el("span", {class: e.stage === "interview" ? "tile tile-mark" : "tile", "aria-hidden": "true", text: (job.company_name || job.position_title || "?").trim().charAt(0).toUpperCase()}),
            el("span", {class: "jl3-main"}, el("b", {translate: "no", text: job.position_title || "Untitled job"}),
              el("span", {class: "ink-2", translate: "no", text: job.company_name || ""})),
            el("span", {class: "jl3-fit num", "data-fit": job.id, text: e.fit === null ? "" : `${e.fit}%`})),
          el("div", {class: "jl3-card-foot"},
            el("span", {class: `jl3-next ${next.tone}`.trim(), text: next.text}),
            el("span", {class: "jl3-moves"},
              move.prev ? el("button", {type: "button", class: "btn btn-text", "aria-label": `Move back to ${STAGE_OF_STATUS[move.prev]}`, title: `Move back to ${STAGE_OF_STATUS[move.prev]}`, onclick: () => go(move.prev)}, icon("chevron")) : null,
              move.next ? el("button", {type: "button", class: "btn btn-text", "aria-label": `Move on to ${STAGE_OF_STATUS[move.next]}`, title: `Move on to ${STAGE_OF_STATUS[move.next]}`, onclick: () => go(move.next)}, icon("chevron")) : null)));
      }

      function group(title, aside, list) {
        const ids = list.map(e => e.job.id);
        const some = ids.filter(id => picked.has(id)).length;
        const all = el("input", {type: "checkbox", class: "jl3-check", "aria-label": `Select all in ${title}`, checked: some === ids.length && ids.length > 0});
        all.indeterminate = some > 0 && some < ids.length;
        all.addEventListener("change", () => {
          for (const id of ids) all.checked ? picked.add(id) : picked.delete(id);
          draw();
        });
        const chosen = list.filter(e => picked.has(e.job.id));
        return el("div", {class: "jl3-group"},
          el("div", {class: "jl3-group-head"}, all, el("h2", {text: title}),
            chosen.length ? bulkBar(chosen) : aside ? el("span", {class: "aside ink-2", text: aside}) : null),
          list.map(row));
      }

      function row(e) {
        const {job, next} = e;
        const check = el("input", {type: "checkbox", class: "jl3-check", "aria-label": `Select ${job.position_title}`, checked: picked.has(job.id)});
        check.addEventListener("click", event => event.stopPropagation());
        check.addEventListener("change", () => { check.checked ? picked.add(job.id) : picked.delete(job.id); draw(); });
        const open = () => window.JobPage.open(job.id);
        return el("div", {class: picked.has(job.id) ? "jl3-row picked" : "jl3-row", "data-job": job.id},
          check,
          el("button", {type: "button", class: "jl3-open", onclick: open, "aria-label": `${job.position_title} at ${job.company_name}`},
            el("span", {class: e.stage === "interview" ? "tile tile-mark" : "tile", "aria-hidden": "true", text: (job.company_name || job.position_title || "?").trim().charAt(0).toUpperCase()}),
            el("span", {class: "jl3-main"}, el("b", {translate: "no", text: job.position_title || "Untitled job"}),
              el("span", {class: "ink-2", translate: "no", text: [job.company_name, window.Places?.summary(job.location).short || job.location, job.work_mode].filter(Boolean).join(" · ")})),
            el("span", {class: "jl3-stage", text: STAGE_LABEL[e.stage]}),
            el("span", {class: "jl3-fit num", "data-fit": job.id, text: e.fit === null ? "" : `${e.fit}%`}),
            el("span", {class: "jl3-pay ink-2 num", text: payLine(job) || "—"}),
            el("span", {class: `jl3-next ${next.tone}`.trim(), text: next.text})));
      }

      // ---------- Bulk actions ----------
      function bulkBar(chosen) {
        const n = chosen.length;
        const closed = chosen.every(e => e.closed);
        const move = el("select", {class: "jl3-move", "aria-label": `Move ${n} selected to a stage`},
          el("option", {value: "", text: "Move to stage"}), MOVE_TO.map(([v, label]) => el("option", {value: v, text: label})));
        move.addEventListener("change", () => move.value && run(chosen, e => e.app
          ? api().applications.update(e.app.id, {status: move.value})
          : api().applications.create({job_id: e.job.id, status: move.value})));
        return el("div", {class: "jl3-bulk"}, el("b", {text: `${n} selected`}),
          closed ? [
            el("button", {type: "button", class: "btn", text: "Restore", onclick: () => run(chosen, e => e.gone === "trash" ? api().jobs.restore(e.job.id) : e.gone === "archived" ? api().jobs.unarchive(e.job.id) : e.app?.archived_at ? api().applications.unarchive(e.app.id) : null)}),
            el("button", {type: "button", class: "btn job3-danger", text: "Delete now", onclick: () => removeForGood(chosen)}),
          ] : [
            move,
            el("button", {type: "button", class: "btn", text: "Archive", onclick: () => run(chosen, e => api().jobs.archive(e.job.id, {archive_application: Boolean(e.app)}))}),
            el("button", {type: "button", class: "btn job3-danger", text: "Trash", onclick: () => trash(chosen)}),
          ],
          el("button", {type: "button", class: "btn btn-text", text: "Clear", onclick: () => { picked.clear(); draw(); }}));
      }

      async function run(chosen, step) {
        const failed = [];
        for (const e of chosen) {
          try { await step(e); } catch (error) { failed.push(`${e.job.position_title}: ${error.message}`); }
        }
        picked.clear();
        window.dispatchEvent(new CustomEvent("fistbump:jobs", {detail: {}}));
        window.dispatchEvent(new CustomEvent("fistbump:applications", {detail: {}}));
        await refresh();
        if (failed.length) window.UI.alert(failed.join("\n"));
      }

      async function trash(chosen) {
        const tracked = chosen.filter(e => e.app);
        const free = chosen.filter(e => !e.app);
        const parts = [free.length ? `${free.length === 1 ? "1 job goes" : `${free.length} jobs go`} to the Trash and ${free.length === 1 ? "is" : "are"} deleted for good later, unless restored.` : "",
          tracked.length ? `${tracked.length === 1 ? "1 job is" : `${tracked.length} jobs are`} in the Tracker, so ${tracked.length === 1 ? "it is" : "they are"} archived instead: hidden, not deleted.` : ""];
        if (!await window.UI.ask(parts.filter(Boolean).join("\n\n"), {confirm: free.length ? "Move to Trash" : "Archive", danger: free.length > 0, title: "Move to Trash"})) return;
        await run(chosen, e => e.app ? api().jobs.archive(e.job.id, {archive_application: true}) : api().jobs.trash(e.job.id));
      }

      async function removeForGood(chosen) {
        const n = chosen.length;
        if (!await window.UI.ask(`Delete ${n === 1 ? "this job" : `these ${n} jobs`} now?\n\nThis cannot be undone. A job that is in the Tracker cannot be deleted until its application is.`,
          {confirm: "Delete now", danger: true, title: "Delete jobs"})) return;
        await run(chosen, e => api().jobs.delete(e.job.id));
      }

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
      const icon = id => window.UI?.icon ? window.UI.icon(id) : el("span");

      refresh();
      return refresh;
    },
  };
})();

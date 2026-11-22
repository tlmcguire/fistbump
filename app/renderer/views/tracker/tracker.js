//the tracker: every application in one table. edits save as you make them, one PUT each, and a
//failed edit puts the old value back. All text is set with textContent.
(() => {
  const STAGES = ["Saved", "Applied", "Interviewing", "Offered", "Rejected"];
  const EXCLUSIVE = {Offered: "Rejected", Rejected: "Offered"};
  const SCOPES = [["", "Active"], ["only", "Archived"], ["include", "All"]];
  //what each scope holds: an archived application is kept, only hidden
  const SCOPE_HINTS = {
    "": "The applications you are working on.",
    only: "Archived applications. They are hidden from Active, but nothing is deleted.",
    include: "Every application, archived or not.",
  };

  //today as YYYY-MM-DD on this computer's clock, so a step due today is not overdue in the evening
  const localToday = () => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  };

  //due today or overdue first, soonest first; then the rest in the order the backend sent (latest update first)
  const order = (list, today) => {
    const due = a => a.next_step_date && a.next_step_date <= today;
    return list
      .map((a, i) => ({a, i}))
      .sort((x, y) => {
        const dx = due(x.a), dy = due(y.a);
        if (dx !== dy) return dx ? -1 : 1;
        if (dx && x.a.next_step_date !== y.a.next_step_date) return x.a.next_step_date < y.a.next_step_date ? -1 : 1;
        return x.i - y.i;
      })
      .map(({a}) => a);
  };

  window.Tracker = {
    focusId: null,
    order,
    localToday,

    //Jobs and Tailor call this to show one application
    open(id) {
      this.focusId = id;
      window.App.go("tracker");
    },

    mount(container) {
      const {el, fill} = window.ResumeBuilder;
      const {icon, pico, stagger, play} = window.UI;
      const api = window.FistBumpAPI;

      let rows = [];             //the applications shown, in table order
      const selected = new Set();
      const statuses = new Set(); //empty means every stage
      let scope = "";
      let sortKey = null;         //null keeps due steps first, then the latest update
      let sortDir = 1;
      let tailoredNames = new Map(); //tailored resume id -> label
      let versionNames = new Map();  //saved resume id -> label
      let loadSeq = 0;

      const status = el("p", {class: "resume-status", role: "status"});
      const say = (message, isError = false) => {
        status.textContent = message;
        status.classList.toggle("error", isError);
      };

      // --- toolbar: search and Active/Archived/All; under it the stage chips ---
      const search = el("input", {type: "search", "aria-label": "Search applications", placeholder: "Search by company or title"});
      const stageChips = STAGES.map(s => el("button", {
        type: "button", class: "stage-chip", "data-stage": s, "aria-pressed": "false",
        onclick: () => {
          if (statuses.has(s)) statuses.delete(s);
          else {
            statuses.add(s);
            //an application is offered or rejected, never both: choosing one clears the other
            const other = EXCLUSIVE[s];
            if (other) statuses.delete(other);
          }
          stageChips.forEach(chip => chip.setAttribute("aria-pressed", String(statuses.has(chip.dataset.stage))));
          draw();
        },
      }, el("span", {class: "stage-dot", "aria-hidden": "true"}), s, el("span", {class: "stage-count", "aria-hidden": "true"})));
      const scopeButtons = SCOPES.map(([value, label]) => el("button", {
        type: "button", class: "tracker-scope", "aria-pressed": String(value === scope), title: SCOPE_HINTS[value],
        onclick: () => {
          scope = value;
          scopeButtons.forEach((b, i) => b.setAttribute("aria-pressed", String(SCOPES[i][0] === scope)));
          load();
        },
      }, label));
      let searchTimer = null;
      search.addEventListener("input", () => { clearTimeout(searchTimer); searchTimer = setTimeout(load, 200); });

      // --- bulk bar, shown while rows are selected ---
      const bulkCount = el("span");
      const bulkStatus = el("select", {"aria-label": "New status for the selected applications"},
        el("option", {value: "", text: "Set status…"}), STAGES.map(s => el("option", {value: s, text: s})));
      const bulkArchive = el("button", {type: "button", title: "Hide these applications from Active. They are kept, not deleted."}, "Archive");
      const bulkRestore = el("button", {type: "button", title: "Bring these applications back to Active."}, "Unarchive");
      const bulkBar = el("div", {class: "tracker-bulk", hidden: true}, bulkCount, bulkStatus, bulkArchive, bulkRestore,
        el("button", {type: "button", onclick: () => { selected.clear(); draw(); }}, "Clear selection"));

      const runBulk = async fn => {
        const ids = rows.filter(a => selected.has(a.id)).map(a => a.id);
        if (!ids.length) return;
        try {
          await fn(ids);
          selected.clear();
          say(`Updated ${ids.length} application${ids.length === 1 ? "" : "s"}.`);
        } catch (err) {
          say(err.message, true); //all or nothing: no row changed
        }
        load();
      };
      bulkStatus.addEventListener("change", () => {
        const value = bulkStatus.value;
        bulkStatus.value = "";
        if (value) runBulk(ids => api.applications.bulkStatus(ids, value));
      });
      bulkArchive.addEventListener("click", () => runBulk(ids => api.applications.bulkArchive(ids, true)));
      bulkRestore.addEventListener("click", () => runBulk(ids => api.applications.bulkArchive(ids, false)));

      const dueStrip = el("section", {class: "due-strip", "aria-label": "Needs you today", hidden: true});
      const table = el("div", {class: "tracker-table-wrap"});
      const toolbar = el("header", {class: "toolbar drag"},
        el("div", {class: "toolbar-form resume-toolbar"},
          el("h1", {class: "view-title", text: "Tracker"}),
          status,
          el("div", {class: "toolbar-actions"}, search, el("div", {class: "tracker-scopes", role: "group", "aria-label": "Show"}, scopeButtons))));
      fill(container, toolbar,
        el("div", {class: "pane tracker"},
          dueStrip,
          el("div", {class: "stage-chips", role: "group", "aria-label": "Filter by stage"}, stageChips),
          bulkBar,
          table));

      // --- one PUT per edit; on failure the old value goes back ---
      const save = async (app, field, value, restore) => {
        try {
          const updated = await api.applications.update(app.id, {[field]: value});
          Object.assign(app, updated);
          say("");
          return updated;
        } catch (err) {
          restore();
          say(err.message, true);
          return null;
        }
      };

      //opens the job in the Jobs view
      const openJob = jobId => {
        window.App.go("jobs");
        window.JobParser.openJob?.(jobId);
      };

      //the company's own site when known, else the listing, in the system browser
      const openSite = async jobId => {
        try {
          const job = await api.jobs.get(jobId);
          const url = job.company_url || job.listing_url;
          if (url) window.open(url, "_blank");
          else say("This job has no website or listing link.", true);
        } catch (err) {
          say(err.message, true);
        }
      };

      const resumeLabel = a => {
        if (a.tailored_resume_id) return tailoredNames.get(a.tailored_resume_id) || "Tailored resume";
        if (a.base_resume_id) return versionNames.get(a.base_resume_id) || "Saved version";
        return "Master";
      };

      const row = (a, today) => {
        const pick = el("input", {type: "checkbox", "aria-label": `Select ${a.company_name} ${a.position_title}`, checked: selected.has(a.id),
          onchange: event => { if (event.target.checked) selected.add(a.id); else selected.delete(a.id); drawBulk(); }});

        const stage = el("select", {class: "stage-select", "aria-label": `Status of ${a.company_name} ${a.position_title}`, "data-stage": a.status}, STAGES.map(s => el("option", {value: s, text: s})));
        stage.value = a.status;
        stage.addEventListener("change", async () => {
          const old = a.status;
          const updated = await save(a, "status", stage.value, () => { stage.value = old; stage.dataset.stage = old; });
          if (updated) {
            stage.dataset.stage = updated.status;
            applied.value = updated.date_applied || ""; //Applied fills today's date
            play(stage, "pop", 500);
            draw();
          }
        });

        const dateInput = (field, label) => {
          const input = el("input", {type: "date", "aria-label": label, value: a[field] || ""});
          input.addEventListener("change", () => {
            const old = a[field] || "";
            save(a, field, input.value || null, () => { input.value = old; }).then(updated => { if (updated) draw(); });
          });
          return input;
        };
        const applied = dateInput("date_applied", "Date applied");
        const next = dateInput("next_step_date", "Next step");
        const due = a.next_step_date && a.next_step_date <= today;

        const notes = el("textarea", {rows: "1", "aria-label": "Notes", placeholder: "Notes", value: a.notes || ""});
        notes.value = a.notes || "";
        notes.addEventListener("blur", () => {
          const old = a.notes || "";
          if (notes.value === old) return;
          save(a, "notes", notes.value.trim() ? notes.value : null, () => { notes.value = old; });
        });

        const pdf = a.exported_pdf_path ? el("span", {class: "tracker-pdf"}, "PDF saved ",
          el("button", {type: "button", class: "link-button", onclick: async event => {
            try {
              await api.shell.showItem(a.exported_pdf_path);
            } catch (err) {
              event.target.textContent = err.message;
              event.target.disabled = true;
            }
          }}, "Show in folder")) : null;

        return el("tr", {class: [due ? "due" : "", a.archived_at ? "archived" : ""].join(" ").trim() || null, "data-id": String(a.id)},
          el("td", {}, pick),
          el("td", {}, el("button", {type: "button", class: "link-button", onclick: () => openJob(a.job_id)}, a.company_name || "Unknown company"),
            el("button", {type: "button", class: "icon-link", title: "Open the company's site", "aria-label": `Open ${a.company_name || "the company"}'s site`,
              onclick: () => openSite(a.job_id)}, icon("external"))),
          el("td", {}, el("button", {type: "button", class: "link-button", onclick: () => openJob(a.job_id)}, a.position_title || "Unknown position"),
            a.archived_at ? el("span", {class: "hint", text: " (archived)"}) : null),
          el("td", {}, stage),
          el("td", {}, applied),
          el("td", {}, next, due ? el("span", {class: "tracker-due"}, icon("clock"), a.next_step_date < today ? "Overdue" : "Due today") : null),
          el("td", {}, el("span", {text: resumeLabel(a)}), pdf),
          el("td", {}, notes));
      };

      const drawBulk = () => {
        const n = rows.filter(a => selected.has(a.id)).length;
        bulkBar.hidden = n === 0;
        bulkCount.textContent = `${n} selected`;
      };

      const SORTS = {company_name: "Company", position_title: "Position", status: "Status", date_applied: "Date applied", next_step_date: "Next step"};
      const sortValue = (a, key) => key === "status" ? STAGES.indexOf(a.status) : (a[key] || "");
      const sorted = (list, today) => {
        if (!sortKey) return order(list, today);
        return [...list].sort((x, y) => {
          const vx = sortValue(x, sortKey), vy = sortValue(y, sortKey);
          //blank dates go last whichever way it is sorted
          if (vx === "" || vy === "") return vx === vy ? 0 : vx === "" ? 1 : -1;
          return (vx < vy ? -1 : vx > vy ? 1 : 0) * sortDir;
        });
      };

      const drawDue = today => {
        const dueRows = rows.filter(a => a.next_step_date && a.next_step_date <= today && !a.archived_at);
        dueStrip.hidden = dueRows.length === 0;
        if (!dueRows.length) return;
        fill(dueStrip,
          el("h2", {text: dueRows.length === 1 ? "1 step needs you" : `${dueRows.length} steps need you`}),
          el("ul", {}, order(dueRows, today).map(a => el("li", {},
            el("button", {type: "button", class: "due-item", onclick: () => {
              const tr = table.querySelector(`tr[data-id="${a.id}"]`);
              if (!tr) return;
              tr.scrollIntoView({block: "center", behavior: window.UI.motion() ? "smooth" : "auto"});
              play(tr, "flash", 1400);
            }}, icon("clock"), el("span", {class: "due-what", text: `${a.company_name || "Unknown company"}, ${a.position_title || "Unknown position"}`}),
              el("span", {class: "due-when", text: a.next_step_date < today ? "Overdue" : "Today"}))))));
      };

      const draw = () => {
        const today = localToday();
        const shown = statuses.size ? rows.filter(a => statuses.has(a.status)) : rows;
        drawBulk();
        drawDue(today);
        for (const chip of stageChips) {
          const n = rows.filter(a => a.status === chip.dataset.stage).length;
          chip.querySelector(".stage-count").textContent = n ? String(n) : "";
        }
        if (!rows.length) {
          const searching = search.value.trim();
          const go = scope === "only" || searching ? null
            : el("button", {type: "button", class: "primary", onclick: () => window.App.openTab("tab-saved")}, "Open saved jobs");
          fill(table, el("div", {class: "empty tracker-empty"}, pico("33-empty-tracker.png"),
            el("h2", {text: scope === "only" ? "No archived applications" : searching ? "No applications match your search" : "Nothing tracked yet"}),
            el("p", {text: scope === "only"
              ? "Archived applications are hidden from Active, but nothing is deleted — unarchive them to bring them back."
              : searching ? "Try another search or view."
              : "Track a job from its listing, or from a tailored resume, and its next step shows up here."}),
            go));
          play(table, "enter", 500);
          return;
        }
        if (!shown.length) {
          fill(table, el("div", {class: "empty tracker-empty"}, el("p", {text: "No applications in the chosen stages."})));
          return;
        }
        const all = el("input", {type: "checkbox", "aria-label": "Select all shown", checked: shown.every(a => selected.has(a.id)),
          onchange: event => { shown.forEach(a => event.target.checked ? selected.add(a.id) : selected.delete(a.id)); draw(); }});
        const head = (key, label) => el("th", {scope: "col", "aria-sort": sortKey === key ? (sortDir === 1 ? "ascending" : "descending") : "none"},
          el("button", {type: "button", class: "sort-button", onclick: () => {
            if (sortKey === key && sortDir === -1) sortKey = null;
            else if (sortKey === key) sortDir = -1;
            else { sortKey = key; sortDir = 1; }
            draw();
          }}, label, el("span", {class: "sort-arrow", "aria-hidden": "true", text: sortKey === key ? (sortDir === 1 ? "\u2191" : "\u2193") : ""})));
        const body = el("tbody", {}, sorted(shown, today).map(a => row(a, today)));
        fill(table, el("table", {class: "tracker-table"},
          el("thead", {}, el("tr", {}, el("th", {scope: "col"}, all),
            Object.entries(SORTS).map(([key, label]) => head(key, label)),
            el("th", {scope: "col", text: "Resume"}), el("th", {scope: "col", text: "Notes"}))),
          body));
        if (!draw.seen) { draw.seen = true; stagger(body, 24, 14); }

        const focus = window.Tracker.focusId;
        if (focus) {
          window.Tracker.focusId = null;
          const tr = table.querySelector(`tr[data-id="${Number(focus)}"]`);
          if (tr) {
            tr.classList.add("focused");
            tr.scrollIntoView({block: "center"});
          }
        }
      };

      //the labels for the Resume column: saved versions in one call, each tailored resume once
      const loadNames = async list => {
        if (!versionNames.size && list.some(a => a.base_resume_id && !a.tailored_resume_id)) {
          try {
            versionNames = new Map((await api.resumes.list()).map(v => [v.id, v.version_label]));
          } catch { /* the column says "Saved version" */ }
        }
        const missing = [...new Set(list.map(a => a.tailored_resume_id).filter(id => id && !tailoredNames.has(id)))];
        await Promise.all(missing.map(async id => {
          try {
            const t = await api.tailored.get(id);
            tailoredNames.set(id, `Tailored, ${new Date(t.created_at).toLocaleDateString()}`);
          } catch {
            tailoredNames.set(id, "Tailored resume");
          }
        }));
      };

      const load = async () => {
        const mine = ++loadSeq;
        try {
          //a focused row may be archived, so it is looked for in every scope
          if (window.Tracker.focusId && scope === "") {
            const focused = await api.applications.get(window.Tracker.focusId).catch(() => null);
            if (focused?.archived_at) {
              scope = "include";
              scopeButtons.forEach((b, i) => b.setAttribute("aria-pressed", String(SCOPES[i][0] === scope)));
            }
          }
          const list = await api.applications.list({q: search.value.trim(), archived: scope});
          await loadNames(list);
          if (mine !== loadSeq) return;
          rows = list;
          for (const id of [...selected]) if (!rows.some(a => a.id === id)) selected.delete(id);
          draw();
        } catch (err) {
          if (mine !== loadSeq) return;
          say(err.message, true);
        }
      };

      load();
      return load; //drawn again whenever the view is shown
    },
  };
})();

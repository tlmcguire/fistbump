//the tracker: every application in one table. edits save as you make them, one PUT each, and a
//failed edit puts the old value back. All text is set with textContent.
(() => {
  const STAGES = ["Saved", "Applied", "Interviewing", "Offered", "Rejected"];
  const SCOPES = [["", "Active"], ["only", "Archived"], ["include", "All"]];

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
      const api = window.FistBumpAPI;

      let rows = [];             //the applications shown, in table order
      const selected = new Set();
      const statuses = new Set(); //empty means every stage
      let scope = "";
      let tailoredNames = new Map(); //tailored resume id -> label
      let versionNames = new Map();  //saved resume id -> label
      let loadSeq = 0;

      const status = el("p", {class: "resume-status", role: "status"});
      const say = (message, isError = false) => {
        status.textContent = message;
        status.classList.toggle("error", isError);
      };

      // --- toolbar: search, stage filter, Active/Archived/All ---
      const search = el("input", {type: "search", "aria-label": "Search applications", placeholder: "Search by company or title"});
      const stageMenu = el("details", {class: "tracker-stages"},
        el("summary", {text: "All stages"}),
        el("div", {class: "tracker-stage-list"}, STAGES.map(s => el("label", {},
          el("input", {type: "checkbox", value: s, onchange: event => {
            if (event.target.checked) statuses.add(s); else statuses.delete(s);
            stageMenu.querySelector("summary").textContent = statuses.size ? [...statuses].join(", ") : "All stages";
            draw();
          }}), ` ${s}`))));
      const scopeButtons = SCOPES.map(([value, label]) => el("button", {
        type: "button", class: "tracker-scope", "aria-pressed": String(value === scope),
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
      const bulkArchive = el("button", {type: "button"}, "Archive");
      const bulkRestore = el("button", {type: "button"}, "Unarchive");
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

      const table = el("div", {class: "tracker-table-wrap"});
      fill(container, el("div", {class: "pane tracker"},
        el("div", {class: "tracker-head"}, el("h1", {class: "view-title", text: "Tracker"}), status),
        el("div", {class: "tracker-filters"}, search, stageMenu, el("div", {class: "tracker-scopes", role: "group", "aria-label": "Show"}, scopeButtons)),
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

        const stage = el("select", {"aria-label": "Status"}, STAGES.map(s => el("option", {value: s, text: s})));
        stage.value = a.status;
        stage.addEventListener("change", async () => {
          const old = a.status;
          const updated = await save(a, "status", stage.value, () => { stage.value = old; });
          if (updated) applied.value = updated.date_applied || ""; //Applied fills today's date
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
              onclick: () => openSite(a.job_id)}, "↗")),
          el("td", {}, el("button", {type: "button", class: "link-button", onclick: () => openJob(a.job_id)}, a.position_title || "Unknown position"),
            a.archived_at ? el("span", {class: "hint", text: " (archived)"}) : null),
          el("td", {}, stage),
          el("td", {}, applied),
          el("td", {}, next, due ? el("span", {class: "tracker-due", text: a.next_step_date < today ? "Overdue" : "Today"}) : null),
          el("td", {}, el("span", {text: resumeLabel(a)}), pdf),
          el("td", {}, notes));
      };

      const drawBulk = () => {
        const n = rows.filter(a => selected.has(a.id)).length;
        bulkBar.hidden = n === 0;
        bulkCount.textContent = `${n} selected`;
      };

      const draw = () => {
        const today = localToday();
        const shown = statuses.size ? rows.filter(a => statuses.has(a.status)) : rows;
        drawBulk();
        if (!rows.length) {
          fill(table, el("div", {class: "empty"}, el("p", {text: scope === "only"
            ? "No archived applications."
            : search.value.trim() ? "No applications match your search." : "No applications yet. Track a job from the Jobs or Tailor view."})));
          return;
        }
        if (!shown.length) {
          fill(table, el("div", {class: "empty"}, el("p", {text: "No applications in the chosen stages."})));
          return;
        }
        const all = el("input", {type: "checkbox", "aria-label": "Select all shown", checked: shown.every(a => selected.has(a.id)),
          onchange: event => { shown.forEach(a => event.target.checked ? selected.add(a.id) : selected.delete(a.id)); draw(); }});
        fill(table, el("table", {class: "tracker-table"},
          el("thead", {}, el("tr", {}, el("th", {}, all),
            ["Company", "Position", "Status", "Date applied", "Next step", "Resume", "Notes"].map(h => el("th", {text: h})))),
          el("tbody", {}, order(shown, today).map(a => row(a, today)))));

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

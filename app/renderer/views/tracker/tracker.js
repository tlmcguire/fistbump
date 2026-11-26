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
      let topSkillGaps = [];
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
      const bulkDelete = el("button", {type: "button", class: "danger-solid", title: "Permanently remove the selected tracked applications from your Tracker."}, "Delete selected");
      const bulkBar = el("div", {class: "tracker-bulk", hidden: true}, bulkCount, bulkStatus, bulkArchive, bulkRestore, bulkDelete,
        el("button", {type: "button", onclick: () => { selected.clear(); draw(); }}, "Clear selection"));

      const notifyAppsChanged = () => {
        window.dispatchEvent(new CustomEvent("fistbump:applications", {detail: {invalidateMatches: false, source: "tracker"}}));
      };
      const runBulk = async fn => {
        const ids = rows.filter(a => selected.has(a.id)).map(a => a.id);
        if (!ids.length) return;
        try {
          await fn(ids);
          selected.clear();
          say(`Updated ${ids.length} application${ids.length === 1 ? "" : "s"}.`);
          notifyAppsChanged();
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
      bulkDelete.addEventListener("click", async () => {
        const ids = rows.filter(a => selected.has(a.id)).map(a => a.id);
        if (!ids.length) return;
        const noun = ids.length === 1 ? "tracked application" : "tracked applications";
        const ok = await window.UI.ask(
          `Delete ${ids.length} ${noun}?\n\nThis removes ${ids.length === 1 ? "it" : "them"} from your Tracker permanently. Your saved job postings are not deleted.`,
          {confirm: "Delete", danger: true, title: "Delete tracked applications"}
        );
        if (!ok) return;
        const settled = await Promise.allSettled(ids.map(id => api.applications.delete(id)));
        const deleted = settled.filter(r => r.status === "fulfilled").length;
        const failed = settled.length - deleted;
        selected.clear();
        if (deleted > 0) notifyAppsChanged();
        if (failed) {
          say(`Deleted ${deleted} ${deleted === 1 ? "application" : "applications"}; ${failed} could not be deleted.`, true);
        } else {
          say(`Deleted ${deleted} ${deleted === 1 ? "application" : "applications"}.`);
        }
        load();
      });

      const downloadBlob = (filename, content, type) => {
        const blob = new Blob([content], {type});
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = filename;
        a.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
      };

      const exportCsvButton = el("button", {type: "button", onclick: () => {
        const esc = v => `"${String(v ?? "").replace(/"/g, '""')}"`;
        const header = ["Company", "Position", "Status", "Date Applied", "Next Step Date", "Notes"].map(esc).join(",");
        const lines = rows.map(a => [a.company_name, a.position_title, a.status, a.date_applied || "", a.next_step_date || "", a.notes || ""].map(esc).join(","));
        downloadBlob("Fistbump-Applications.csv", [header, ...lines].join("\r\n") + "\r\n", "text/csv;charset=utf-8");
        say(`Exported ${rows.length} application${rows.length === 1 ? "" : "s"} to CSV.`);
      }}, "Export CSV");

      const money = n => `${n < 0 ? "-" : ""}$${Math.abs(Math.round(n || 0)).toLocaleString()}`;

      // --- Side-by-side offer comparison dialog (Item 13) ---
      const COMPARE_PILL = {
        pay: {well_above: "Well above market", above: "Above market", in_line: "At market", below: "Below market", well_below: "Well below market", unknown: "Not compared"},
        benefits: {strong: "Strong", decent: "Decent", basic: "Limited", unclear: "Unclear", unlisted: "Not listed"},
        life: {comfortable: "Comfortable", okay: "Workable", tight: "Tight", short: "Short"},
      };
      const COMPARE_TONE = {
        well_above: "good", above: "good", in_line: "ok", below: "warn", well_below: "bad", unknown: "ok",
        comfortable: "good", okay: "ok", tight: "warn", short: "bad",
        strong: "good", decent: "ok", basic: "warn", unclear: "ok", unlisted: "ok",
      };

      const openCompareOffers = async () => {
        if (rows.length < 2) {
          say("Track at least 2 jobs to compare offers side by side.", true);
          return;
        }
        const picked = rows.filter(a => selected.has(a.id));
        const initial = (picked.length >= 2 ? picked : rows).slice(0, 3);
        const dialog = el("dialog", {class: "ask compare-dialog", "aria-label": "Compare offers side by side"});
        const inputsWrap = el("div", {class: "compare-inputs"});
        const resultBox = el("div", {class: "compare-results"});
        let jobsCache = [];
        let recalcTimer = null;

        const makeField = (label, placeholder, onInput) => {
          const input = el("input", {type: "number", min: "0", step: "1000", placeholder});
          input.addEventListener("input", onInput);
          return {
            input,
            node: el("label", {class: "compare-field"}, el("span", {text: label}), input),
          };
        };

        const scheduleCompare = () => {
          clearTimeout(recalcTimer);
          recalcTimer = setTimeout(runCompare, 350);
        };

        const inputs = initial.map(a => {
          const base = makeField("Base ($/yr)", "From posting", scheduleCompare);
          const bonus = makeField("Bonus ($/yr)", "0", scheduleCompare);
          const equity = makeField("Equity ($/yr)", "0", scheduleCompare);
          const meta = el("span", {class: "compare-card-meta", text: a.position_title || "Role"});
          const card = el("div", {class: "compare-card"},
            el("div", {class: "compare-card-head"},
              el("strong", {text: a.company_name || "Company"}),
              meta),
            el("div", {class: "compare-fields"}, base.node, bonus.node, equity.node));
          return {app: a, base: base.input, bonus: bonus.input, equity: equity.input, meta, card};
        });
        inputsWrap.append(...inputs.map(x => x.card));

        const verdictPill = (kind, raw) => {
          if (!raw) return el("span", {class: "compare-muted", text: "\u2014"});
          const label = COMPARE_PILL[kind]?.[raw] || String(raw).replace(/_/g, " ");
          const tone = COMPARE_TONE[raw] || "ok";
          return el("span", {class: `cost-pill ${tone}`, text: label});
        };

        const runCompare = async () => {
          clearTimeout(recalcTimer);
          resultBox.replaceChildren(el("p", {class: "hint", text: "Calculating comparison\u2026"}));
          try {
            if (!jobsCache.length) {
              jobsCache = await Promise.all(inputs.map(x => api.jobs.get(x.app.job_id)));
              inputs.forEach((x, i) => {
                const job = jobsCache[i];
                const parts = [job?.position_title || x.app.position_title, job?.location || job?.work_mode].filter(Boolean);
                x.meta.textContent = parts.join(" \u00b7 ");
                if (job?.pay_min || job?.pay_max) {
                  const mid = job.pay_min && job.pay_max ? Math.round((job.pay_min + job.pay_max) / 2) : (job.pay_max || job.pay_min);
                  x.base.placeholder = `Est. ${money(mid)}`;
                } else {
                  x.base.placeholder = "Enter base";
                }
              });
            }
            const offers = inputs.map((x, i) => {
              const job = jobsCache[i];
              const customBase = Math.max(0, Number(x.base.value) || 0);
              return {
                label: `${x.app.company_name || "Company"} \u2014 ${x.app.position_title || "Role"}`,
                title: job?.position_title || x.app.position_title || "",
                location: job?.location || "",
                work_mode: job?.work_mode || "",
                pay_min: customBase > 0 ? customBase : (job?.pay_min || 0),
                pay_max: customBase > 0 ? customBase : (job?.pay_max || 0),
                bonus: Math.max(0, Number(x.bonus.value) || 0),
                annual_equity: Math.max(0, Number(x.equity.value) || 0),
                text: job?.raw_text || "",
              };
            });
            const res = await api.cost.compare({offers});
            const cols = res.offers || [];

            const bestIndex = (values, mode = "max") => {
              const valid = values.map((v, i) => ({v, i})).filter(x => Number.isFinite(x.v) && x.v > 0);
              if (valid.length < 2) return -1;
              valid.sort((a, b) => mode === "min" ? a.v - b.v : b.v - a.v);
              return valid[0].v === valid[1].v ? -1 : valid[0].i;
            };

            const numRow = (label, getVal, fmt, {mode = "max", emph = false} = {}) => {
              const vals = cols.map(getVal);
              const best = bestIndex(vals, mode);
              return el("tr", {class: emph ? "compare-emph" : null},
                el("th", {scope: "row", text: label}),
                ...vals.map((v, i) => {
                  if (!Number.isFinite(v)) return el("td", {}, el("span", {class: "compare-muted", text: "\u2014"}));
                  return el("td", {class: i === best ? "compare-best" : null, text: fmt(v)});
                }));
            };

            const pillRow = (label, kind, getVal) => el("tr", {},
              el("th", {scope: "row", text: label}),
              ...cols.map(c => el("td", {}, verdictPill(kind, getVal(c)))));

            resultBox.replaceChildren(
              el("div", {class: "compare-table-wrap"},
                el("table", {class: "compare-table"},
                  el("thead", {}, el("tr", {},
                    el("th", {scope: "col", text: "Metric"}),
                    ...inputs.map(x => el("th", {scope: "col"},
                      el("span", {class: "compare-col-company", text: x.app.company_name || "Company"}),
                      el("span", {class: "compare-col-role", text: x.app.position_title || "Role"}))))),
                  el("tbody", {},
                    numRow("Total comp (est. yr)", c => c.total_comp || c.analysis?.lifestyle?.gross || NaN, money, {emph: true}),
                    numRow("Bonus + Equity", c => (c.bonus || 0) + (c.annual_equity || 0), money),
                    numRow("Effective tax rate", c => c.analysis?.lifestyle ? c.analysis.lifestyle.tax_rate : NaN, v => `${v}%`, {mode: "min"}),
                    numRow("Monthly take-home", c => c.analysis?.lifestyle ? c.analysis.lifestyle.net_monthly : NaN, money),
                    numRow("Monthly leftover", c => c.analysis?.lifestyle ? c.analysis.lifestyle.left_monthly : NaN, money, {emph: true}),
                    pillRow("Pay verdict", "pay", c => c.analysis?.pay?.verdict),
                    pillRow("Benefits", "benefits", c => c.analysis?.benefits?.rating),
                    pillRow("Cost of living", "life", c => c.analysis?.lifestyle?.verdict)))));
          } catch (err) {
            resultBox.replaceChildren(el("p", {class: "resume-status error", text: err.message}));
          }
        };
        const close = () => { clearTimeout(recalcTimer); dialog.close(); dialog.remove(); };
        dialog.append(
          el("h2", {text: "Side-by-side offer comparison"}),
          el("p", {class: "hint", text: "Enter an optional base salary (if not listed), annual bonus, or equity to compare total compensation and cost of living."}),
          inputsWrap,
          resultBox,
          el("div", {class: "ask-actions"},
            el("button", {type: "button", onclick: close}, "Close"),
            el("button", {type: "button", class: "primary", onclick: runCompare}, "Recalculate")));
        window.UI.wireDialogClose(dialog, close);
        dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
        document.body.append(dialog);
        dialog.showModal();
        runCompare();
      };

      // --- Editable, persistent Interview Prep Sheet dialog (synced with Tailor drafts) ---
      const renderSimpleMarkdown = md => {
        const frag = document.createDocumentFragment();
        const lines = String(md || "").replace(/\r\n/g, "\n").split("\n");
        let listEl = null;
        let listTag = null;
        const flushList = () => { if (listEl) { frag.append(listEl); listEl = null; listTag = null; } };
        for (const raw of lines) {
          const line = raw.trim();
          if (!line) { flushList(); continue; }
          if (/^###\s+/.test(line)) { flushList(); frag.append(el("h4", {text: line.replace(/^###\s+/, "")})); continue; }
          if (/^##\s+/.test(line)) { flushList(); frag.append(el("h3", {text: line.replace(/^##\s+/, "")})); continue; }
          if (/^#\s+/.test(line)) { flushList(); frag.append(el("h2", {text: line.replace(/^#\s+/, "")})); continue; }
          if (/^[-*]\s+/.test(line)) {
            if (listTag !== "ul") { flushList(); listEl = el("ul", {class: "prep-list"}); listTag = "ul"; }
            listEl.append(el("li", {text: line.replace(/^[-*]\s+/, "")}));
            continue;
          }
          if (/^\d+\.\s+/.test(line)) {
            if (listTag !== "ol") { flushList(); listEl = el("ol", {class: "problem-steps"}); listTag = "ol"; }
            listEl.append(el("li", {text: line.replace(/^\d+\.\s+/, "")}));
            continue;
          }
          flushList();
          frag.append(el("p", {text: line}));
        }
        flushList();
        return frag;
      };

      const openPrepSheet = async a => {
        const dialog = el("dialog", {class: "ask prep-dialog", "aria-label": "Interview prep sheet"});
        const body = el("div", {class: "prep-body"}, el("p", {class: "hint", text: "Loading interview prep sheet\u2026"}));
        let mdContent = "";
        let saveTimer = null;
        let saveNow = async () => {};
        const close = async () => {
          clearTimeout(saveTimer);
          await saveNow().catch(() => {});
          dialog.close();
          dialog.remove();
        };
        const copyBtn = el("button", {type: "button", onclick: async () => {
          if (!mdContent.trim()) return;
          try { await navigator.clipboard.writeText(mdContent); prepStatus.textContent = "Copied."; } catch {}
        }}, "Copy");
        const dlBtn = el("button", {type: "button", onclick: () => {
          if (!mdContent.trim()) return;
          downloadBlob(`${(a.company_name || "Interview").replace(/[^a-z0-9_-]+/gi, "-")}-Prep-Sheet.md`, mdContent.trim() + "\n", "text/markdown;charset=utf-8");
          prepStatus.textContent = "Downloaded .md.";
        }}, "Download .md");
        const tailorBtn = el("button", {type: "button", title: "Open this job in Tailor", onclick: async () => {
          await close();
          window.Tailor?.open?.(a.job_id);
        }}, "Open in Tailor");
        const printBtn = el("button", {type: "button", class: "primary", onclick: () => window.print()}, "Print");
        const prepStatus = el("span", {class: "hint prep-status", role: "status"});
        dialog.append(
          el("h2", {text: `Interview prep: ${a.company_name || "Company"} \u2014 ${a.position_title || "Role"}`}),
          body,
          el("div", {class: "ask-actions prep-actions"}, prepStatus, tailorBtn, copyBtn, dlBtn, printBtn, el("button", {type: "button", onclick: close}, "Done")));
        window.UI.wireDialogClose(dialog, close);
        dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
        document.body.append(dialog);
        dialog.showModal();
        try {
          const [job, analysis, drafts] = await Promise.all([
            api.jobs.get(a.job_id),
            api.jobs.analyze(a.job_id).catch(() => null),
            api.drafts?.list ? api.drafts.list(a.job_id).catch(() => []) : Promise.resolve([]),
          ]);
          const matched = (analysis?.required?.tech?.matched || []).map(x => x.skill);
          const missing = analysis?.required?.tech?.missing || [];
          const buildDefaultPrep = () => [
            `# Interview Prep: ${a.company_name || "Company"} — ${a.position_title || "Role"}`,
            `Stage: ${a.status}${a.next_step_date ? ` · Next step: ${a.next_step_date}` : ""}`,
            a.notes ? `\n## Your Notes\n${a.notes}` : "",
            `\n## Matched Required Skills\n${matched.length ? matched.join(", ") : "None listed"}`,
            `\n## Gaps to Address Honestly\n${missing.length ? `- Skills not yet on your resume: ${missing.slice(0, 5).join(", ")}.\n- Prepare honest framing connecting adjacent fundamentals and how you are ramping up.` : "No major required skill gaps detected."}`,
            `\n## Practice Questions (STAR)\n1. Why ${a.company_name || "this company"} and why the ${a.position_title || "role"} right now?\n2. ${matched.length ? `Walk through a concrete production problem you solved with ${matched.slice(0, 2).join(" and ")} and how you measured impact.` : "Walk through a complex project you owned end-to-end and how you measured success."}\n3. ${missing.length ? `How would you ramp up on ${missing[0]} in your first 30–60 days?` : "Tell me about a tough trade-off you had to make under tight constraints."}\n4. Describe a time you collaborated across teams or resolved a technical disagreement.`,
            `\n## Questions to Ask the Interviewer\n- What does success look like in the first 90 days for this ${a.position_title || "role"}?\n- What are the biggest technical or product challenges the team is tackling this quarter?`,
          ].filter(Boolean).join("\n");

          const isPrepDraft = d => d.kind === "interview_prep" || (d.kind === "qa" && String(d.prompt || "").trim().toLowerCase() === "interview prep");
          let versions = (drafts || []).filter(isPrepDraft).sort((x, y) => (x.created_at < y.created_at ? -1 : x.created_at > y.created_at ? 1 : x.id - y.id));
          let activeDraft = versions.at(-1) || null;
          mdContent = activeDraft?.content || buildDefaultPrep();
          let lastSaved = activeDraft?.content || "";
          let mode = "edit";

          const editor = el("textarea", {class: "prep-editor", rows: 14, "aria-label": "Edit interview prep sheet"});
          editor.value = mdContent;
          const preview = el("div", {class: "prep-preview cost-card", hidden: true});
          const versionsBar = el("div", {class: "draft-versions", role: "group", "aria-label": "Saved prep versions"});
          const wc = el("span", {class: "hint"});
          const syncWC = () => {
            const n = editor.value.trim() ? editor.value.trim().split(/\s+/).length : 0;
            wc.textContent = `${n} word${n === 1 ? "" : "s"}`;
          };
          syncWC();

          const createSavedPrep = async text => {
            if (!api.drafts?.create) return null;
            try {
              return await api.drafts.create(a.job_id, {kind: "interview_prep", prompt: "Interview prep", content: text, generate: false});
            } catch (err) {
              if (/kind must be cover_letter or qa/i.test(err?.message || "")) {
                return await api.drafts.create(a.job_id, {kind: "qa", prompt: "Interview prep", content: text, generate: false});
              }
              throw err;
            }
          };

          const drawVersions = () => {
            versionsBar.hidden = versions.length < 2;
            versionsBar.replaceChildren(...versions.map((v, i) => el("button", {
              type: "button",
              class: "draft-version-chip",
              "aria-pressed": String(activeDraft && v.id === activeDraft.id),
              onclick: async () => {
                await saveNow();
                activeDraft = v;
                mdContent = v.content;
                lastSaved = v.content;
                editor.value = mdContent;
                syncWC();
                if (mode === "preview") preview.replaceChildren(renderSimpleMarkdown(mdContent));
                drawVersions();
              },
            }, `v${i + 1}`)));
          };
          drawVersions();

          saveNow = async () => {
            clearTimeout(saveTimer);
            const text = editor.value;
            mdContent = text;
            if (!text.trim() || text === lastSaved || !api.drafts?.create) return;
            prepStatus.textContent = "Saving…";
            try {
              if (activeDraft) {
                const updated = await api.drafts.update(a.job_id, activeDraft.id, {prompt: activeDraft.prompt || "Interview prep", content: text});
                activeDraft.content = updated.content;
              } else {
                const created = await createSavedPrep(text);
                if (created) {
                  activeDraft = created;
                  versions.push(created);
                  drawVersions();
                }
              }
              lastSaved = text;
              const idx = activeDraft ? versions.findIndex(v => v.id === activeDraft.id) + 1 : 1;
              prepStatus.textContent = `Saved v${idx}`;
            } catch (err) {
              prepStatus.textContent = err?.message || "Could not save";
            }
          };
          editor.addEventListener("input", () => {
            mdContent = editor.value;
            syncWC();
            clearTimeout(saveTimer);
            saveTimer = setTimeout(saveNow, 600);
          });
          editor.addEventListener("blur", saveNow);

          const modeBtns = [["edit", "Edit"], ["preview", "Preview"]].map(([val, label]) => {
            const btn = el("button", {
              type: "button",
              "aria-pressed": String(mode === val),
              onclick: () => {
                mode = val;
                modeBtns.forEach(([v, b]) => b.setAttribute("aria-pressed", String(mode === v)));
                editor.hidden = mode !== "edit";
                preview.hidden = mode !== "preview";
                if (mode === "preview") preview.replaceChildren(renderSimpleMarkdown(editor.value));
              },
            }, label);
            return [val, btn];
          });
          const modeToggle = el("div", {class: "segmented", role: "group", "aria-label": "Prep sheet view"}, modeBtns.map(([, b]) => b));

          const saveNewBtn = el("button", {
            type: "button",
            title: "Keep the current version and save a new snapshot",
            onclick: async () => {
              if (!editor.value.trim() || !api.drafts?.create) return;
              prepStatus.textContent = "Saving new version…";
              try {
                const created = await createSavedPrep(editor.value);
                if (created) {
                  activeDraft = created;
                  versions.push(created);
                  lastSaved = created.content;
                  drawVersions();
                  prepStatus.textContent = `Saved v${versions.length}`;
                }
              } catch (err) {
                prepStatus.textContent = err?.message || "Could not save";
              }
            },
          }, "+ Save new version");

          const aiGenBtn = el("button", {
            type: "button",
            title: "Generate a tailored interview prep sheet using your configured local or remote AI model",
            onclick: async () => {
              if (!api.drafts?.create) return;
              aiGenBtn.disabled = true;
              const started = Date.now();
              prepStatus.textContent = "Generating prep sheet with AI… 0 s";
              const timer = setInterval(() => {
                const sec = Math.round((Date.now() - started) / 1000);
                prepStatus.textContent = sec >= 20
                  ? `Generating prep sheet with AI… ${sec} s (taking longer than usual — check Settings → AI suggestions)`
                  : `Generating prep sheet with AI… ${sec} s`;
              }, 1000);
              let timeoutId = null;
              try {
                const created = await Promise.race([
                  api.drafts.create(a.job_id, {kind: "interview_prep", prompt: "Interview prep", mode: "ai", generate: true}),
                  new Promise((_, reject) => {
                    timeoutId = setTimeout(() => reject(new Error("AI stalled after 90 s. Check Settings → AI suggestions.")), 90000);
                  }),
                ]);
                const sec = ((Date.now() - started) / 1000).toFixed(1);
                if (created?.content) {
                  activeDraft = created;
                  versions.push(created);
                  mdContent = created.content;
                  lastSaved = created.content;
                  editor.value = mdContent;
                  syncWC();
                  if (mode === "preview") preview.replaceChildren(renderSimpleMarkdown(mdContent));
                  drawVersions();
                  const eng = created.engine === "local" || created.engine === "remote" ? `${created.engine} AI` : "Instant";
                  prepStatus.textContent = `Generated with ${eng} in ${sec} s.`;
                }
              } catch (err) {
                const msg = err?.message || "Could not generate with AI";
                prepStatus.textContent = /unknown field mode|kind must be cover_letter or qa/i.test(msg)
                  ? "Rebuild Go backend (npm run build:go) to enable AI prep sheets."
                  : msg;
              } finally {
                clearInterval(timer);
                clearTimeout(timeoutId);
                aiGenBtn.disabled = false;
              }
            },
          }, "Generate with AI");

          const regenBtn = el("button", {
            type: "button",
            class: "quiet",
            title: "Reset editor to a fresh Instant template from this job and your resume",
            onclick: async () => {
              if (editor.value.trim() && !await window.UI.ask("Replace the editor text with a fresh prep sheet template?", {confirm: "Replace", title: "Reset prep sheet"})) return;
              editor.value = buildDefaultPrep();
              mdContent = editor.value;
              syncWC();
              if (mode === "preview") preview.replaceChildren(renderSimpleMarkdown(mdContent));
              await saveNow();
            },
          }, "Reset template");

          body.replaceChildren(
            el("div", {class: "prep-toolbar"},
              modeToggle,
              versionsBar,
              aiGenBtn,
              saveNewBtn,
              regenBtn,
              wc),
            el("p", {class: "hint", text: `Stage: ${a.status}${a.next_step_date ? ` \u00b7 Next step: ${a.next_step_date}` : ""} · Changes auto-save and stay synced with Tailor.`}),
            editor,
            preview,
            job?.raw_text ? el("details", {class: "cost-sources"},
              el("summary", {text: "Job description reference"}),
              el("pre", {class: "preview-text", text: job.raw_text.slice(0, 3000)})) : null);
        } catch (err) {
          body.replaceChildren(el("p", {class: "resume-status error", text: err.message}));
        }
      };

      const compareButton = el("button", {type: "button", title: "Compare 2\u20133 tracked offers side by side", onclick: openCompareOffers}, "Compare offers");
      const statsStrip = el("p", {class: "hint", role: "status", hidden: true});
      const dueStrip = el("section", {class: "due-strip", "aria-label": "Needs you today", hidden: true});
      const table = el("div", {class: "tracker-table-wrap"});
      const toolbar = el("header", {class: "toolbar drag", "data-tauri-drag-region": ""},
        el("div", {class: "toolbar-form resume-toolbar"},
          el("h1", {class: "view-title", text: "Tracker"}),
          status,
          el("div", {class: "toolbar-actions"}, search, compareButton, exportCsvButton, el("div", {class: "tracker-scopes", role: "group", "aria-label": "Show"}, scopeButtons))));
      fill(container, toolbar,
        el("div", {class: "pane tracker"},
          dueStrip,
          statsStrip,
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

      //opens the job on its own page (v3)
      const openJob = jobId => window.JobPage.open(jobId);

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
            notifyAppsChanged();
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
          el("td", {}, next,
            a.next_step_date ? el("button", {type: "button", class: "link-button", title: "Download .ics calendar reminder", onclick: () => {
              const d = a.next_step_date.replace(/-/g, "");
              const nextDay = new Date(a.next_step_date + "T00:00:00");
              nextDay.setDate(nextDay.getDate() + 1);
              const end = `${nextDay.getFullYear()}${String(nextDay.getMonth() + 1).padStart(2, "0")}${String(nextDay.getDate()).padStart(2, "0")}`;
              const ics = ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Fistbump//Tracker//EN", "BEGIN:VEVENT",
                `UID:fistbump-app-${a.id}@local`, `DTSTART;VALUE=DATE:${d}`, `DTEND;VALUE=DATE:${end}`,
                `SUMMARY:Follow-up: ${a.company_name || "Company"} - ${a.position_title || "Role"}`,
                `DESCRIPTION:Status: ${a.status}`, "END:VEVENT", "END:VCALENDAR", ""].join("\r\n");
              downloadBlob(`${(a.company_name || "Follow-up").replace(/[^a-z0-9_-]+/gi, "-")}.ics`, ics, "text/calendar;charset=utf-8");
            }}, ".ics") : null,
            due ? el("span", {class: "tracker-due"}, icon("clock"), a.next_step_date < today ? "Overdue" : "Due today") : null),
          el("td", {}, el("span", {text: resumeLabel(a)}), pdf,
            el("div", {class: "tracker-row-actions"},
              el("button", {type: "button", class: "link-button", title: "Edit or print interview prep sheet", onclick: () => openPrepSheet(a)}, "Prep sheet"),
              el("button", {type: "button", class: "link-button", title: "Open this job in Tailor for resume, cover letter, and Q&A", onclick: () => window.Tailor?.open?.(a.job_id)}, "Tailor"))),
          el("td", {}, notes,
            el("div", {class: "tracker-row-actions"},
              el("button", {
                type: "button",
                class: "link-button",
                title: a.archived_at ? "Restore this application to Active" : "Archive this application",
                onclick: async () => {
                  try {
                    await api.applications.bulkArchive([a.id], !a.archived_at);
                    say(a.archived_at ? "Restored application." : "Archived application.");
                    notifyAppsChanged();
                    load();
                  } catch (err) {
                    say(err.message, true);
                  }
                },
              }, a.archived_at ? "Unarchive" : "Archive"),
              el("button", {
                type: "button",
                class: "link-button danger-link",
                title: "Delete this tracked application",
                onclick: async () => {
                  const ok = await window.UI.ask(
                    `Delete "${a.company_name || "Unknown"} — ${a.position_title || "Role"}" from your Tracker?`,
                    {confirm: "Delete", danger: true, title: "Delete tracked application"}
                  );
                  if (!ok) return;
                  try {
                    await api.applications.delete(a.id);
                    say("Deleted application.");
                    notifyAppsChanged();
                    load();
                  } catch (err) {
                    say(err.message, true);
                  }
                },
              }, "Delete"))));
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

      const drawStats = () => {
        if (!rows.length) {
          statsStrip.textContent = "";
          statsStrip.hidden = true;
          return;
        }
        const countStage = name => rows.filter(a => a.status === name).length;
        const gaps = topSkillGaps.slice(0, 4).map(g => `${g.skill} (${g.count})`).join(", ");
        statsStrip.textContent = `Pipeline: ${rows.length} total · ${countStage("Applied")} applied · ${countStage("Interviewing")} interviewing · ${countStage("Offered")} offered` + (gaps ? ` · Top skill gaps: ${gaps}` : "");
        statsStrip.hidden = false;
      };

      const draw = () => {
        const today = localToday();
        if (!rows.length && statuses.size) {
          statuses.clear();
          stageChips.forEach(chip => chip.setAttribute("aria-pressed", "false"));
        }
        const shown = statuses.size ? rows.filter(a => statuses.has(a.status)) : rows;
        drawBulk();
        drawDue(today);
        drawStats();
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
          if (!rows.length) topSkillGaps = [];
          for (const id of [...selected]) if (!rows.some(a => a.id === id)) selected.delete(id);
          draw();
          if (rows.length && window.api?.applications?.stats) {
            api.applications.stats().then(st => {
              if (mine !== loadSeq) return;
              topSkillGaps = (st?.top_missing_skills || []).slice(0, 4);
              drawStats();
            }).catch(() => {
              if (mine !== loadSeq) return;
              topSkillGaps = [];
              drawStats();
            });
          }
        } catch (err) {
          if (mine !== loadSeq) return;
          say(err.message, true);
        }
      };

      const onDataChange = event => {
        if (event?.detail?.source === "tracker") return;
        topSkillGaps = [];
        versionNames.clear();
        tailoredNames.clear();
        if (!container.hidden) {
          load();
        } else {
          rows = [];
          selected.clear();
          draw();
        }
      };
      window.addEventListener("fistbump:jobs", onDataChange);
      window.addEventListener("fistbump:applications", onDataChange);
      window.addEventListener("fistbump:storage-cleaned", onDataChange);
      window.addEventListener("fistbump:versions", onDataChange);

      load();
      return load; //drawn again whenever the view is shown
    },
  };
})();

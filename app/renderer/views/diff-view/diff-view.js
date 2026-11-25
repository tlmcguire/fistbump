//the tailor view. pick a saved job, generate suggestions, decide on each one, build the tailored resume,
//and see it as changes or as the PDF it will export
(() => {
  window.Tailor = {
    jobId: null,

    //jobs view calls this from "Tailor resume".
    open(id) {
      this.jobId = id;
      window.App.go("tailor");
    },

    mount(container) {
      const {el, fill, exportFile, savedNote} = window.ResumeBuilder;
      const {icon, pico, ask, confirmFirst, motion, play, stagger, countTo} = window.UI;
      const api = window.FistBumpAPI;
      const SECTION = {header: "Header", summary: "Summary", experience: "Experience", education: "Education", skills: "Skills"};
      const CHANGE = {changed: "Changed", added: "Added", removed: "Removed"};
      const STATUS = {queued: "Starting", running: "Writing", done: "Ready", failed: "Failed", canceled: "Canceled"};
      const ENGINE = {local: "Local model", remote: "Remote provider", rules: "Built-in rules"};
      const STATE = {
        pending: {label: "To review", icon: "clock"},
        accepted: {label: "Accepted", icon: "check"},
        rejected: {label: "Rejected", icon: "x"},
        edited: {label: "Edited by you", icon: "pencil"},
      };

      let jobId = window.Tailor.jobId;
      let revId = null;       //the round being reviewed
      let tailoredId = null;  //set while a tailored resume is open
      let showDrafts = false; //true while viewing Answers & Cover Letter for the chosen job
      const warned = new Map(); //warnings from building a tailored resume, by its id
      let rounds = [];        //newest first, each with its number n
      let tailored = [];
      let savedJobs = [];     //the saved jobs, so the empty page can offer one to start from
      const DISMISSED_KEY = "fistbump.tailor-dismissed-jobs";
      const loadDismissed = () => {
        try {
          const raw = JSON.parse(localStorage.getItem(DISMISSED_KEY) || "[]");
          return new Set(Array.isArray(raw) ? raw.map(Number).filter(Number.isFinite) : []);
        } catch { return new Set(); }
      };
      const dismissedJobs = loadDismissed();
      const saveDismissed = () => {
        try { localStorage.setItem(DISMISSED_KEY, JSON.stringify([...dismissedJobs])); } catch {}
      };
      let problem = null;     //what went wrong, when a round could not be written
      let poll = null;
      let nextTab = "changes"; //the tab a tailored resume opens on: the editor, when it was just started by hand
      let gaps = [];          //skills the job asks for that the master resume lacks
      let experienceTitles = {}; //experience id -> "Title, Company" for clear card headings
      let cards = [];         //the cards on screen, for the keyboard
      let current = 0;
      let pdf = null;         //the open PDF preview, so its blob is let go

      // ---------- shell ----------
      const status = el("p", {class: "resume-status", role: "status"});
      const say = (message, isError = false) => {
        status.textContent = message;
        status.classList.toggle("error", isError);
      };
      //runs fn. a failure goes to the status line and gives null, success gives {value}.
      //onError, when it is given, takes the error instead of the status line.
      const attempt = async (fn, onError) => {
        try {
          return {value: await fn()};
        } catch (error) {
          if (onError) onError(error);
          else say(error.message, true);
          return null;
        }
      };

      const jobSelect = el("select", {id: "tailor-job"});
      const baseSelect = el("select", {id: "tailor-base"});
      const generate = el("button", {type: "button", class: "primary", title: "Write a new round of suggestions for the chosen job", onclick: () => { showDrafts = false; startRound(); }}, "Generate suggestions");
      const ownEdit = el("button", {type: "button", title: "Start from your resume and edit it yourself, with no suggestions", onclick: () => { showDrafts = false; startOwnEdit(); }}, "Edit it myself");
      const draftsBtn = el("button", {type: "button", title: "Write a cover letter or application Q&A answers for this job", onclick: () => { showDrafts = !showDrafts; drawBody(); }}, "Cover letter & Q&A");
      const engineChip = el("button", {type: "button", class: "engine-chip", onclick: () => window.App.openTab("tab-settings")});
      const field = (id, label, select) => el("div", {class: "picker"}, el("label", {for: id, text: label}), select);

      const toolbar = el("header", {class: "toolbar drag", "data-tauri-drag-region": ""},
        el("div", {class: "toolbar-form tailor-toolbar"},
          el("h1", {class: "view-title", text: "Tailor"}),
          engineChip,
          status,
          el("div", {class: "toolbar-actions"},
            field("tailor-job", "Job", jobSelect),
            field("tailor-base", "Based on", baseSelect),
            draftsBtn,
            ownEdit,
            generate)));

      const roundsBar = el("div", {class: "tailor-rounds", role: "group", "aria-label": "Rounds", title: "Each round is one set of suggestions. Start another to compare them."});
      const stage = el("div", {class: "tailor-stage"});
      const mainPane = el("div", {class: "pane tailor-main"}, roundsBar, stage);
      const aside = el("aside", {class: "pane tailor-aside", "aria-label": "About this job"});
      fill(container, toolbar, el("div", {class: "content tailor-content"}, mainPane, aside));

      //Picks the job to tailor for: the one in the toolbar, or one of the saved jobs on the empty page.
      const chooseJob = async id => {
        jobId = id || null;
        window.Tailor.jobId = jobId;
        revId = null;
        tailoredId = null;
        problem = null;
        say("");
        jobSelect.value = jobId ? String(jobId) : "";
        await swapJob();
      };

      //Changing the job: the old job's page goes at once, so it is never seen beside the new one, then the
      //rounds, the review and the sidebar are loaded together and drawn in one go.
      const swapJob = async () => {
        stopPolling();
        releasePdf();
        cards = [];
        stage.replaceChildren();
        roundsBar.replaceChildren();
        await Promise.all([loadRounds(false), drawAside()]);
        await drawBody();
      };

      jobSelect.addEventListener("change", () => chooseJob(jobSelect.value ? Number(jobSelect.value) : null));

      // ---------- which engine will write ----------
      const drawEngine = s => {
        const mode = s?.mode || "auto";
        const engine = s?.active_engine || (mode === "rules" ? "rules" : "");
        const text = engine === "local" ? "On this computer"
          : engine === "remote" ? "Remote: your provider"
          : engine === "rules" ? "Built-in rules, no AI"
          : "No engine ready";
        const note = engine === "remote" ? "Your resume text goes to the provider you set up." : "Nothing leaves this computer.";
        engineChip.dataset.engine = engine || "none";
        engineChip.title = `${note} Click to change.`; //"{0} Click to change." in the language files
        engineChip.setAttribute("aria-label", `Suggestions written by: ${text}. ${note} Open settings.`);
        fill(engineChip, icon(engine === "remote" ? "cloud" : "shield"), el("span", {text}));
      };
      const loadEngine = async () => {
        try {
          drawEngine(await api.ai.status());
        } catch {
          drawEngine(null);
        }
      };
      window.addEventListener("fistbump:ai", event => drawEngine(event.detail));

      // ---------- data ----------
      const loadPickers = async () => {
        const [jobs, versions] = await Promise.all([attempt(() => api.jobs.list("")), attempt(() => api.resumes.list())]);
        savedJobs = jobs?.value || [];
        fill(jobSelect,
          el("option", {value: ""}, "Choose a saved job"),
          (jobs?.value || []).map(j => el("option", {value: j.id}, `${j.position_title} at ${j.company_name}`)));
        fill(baseSelect,
          el("option", {value: ""}, "Master resume"),
          (versions?.value || []).map(v => el("option", {value: v.id}, v.version_label)));
        jobSelect.value = jobId ? String(jobId) : "";
      };

      const loadRounds = async (draw = true) => {
        generate.disabled = !jobId;
        ownEdit.disabled = !jobId;
        draftsBtn.disabled = !jobId;
        if (!jobId) showDrafts = false;
        if (!jobId) {
          rounds = [];
          tailored = [];
        } else {
          const [revs, tl] = await Promise.all([attempt(() => api.revisions.list(jobId)), attempt(() => api.tailored.list(jobId))]);
          rounds = (revs?.value || []).map((r, i, all) => ({...r, n: all.length - i})); //the API lists newest first
          tailored = tl?.value || [];
        }
        if (!rounds.some(r => r.id === revId)) revId = rounds[0]?.id ?? null;
        if (draw) drawRounds();
      };

      //Why a round failed, in words someone can act on. The backend's own words stay as the reason.
      const explain = reason => {
        const text = String(reason || "").trim();
        const low = text.toLowerCase();
        const known = [
          [/profile/, "Your master resume is not ready", ["Save your master resume first, then try again."], false, false],
          [/all engines failed/, "Every suggestion engine failed", ["The reason is below. Try again, or use the built-in rules from Settings."], true, true],
          [/unavailable|not available|no engine/, "No engine is ready to write suggestions", ["Set up the local model or a remote provider in Settings, or use the built-in rules."], true, false],
          [/interrupted/, "The round was interrupted", ["The app closed while the suggestions were being written. Try again."], true, true],
          [/empty reply/, "The model sent an empty reply", ["Try again, or use the built-in rules from Settings."], true, true],
          [/provider|api key|unauthorized|forbidden/, "Your remote provider refused the request", ["Check the address, the model name and the key in Settings."], true, true],
          [/llama-server|local model/, "The local model could not be started", ["Check that the model is downloaded, then try again."], true, true],
          //a database or backend fault: the suggestions were written but could not be saved or read
          [/sql|database|no such (table|column)|no column named|went wrong inside/, "FistBump could not save the suggestions", ["Restart FistBump and try again.", "If it keeps failing, install the latest version. It repairs the database when it opens."], false, true],
        ];
        for (const [re, title, steps, settings, retry] of known) {
          if (re.test(low)) return {title, steps, settings, retry, reason: text};
        }
        return {title: "Suggestions could not be written", steps: ["Try again. If it keeps failing, use the built-in rules from Settings."], settings: true, retry: true, reason: text};
      };

      //The panel that says what went wrong and offers the next step.
      const problemPanel = p => el("div", {class: "stage-problem", role: "alert"},
        pico("29-something-went-wrong.png"),
        el("h2", {text: p.title}),
        p.reason ? el("p", {class: "problem-reason", text: p.reason}) : null,
        el("ul", {class: "problem-steps"}, p.steps.map(step => el("li", {text: step}))),
        el("div", {class: "round-actions"},
          p.retry ? el("button", {type: "button", class: "primary", onclick: () => startRound()}, "Try again") : null,
          p.settings ? el("button", {type: "button", onclick: () => window.App.openTab("tab-settings")}, "Open settings") : null));

      //a copy of the resume for this job that is edited by hand, with no suggestions behind it
      const startOwnEdit = async () => {
        const created = await attempt(() => api.tailored.createBlank(jobId, baseSelect.value ? Number(baseSelect.value) : null));
        if (!created) return;
        say("");
        tailoredId = created.value.id;
        nextTab = "edit";
        await loadRounds(false);
        drawBody();
      };

      const startRound = async () => {
        const request = {job_id: jobId, engine: "auto", language: window.I18n?.locale() || "en"};
        if (baseSelect.value) request.base_resume_id = Number(baseSelect.value);
        problem = null;
        const created = await attempt(() => api.revisions.create(request), err => { problem = explain(err.message); });
        if (!created) {
          drawBody();
          return;
        }
        say("");
        revId = created.value.id;
        tailoredId = null;
        await loadRounds();
        drawBody();
      };

      // ---------- the job beside the suggestions ----------
      let asideSeq = 0;
      let asideJob = null; //the job the sidebar last arrived for
      const drawAside = async () => {
        const mine = ++asideSeq;
        gaps = [];
        if (!jobId) {
          asideJob = null;
          fill(aside, el("div", {class: "aside-empty"}, pico("16-skill-gap.png"), el("p", {text: "Pick a saved job and its skill gaps show here while you review."})));
          return;
        }
        const [job, analysis, master] = await Promise.all([attempt(() => api.jobs.get(jobId)), attempt(() => api.jobs.analyze(jobId)), attempt(() => api.resume.get())]);
        if (mine !== asideSeq) return;
        if (!job) return;
        const a = analysis?.value;
        const missing = a ? [...a.required.tech.missing, ...a.required.soft.missing] : [];
        const preferredMissing = a ? [...a.preferred.tech.missing, ...a.preferred.soft.missing] : [];
        gaps = [...missing, ...preferredMissing].map(String);
        const prefGroups = a ? [a.preferred.tech, a.preferred.soft] : [];
        const preferredHave = prefGroups.reduce((n, g) => n + g.matched.length, 0);
        const preferredTotal = prefGroups.reduce((n, g) => n + g.matched.length + (g.likely || []).length + g.missing.length, 0);
        const titles = Object.fromEntries((master?.value?.experiences || []).map(x => [x.id, `${x.job_title}, ${x.company_name}`]));
        experienceTitles = titles;
        const match = a ? window.ResumeBuilder.skillMatch(a, titles) : null;
        const score = a ? Math.round(a.score * 100) : null;
        const scoreNode = el("span", {class: "score-number", text: String(score ?? 0)});
        const ring = el("div", {class: "score-ring"}, scoreNode, "%");
        ring.style.setProperty("--pct", String(score ?? 0)); //a style attribute would break the page's CSP
        const bridgeBox = missing.length ? buildKeywordBridge(missing) : null;
        fill(aside,
          el("h2", {text: job.value.position_title}),
          el("p", {class: "aside-company", text: job.value.company_name}),
          a ? el("div", {class: "score"}, ring,
            el("p", {text: "of required skills are already on your resume"})) : null,
          a && preferredTotal ? el("p", {class: "aside-preferred", text: `${preferredHave} of ${preferredTotal} nice-to-have skills are on your resume`}) : null,
          match ? el("div", {class: "skill-match"}, el("h3", {text: match.heading}), ...match.groups) : null,
          bridgeBox,
          !a ? el("p", {class: "hint", text: analysis === null ? "" : "Add your master resume to see how it matches."}) : null,
          el("p", {class: "hint", text: "Suggestions only reword what is already true. Skills you have not used stay missing until you add them yourself."}));
        if (a && asideJob !== jobId) play(aside, "in", 220);
        asideJob = jobId;
        if (revId !== null && !tailoredId) markGaps();
      };

      // Transparent, visible keyword gap-bridging (no hidden text): lets candidates honestly list missing keywords
      // as "Currently learning" or "Don't have yet (eager to learn)" so automated filters pass them to a human.
      const BRIDGE_STYLES = [
        ["learning", "Currently learning"],
        ["dont_have", "Don't have yet (eager to learn)"],
        ["exploring", "Familiar with / ramping up on"],
      ];
      const buildKeywordBridge = missingSkills => {
        const picked = new Set(missingSkills.slice(0, 6));
        const styleSel = el("select", {"aria-label": "Honest phrasing style"},
          BRIDGE_STYLES.map(([v, label]) => el("option", {value: v, text: label})));
        const preview = el("textarea", {rows: 2, "aria-label": "Visible skill note preview"});
        const noteStatus = el("p", {class: "hint", role: "status"});
        const updatePreview = () => {
          const list = missingSkills.filter(s => picked.has(s));
          const prefix = BRIDGE_STYLES.find(([v]) => v === styleSel.value)?.[1] || "Currently learning";
          preview.value = list.length ? `${prefix}: ${list.join(", ")}` : "";
        };
        styleSel.addEventListener("change", updatePreview);
        const skillChecks = el("div", {class: "chips"}, missingSkills.slice(0, 10).map(skill => {
          const cb = el("input", {type: "checkbox", checked: picked.has(skill)});
          cb.addEventListener("change", () => {
            if (cb.checked) picked.add(skill); else picked.delete(skill);
            updatePreview();
          });
          return el("label", {class: "chip skill skill-gap"}, cb, " ", skill);
        }));
        updatePreview();
        const insertLineIntoMarkdown = (md, line) => {
          const clean = line.trim();
          if (!clean) return md;
          const rx = /^\s*(?:Currently learning|Don't have yet \(eager to learn\)|Familiar with \/ ramping up on):.*$/im;
          if (rx.test(md)) return md.replace(rx, clean);
          if (/^##\s+Skills\s*$/im.test(md)) {
            return md.replace(/(^##\s+Skills\s*\n(?:[^#]*))/im, (m) => m.trimEnd() + "\n\n" + clean + "\n\n");
          }
          return md.trimEnd() + "\n\n## Skills\n\n" + clean + "\n";
        };
        const copyBtn = el("button", {type: "button", onclick: async () => {
          if (!preview.value.trim()) return;
          try { await navigator.clipboard.writeText(preview.value.trim()); noteStatus.textContent = "Copied visible note."; }
          catch { noteStatus.textContent = "Select and copy the text above."; }
        }}, "Copy line");
        const addBtn = el("button", {type: "button", class: "primary", onclick: async () => {
          const line = preview.value.trim();
          if (!line) return;
          addBtn.disabled = true;
          try {
            let targetId = tailoredId;
            if (!targetId) {
              const created = await attempt(() => api.tailored.createBlank(jobId, baseSelect.value ? Number(baseSelect.value) : null));
              if (!created) return;
              targetId = created.value.id;
            }
            const cur = await attempt(() => api.tailored.get(targetId));
            if (!cur) return;
            const nextContent = insertLineIntoMarkdown(cur.value.content || "", line);
            const saved = await attempt(() => api.tailored.update(targetId, nextContent));
            if (saved) {
              tailoredId = targetId;
              showDrafts = false;
              nextTab = "changes";
              await loadRounds(false);
              await drawBody();
              noteStatus.textContent = "Added visible note under Skills.";
            }
          } finally {
            addBtn.disabled = false;
          }
        }}, "Add to tailored resume");
        return el("div", {class: "cost-card keyword-bridge"},
          el("h3", {text: "Honest keyword bridge"}),
          el("p", {class: "hint", text: "Visible, honest phrasing (no hidden text) so automated keyword filters pass your resume to a human."}),
          skillChecks,
          styleSel,
          preview,
          el("div", {class: "round-actions"}, copyBtn, addBtn),
          noteStatus);
      };

      //skills a suggestion brings in that the job asks for
      const bringsIn = s => {
        const text = String(s.state === "edited" ? s.edited_text : s.proposed_text).toLowerCase();
        const before = String(s.original_text || "").toLowerCase();
        return gaps.filter(g => g && text.includes(g.toLowerCase()) && !before.includes(g.toLowerCase()));
      };
      const markGaps = () => {
        for (const c of cards) {
          const list = c.node.querySelector(".brings-in");
          if (list) list.replaceChildren(...bringsIn(c.s).map(g => el("li", {class: "chip gap", text: g})));
        }
      };

      //polling, once a second until the round is done, failed or canceled
      const stopPolling = () => {
        clearInterval(poll);
        poll = null;
      };
      const watch = id => {
        stopPolling();
        const started = Date.now();
        poll = setInterval(async () => {
          const elapsed = document.getElementById("tailor-elapsed");
          if (elapsed) elapsed.textContent = `${Math.round((Date.now() - started) / 1000)} s`;
          const r = await attempt(() => api.revisions.get(id));
          if (r && ["queued", "running"].includes(r.value.status)) return;
          stopPolling();
          if (revId === id && !tailoredId) {
            await loadRounds();
            drawBody();
          }
        }, 1000);
      };

      // ---------- rounds ----------
      //Each round is a chip with its own little x, so a round can be removed from here.
      //resumes edited by hand, or whose round was deleted, have no round chip of their own
      const ownDrafts = () => tailored.filter(t => !rounds.some(r => r.id === t.revision_id));

      const drawRounds = () => fill(roundsBar, [...rounds.map(r => {
        const remove = el("button", {type: "button", class: "round-delete", title: "Delete round", "aria-label": `Delete round ${r.n}`}, icon("x"));
        remove.addEventListener("click", async event => {
          event.stopPropagation();
          if (!await ask(`Delete round ${r.n}? Its suggestions are removed and cannot be brought back.`, {confirm: "Delete", danger: true, title: "Delete round?"})) return;
          const done = await attempt(() => api.revisions.delete(r.id));
          if (!done) return;
          if (revId === r.id) revId = null;
          await loadRounds();
          drawBody();
        });
        const chip = el("button", {
          type: "button",
          class: "tailor-round",
          "aria-pressed": String(r.id === revId && !tailoredId),
          onclick: () => { revId = r.id; tailoredId = null; drawRounds(); drawBody(); },
        }, `Round ${r.n}`, el("span", {class: "round-meta", text: `${STATUS[r.status] || r.status} · ${ENGINE[r.engine] || r.engine}`}));
        return el("div", {class: "tailor-round-item"}, chip, remove);
      }), ...ownDrafts().map((t, i, all) => {
        const label = `Your edit ${all.length - i}`;
        const remove = el("button", {type: "button", class: "round-delete", title: "Delete edit", "aria-label": `Delete ${label.toLowerCase()}`}, icon("x"));
        remove.addEventListener("click", async event => {
          event.stopPropagation();
          if (!await ask(`Delete ${label.toLowerCase()}? The resume text you wrote is removed and cannot be brought back.`, {confirm: "Delete", danger: true, title: "Delete edit?"})) return;
          if (!await attempt(() => api.tailored.delete(t.id))) return;
          if (tailoredId === t.id) tailoredId = null;
          await loadRounds();
          drawBody();
        });
        const chip = el("button", {
          type: "button", class: "tailor-round", "aria-pressed": String(t.id === tailoredId),
          onclick: () => { tailoredId = t.id; drawRounds(); drawBody(); },
        }, label, el("span", {class: "round-meta", text: "Edited by you"}));
        return el("div", {class: "tailor-round-item"}, chip, remove);
      })]);

      // ---------- review ----------
      const releasePdf = () => { pdf?.dispose(); pdf = null; };

      //The saved jobs on the empty page: a way in without the dropdown.
      const suggestedJobs = () => {
        const visible = savedJobs.filter(j => !dismissedJobs.has(Number(j.id)));
        const hasDismissed = savedJobs.some(j => dismissedJobs.has(Number(j.id)));
        if (!visible.length) {
          return el("div", {class: "stage-jobs"},
            el("button", {type: "button", class: "quiet", onclick: () => {
              dismissedJobs.clear();
              saveDismissed();
              drawBody();
            }}, "Show dismissed job suggestions"));
        }
        return el("div", {class: "stage-jobs"},
          el("p", {class: "hint", text: "Or start with one of your saved jobs:"}),
          el("div", {class: "stage-job-list"}, visible.slice(0, 6).map(j => {
            const title = j.position_title || j.company_name || "";
            const company = j.position_title ? j.company_name : "";
            const dismiss = el("button", {
              type: "button",
              class: "round-delete stage-job-dismiss",
              title: "Dismiss suggestion",
              "aria-label": `Dismiss ${title}`,
            }, icon("x"));
            dismiss.addEventListener("click", event => {
              event.stopPropagation();
              dismissedJobs.add(Number(j.id));
              saveDismissed();
              drawBody();
            });
            const cardBtn = el("button", {type: "button", class: "stage-job", translate: "no", onclick: () => chooseJob(j.id)},
              el("strong", {text: title}),
              company ? el("span", {text: company}) : null,
              j.location ? el("small", {text: j.location}) : null);
            return el("div", {class: "stage-job-item"}, cardBtn, dismiss);
          })),
          hasDismissed ? el("button", {type: "button", class: "quiet", onclick: () => {
            dismissedJobs.clear();
            saveDismissed();
            drawBody();
          }}, "Show dismissed suggestions") : null);
      };

      //No saved jobs yet: point at the place where they come from.
      const noJobsYet = () => el("div", {},
        el("p", {class: "hint", text: "You have no saved jobs yet. Save one in the Jobs view first."}),
        el("button", {type: "button", onclick: () => window.App.go("jobs")}, "Find jobs"));

      const selectedDraftByGroup = new Map(); // groupKey -> draft id
      const downloadDraftFile = (filename, content, mime) => {
        const blob = new Blob([content], {type: mime});
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = filename;
        a.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
      };
      const safeFileSlug = text => String(text || "draft").replace(/[^a-z0-9_-]+/gi, "-").replace(/^-+|-+$/g, "").slice(0, 48) || "draft";
      const draftWordHtml = (title, body) => {
        const esc = s => String(s || "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
        const paras = String(body || "").split(/\n\s*\n/).map(p => `<p style="margin:0 0 11pt 0;line-height:1.45;">${esc(p).replace(/\n/g, "<br/>")}</p>`).join("");
        return `<!doctype html><html><head><meta charset="utf-8"><title>${esc(title)}</title></head><body style="font-family:Georgia,serif;font-size:11pt;color:#111827;margin:0.8in;">${paras}</body></html>`;
      };

      const drawDrafts = async () => {
        drawRounds();
        const listRes = await attempt(() => api.drafts.list(jobId));
        const drafts = listRes?.value || [];
        const activeJob = savedJobs.find(j => Number(j.id) === Number(jobId));
        const jobSlug = safeFileSlug(activeJob ? `${activeJob.company_name || ""}-${activeJob.position_title || ""}` : "application");

        // Group drafts by (kind, prompt) so multiple saved versions live together in one versioned card.
        const groupOrder = [];
        const groups = new Map();
        const chrono = [...drafts].sort((a, b) => (a.created_at < b.created_at ? -1 : a.created_at > b.created_at ? 1 : a.id - b.id));
        for (const d of chrono) {
          const key = `${d.kind}::${String(d.prompt || "").trim().toLowerCase()}`;
          if (!groups.has(key)) {
            groups.set(key, {key, kind: d.kind, prompt: d.prompt, versions: []});
            groupOrder.push(key);
          }
          groups.get(key).versions.push(d);
        }
        groupOrder.sort((ka, kb) => {
          const la = groups.get(ka).versions.at(-1);
          const lb = groups.get(kb).versions.at(-1);
          return la.updated_at < lb.updated_at ? 1 : la.updated_at > lb.updated_at ? -1 : lb.id - la.id;
        });

        const customPrompt = el("input", {type: "text", placeholder: "Custom question, e.g. Why do you want to work here?"});
        const gen = async (kind, prompt) => {
          const created = await attempt(() => api.drafts.create(jobId, {kind, prompt, generate: true}));
          if (created?.value) {
            selectedDraftByGroup.set(`${kind}::${prompt.trim().toLowerCase()}`, created.value.id);
            drawDrafts();
          }
        };
        const exportAllBtn = groupOrder.length ? el("button", {type: "button", title: "Download the latest version of every draft for this job as one Markdown file", onclick: () => {
          const sections = groupOrder.map(k => {
            const g = groups.get(k);
            const latest = g.versions.at(-1);
            return `## ${g.prompt} (v${g.versions.length})\n\n${latest.content.trim()}`;
          });
          const title = activeJob ? `# Application Drafts — ${activeJob.position_title} at ${activeJob.company_name}` : "# Application Drafts";
          downloadDraftFile(`${jobSlug}-drafts.md`, `${title}\n\n${sections.join("\n\n---\n\n")}\n`, "text/markdown;charset=utf-8");
          say("Exported all drafts as Markdown.");
        }}, "Export all (.md)") : null;

        const quickBar = el("div", {class: "round-actions"},
          el("button", {type: "button", class: "primary", onclick: () => gen("cover_letter", "Cover letter")}, "Generate cover letter"),
          el("button", {type: "button", onclick: () => gen("qa", "Why this role?")}, "Why this role?"),
          el("button", {type: "button", onclick: () => gen("qa", "Describe a relevant project")}, "Relevant project"),
          exportAllBtn,
          el("button", {type: "button", onclick: () => { showDrafts = false; drawBody(); }}, "Back to resume"));
        const customForm = el("div", {class: "round-actions draft-custom-form"},
          customPrompt,
          el("button", {type: "button", onclick: () => {
            const q = customPrompt.value.trim();
            if (!q) return;
            gen("qa", q);
          }}, "Draft answer"));

        const ITERATE_CHIPS = [
          ["Shorter", "Make it shorter and more concise"],
          ["More detailed", "Make it more detailed with another concrete highlight from my resume"],
          ["Stronger opening", "Give it a stronger opening hook"],
          ["More formal", "Make the tone more formal and polished"],
          ["More conversational", "Make the tone warmer and more conversational"],
          ["Highlight impact", "Highlight quantified impact and measurable results"],
        ];

        const cardsEl = groupOrder.length ? groupOrder.map(key => {
          const group = groups.get(key);
          const wantedId = selectedDraftByGroup.get(key);
          const active = group.versions.find(v => v.id === wantedId) || group.versions.at(-1);
          const activeIdx = group.versions.indexOf(active);
          selectedDraftByGroup.set(key, active.id);

          const area = el("textarea", {rows: 8, value: active.content});
          area.value = active.content;
          const wc = el("span", {class: "hint"});
          const syncWC = () => {
            const n = area.value.trim() ? area.value.trim().split(/\s+/).length : 0;
            wc.textContent = `${n} word${n === 1 ? "" : "s"}`;
          };
          syncWC();
          area.addEventListener("input", syncWC);
          const copyStatus = el("span", {class: "hint"});
          const fileBase = `${jobSlug}-${safeFileSlug(group.prompt)}-v${activeIdx + 1}`;

          const versionTabs = el("div", {class: "draft-versions", role: "group", "aria-label": "Saved versions"},
            group.versions.map((v, i) => el("button", {
              type: "button",
              class: "draft-version-chip",
              "aria-pressed": String(v.id === active.id),
              title: v.updated_at ? `Updated ${new Date(v.updated_at).toLocaleString()}` : `Version ${i + 1}`,
              onclick: () => {
                selectedDraftByGroup.set(key, v.id);
                drawDrafts();
              },
            }, `v${i + 1}`)));

          const iterateInput = el("input", {
            type: "text",
            class: "draft-iterate-input",
            placeholder: "Iterate on this draft, e.g. emphasize Python & leadership…",
          });
          const runIterate = async instruction => {
            const clean = String(instruction || "").trim();
            if (!clean) return;
            copyStatus.textContent = "Creating new version…";
            const created = await attempt(() => api.drafts.create(jobId, {
              kind: group.kind,
              prompt: group.prompt,
              base_content: area.value,
              instruction: clean,
              generate: true,
            }));
            if (created?.value) {
              selectedDraftByGroup.set(key, created.value.id);
              drawDrafts();
            }
          };

          const iterateRow = el("div", {class: "draft-iterate"},
            el("div", {class: "draft-iterate-chips"},
              ITERATE_CHIPS.map(([label, inst]) => el("button", {
                type: "button",
                class: "quiet draft-chip",
                onclick: () => runIterate(inst),
              }, label))),
            el("div", {class: "draft-iterate-bar"},
              iterateInput,
              el("button", {type: "button", onclick: () => runIterate(iterateInput.value)}, "Iterate as new version")));

          return el("section", {class: "tailor-card draft-card"},
            el("div", {class: "card-head"},
              el("h3", {text: group.prompt}),
              el("span", {class: "engine-badge", text: group.kind === "cover_letter" ? "Cover letter" : "Q&A"}),
              versionTabs,
              wc),
            area,
            iterateRow,
            el("div", {class: "tailor-actions"},
              el("button", {type: "button", class: "primary", onclick: async () => {
                if (await attempt(() => api.drafts.update(jobId, active.id, {prompt: group.prompt, content: area.value}))) copyStatus.textContent = `Saved v${activeIdx + 1}.`;
              }}, `Save v${activeIdx + 1}`),
              el("button", {type: "button", title: "Keep the current version and save your edits as a new version", onclick: async () => {
                if (!area.value.trim()) return;
                const created = await attempt(() => api.drafts.create(jobId, {kind: group.kind, prompt: group.prompt, content: area.value, generate: false}));
                if (created?.value) {
                  selectedDraftByGroup.set(key, created.value.id);
                  drawDrafts();
                }
              }}, "Save as new version"),
              el("button", {type: "button", onclick: async () => {
                try { await navigator.clipboard.writeText(area.value); copyStatus.textContent = "Copied."; } catch {}
              }}, "Copy"),
              el("button", {type: "button", onclick: () => {
                downloadDraftFile(`${fileBase}.txt`, area.value.trim() + "\n", "text/plain;charset=utf-8");
                copyStatus.textContent = "Exported .txt.";
              }}, "Export .txt"),
              el("button", {type: "button", onclick: () => {
                downloadDraftFile(`${fileBase}.md`, `# ${group.prompt}\n\n${area.value.trim()}\n`, "text/markdown;charset=utf-8");
                copyStatus.textContent = "Exported .md.";
              }}, "Export .md"),
              el("button", {type: "button", onclick: () => {
                downloadDraftFile(`${fileBase}.doc`, draftWordHtml(group.prompt, area.value), "application/msword;charset=utf-8");
                copyStatus.textContent = "Exported .doc.";
              }}, "Export .doc"),
              el("button", {type: "button", class: "quiet", onclick: async () => {
                if (await attempt(() => api.drafts.delete(jobId, active.id))) {
                  selectedDraftByGroup.delete(key);
                  drawDrafts();
                }
              }}, group.versions.length > 1 ? `Delete v${activeIdx + 1}` : "Delete"),
              copyStatus));
        }) : [el("p", {class: "hint", text: "No drafts yet. Generate a resume-grounded cover letter or application Q&A answer above."})];

        fill(stage,
          el("div", {class: "round-head"}, el("h2", {text: "Cover letter & application Q&A"}), quickBar),
          customForm,
          el("div", {class: "cards"}, cardsEl));
      };

      const drawBody = async () => {
        stopPolling();
        releasePdf();
        cards = [];
        if (showDrafts && jobId) {
          return drawDrafts();
        }
        if (tailoredId) {
          const tab = nextTab;
          nextTab = "changes";
          return drawTailored(tab);
        }
        drawRounds();
        if (problem) {
          fill(stage, problemPanel(problem));
          play(stage, "enter", 500);
          return;
        }
        if (!jobId || revId === null) {
          const empty = el("div", {class: "stage-empty"},
            pico(jobId ? "17-tailoring-in-progress.png" : "14-writing.png"),
            el("h2", {text: jobId ? "Nothing tailored for this job yet" : "Pick a job to tailor your resume for"}),
            el("p", {text: jobId
              ? "Press Generate suggestions and approve each change, or press Edit it myself to change your resume directly."
              : "Choose a saved job above. Nothing changes in your resume until you accept it."}));
          //with no job chosen, the saved jobs are offered here, so there is something to start from
          if (!jobId) empty.append(savedJobs.length ? suggestedJobs() : noJobsYet());
          fill(stage, empty);
          play(stage, "enter", 500);
          return;
        }
        const asked = revId;
        const r = await attempt(() => api.revisions.get(asked));
        if (r && r.value.id === revId && asked === revId) show(r.value);
      };

      const show = rev => {
        const round = rounds.find(x => x.id === rev.id);
        const busy = rev.status === "queued" || rev.status === "running";
        const draft = tailored.find(t => t.revision_id === rev.id);
        if (busy) watch(rev.id);
        cards = [];
        const roundActions = el("div", {class: "round-actions"},
          draft && el("button", {type: "button", class: "primary", onclick: () => { tailoredId = draft.id; drawRounds(); drawBody(); }}, "View tailored resume"),
          busy && el("button", {type: "button", onclick: async () => { await attempt(() => api.revisions.cancel(rev.id)); await loadRounds(); drawBody(); }}, "Cancel"),
          confirmFirst(el("button", {type: "button", class: "quiet"}, "Delete round"), "Delete round?", async () => {
            if (await attempt(() => api.revisions.delete(rev.id))) { revId = null; await loadRounds(); drawBody(); }
          }));
        fill(stage,
          el("div", {class: "round-head"},
            el("h2", {text: `Round ${round?.n ?? ""}`}),
            el("span", {class: "round-meta", text: `${STATUS[rev.status] || rev.status} · ${ENGINE[rev.engine] || rev.engine}`}),
            roundActions),
          rev.status === "failed" ? problemPanel(explain(rev.error)) : null,
          busy && writing(rev),
          rev.status === "done" && suggestions(rev));
        if (!busy && (rev.status !== "done" || !rev.suggestions?.length)) play(stage, "enter", 220);
      };

      //the wait: Pico thinking, a clock, and cards taking shape
      const writing = rev => {
        const thinking = motion()
          ? el("img", {class: "mascot thinking", src: "views/graphics/mascots/animations/thinking-anim.png", alt: "", "aria-hidden": "true"})
          : pico("animations/thinking.png");
        return el("div", {class: "writing", role: "status"},
          thinking,
          el("div", {},
            el("p", {class: "writing-title", text: `Writing suggestions with the ${(ENGINE[rev.engine] || "engine").toLowerCase()}`}),
            el("p", {class: "hint"}, el("span", {id: "tailor-elapsed", text: "0 s"}), " so far. Nothing is changed in your resume yet.")),
          el("div", {class: "skeletons", "aria-hidden": "true"}, [0, 1, 2].map(() => el("div", {class: "skeleton"}))));
      };

      const suggestions = rev => {
        const list = rev.suggestions;
        if (!list.length) {
          return el("div", {class: "stage-empty"}, pico("34-no-changes.png"),
            el("h2", {text: "Your resume already fits this job"}),
            el("p", {text: "No wording changes found. Try another round, or a different job."}));
        }
        const pending = () => list.filter(s => s.state === "pending");

        const acceptAll = async () => {
          for (const s of pending()) {
            const r = await attempt(() => api.suggestions.accept(rev.id, s.id));
            if (!r) break;
            Object.assign(s, r.value);
          }
          show(rev);
        };

        const countText = el("strong");
        const countBox = el("div", {class: "decide-count"}, countText, " decided");
        const meter = el("div", {class: "meter", role: "progressbar", "aria-label": "Suggestions decided", "aria-valuemin": "0", "aria-valuemax": String(list.length)},
          el("div", {class: "meter-fill"}));
        const actionsSlot = el("div", {class: "round-actions"});
        const bar = el("div", {class: "decide-bar"}, countBox, meter, actionsSlot);
        const drawBar = () => {
          const left = pending().length;
          const done = list.length - left;
          countText.textContent = `${done} of ${list.length}`;
          meter.setAttribute("aria-valuenow", String(done));
          meter.firstChild.style.width = `${(done / list.length) * 100}%`;
          const build = el("button", {
            type: "button", class: left ? "" : "primary ready", disabled: left > 0,
            onclick: async () => {
              const r = await attempt(() => api.tailored.create(rev.id));
              if (!r) return;
              tailoredId = r.value.id;
              warned.set(tailoredId, r.value.warnings || []);
              await loadRounds();
              drawBody();
            },
          }, left ? `Decide ${left} more to build` : "Build tailored resume");
          const all = left > 1 ? confirmFirst(el("button", {type: "button", class: "quiet"}, `Accept the other ${left}`), `Accept all ${left}?`, acceptAll) : null;
          fill(actionsSlot, all, build);
          return left;
        };

        const wrap = el("div", {class: "cards"});
        cards = list.map(s => ({s, node: card(rev, s, () => { drawBar(); })}));
        wrap.append(...cards.map(c => c.node));
        drawBar();
        stagger(wrap, 20, 6);
        current = Math.min(current, cards.length - 1);
        const firstPending = cards.findIndex(c => c.s.state === "pending");
        current = firstPending >= 0 ? firstPending : 0;
        markCurrent(false);
        markGaps();
        return [
          el("p", {class: "key-hint hint"}, "Keys: ", el("kbd", {text: "J"}), " ", el("kbd", {text: "K"}), " move, ",
            el("kbd", {text: "A"}), " accept, ", el("kbd", {text: "R"}), " reject, ", el("kbd", {text: "E"}), " edit"),
          wrap,
          bar,
        ];
      };

      const markCurrent = scroll => {
        cards.forEach((c, i) => c.node.classList.toggle("current", i === current));
        if (scroll) cards[current]?.node.scrollIntoView({block: "nearest", behavior: motion() ? "smooth" : "auto"});
      };

      const card = (rev, s, onDecided) => {
        const roleTitle = s.target_id && experienceTitles[s.target_id] ? experienceTitles[s.target_id] : "";
        const heading = roleTitle ? `${SECTION[s.section] || s.section} — ${roleTitle}` : (SECTION[s.section] || s.section);
        const node = el("section", {class: "tailor-card", "data-state": s.state, tabindex: "-1", "aria-label": `${heading} suggestion`});
        const paint = () => {
          const final = s.state === "edited" ? s.edited_text : s.proposed_text;
          const words = window.WordDiff.diff(s.original_text, final);
          node.dataset.state = s.state;
          const st = STATE[s.state] || STATE.pending;
          const act = (label, key, fn, on, primary) => el("button", {
            type: "button", class: primary ? "primary" : "", "aria-pressed": on === null ? null : String(on),
            title: label === "Accept" ? "Keep this change in the tailored resume" : "Keep your original text",
            onclick: async () => {
              const r = await attempt(fn);
              if (r) {
                Object.assign(s, r.value);
                paint();
                play(node, "decided", 700);
                onDecided();
                advance();
              }
            },
          }, label, el("kbd", {text: key}));
          fill(node,
            el("div", {class: "card-head"},
              el("h3", {text: heading}),
              s.engine ? el("span", {class: "engine-badge", text: ENGINE[s.engine] || s.engine}) : null,
              el("span", {class: "state-chip", "data-state": s.state}, icon(st.icon), st.label),
              el("ul", {class: "chips brings-in", "aria-label": "Skills this brings in that the job asks for"})),
            s.reason ? el("div", {class: "reason"}, el("span", {class: "reason-label", text: "Why this helps"}), el("p", {text: s.reason})) : null,
            el("div", {class: "diff-cols"},
              el("div", {class: "diff-col"}, el("h4", {text: "Current"}),
                s.original_text ? window.WordDiff.render(words, "before") : el("span", {class: "hint", text: "Nothing yet (new text)"})),
              el("div", {class: "diff-col"}, el("h4", {text: s.state === "edited" ? "Your edit" : "Suggested"}),
                window.WordDiff.render(words, "after"))),
            el("div", {class: "tailor-actions"},
              act("Accept", "A", () => api.suggestions.accept(rev.id, s.id), s.state === "accepted", s.state === "pending"),
              act("Reject", "R", () => api.suggestions.reject(rev.id, s.id), s.state === "rejected", false),
              el("button", {type: "button", title: "Write your own version", onclick: () => edit(rev, s, node, paint, onDecided)}, "Edit", el("kbd", {text: "E"})),
              s.state !== "pending" ? el("span", {class: "hint", text: "You can still change your mind."}) : null));
          const list = node.querySelector(".brings-in");
          if (list) list.replaceChildren(...bringsIn(s).map(g => el("li", {class: "chip gap", text: g})));
        };
        node.paint = paint;
        node.addEventListener("click", () => { current = cards.findIndex(c => c.node === node); markCurrent(false); });
        paint();
        return node;
      };

      //after a decision, move to the next card that still needs one
      const advance = () => {
        const next = cards.findIndex((c, i) => i > current && c.s.state === "pending");
        if (next >= 0) { current = next; markCurrent(true); }
      };

      const edit = (rev, s, node, paint, onDecided) => {
        const area = el("textarea", {rows: 6, "aria-label": `Edit ${SECTION[s.section] || s.section}`, value: s.state === "edited" ? s.edited_text : s.proposed_text});
        fill(node,
          el("h3", {text: `Edit ${SECTION[s.section] || s.section}`}),
          area,
          el("div", {class: "tailor-actions"},
            el("button", {type: "button", class: "primary", onclick: async () => {
              const r = await attempt(() => api.suggestions.edit(rev.id, s.id, area.value));
              if (r) { Object.assign(s, r.value); paint(); play(node, "decided", 460); onDecided?.(); }
            }}, "Save edit"),
            el("button", {type: "button", onclick: paint}, "Cancel")));
        area.focus();
      };

      //A J K move, A accepts, R rejects, E edits, while the review is on screen
      document.addEventListener("keydown", event => {
        if (document.querySelector(".app")?.dataset.route !== "tailor" || tailoredId || !cards.length) return;
        if (event.metaKey || event.ctrlKey || event.altKey || document.querySelector("dialog[open]")) return;
        if (/^(INPUT|TEXTAREA|SELECT)$/.test(event.target.tagName)) return;
        const key = event.key.toLowerCase();
        const card = cards[current];
        if (key === "j" || key === "arrowdown") current = Math.min(cards.length - 1, current + 1);
        else if (key === "k" || key === "arrowup") current = Math.max(0, current - 1);
        else if (key === "a" || key === "r") { card?.node.querySelectorAll(".tailor-actions button")[key === "a" ? 0 : 1]?.click(); return; }
        else if (key === "e") { card?.node.querySelectorAll(".tailor-actions button")[2]?.click(); event.preventDefault(); return; }
        else return;
        event.preventDefault();
        markCurrent(true);
      });

      // ---------- the tailored resume ----------
      const side = (h, which) => {
        if (h.status === "changed") return window.WordDiff.render(h.words, which);
        const text = window.WordDiff.plain(which === "before" ? h.before : h.after);
        const tag = h.status === "added" ? "ins" : h.status === "removed" ? "del" : null;
        return tag && text ? el(tag, {text}) : text;
      };

      //"Data Analyst, Acme" for an experience, else the section name
      const hunkTitle = h => h.heading.replace(/^#+\s*/, "") || SECTION[h.section] || h.section;

      const drawTailored = async (startTab = "changes", savedMessage = "") => {
        const [t, d] = await Promise.all([attempt(() => api.tailored.get(tailoredId)), attempt(() => api.tailored.diff(tailoredId))]);
        if (!t || !d) { tailoredId = null; return; }
        drawRounds();
        let showAll = false;
        const hasChanges = d.value.hunks.some(h => h.status !== "unchanged");
        const showAllBox = el("input", {type: "checkbox", onchange: event => { showAll = event.target.checked; drawHunks(); }});
        const diffHead = el("div", {class: "diff-cols"}, el("h4", {text: "Your resume"}), el("h4", {text: "Tailored"}));
        const hunks = el("div", {class: "tailor-stage"});
        const drawHunks = () => {
          diffHead.hidden = !hasChanges && !showAll;
          if (!hasChanges && !showAll) {
            fill(hunks, el("div", {class: "stage-empty changes-empty"},
              el("p", {text: "No changes from your base resume yet."}),
              el("div", {class: "round-actions"},
                el("button", {type: "button", class: "primary", onclick: () => pick("edit")}, "Edit resume"),
                el("button", {type: "button", onclick: () => { showAll = true; showAllBox.checked = true; drawHunks(); }}, "Show full text"))));
            return;
          }
          const nodes = [];
          let hiddenLines = 0;
          const flushHidden = () => {
            if (!hiddenLines) return;
            const n = hiddenLines;
            hiddenLines = 0;
            nodes.push(el("button", {
              type: "button",
              class: "quiet unchanged-fold",
              onclick: () => { showAll = true; showAllBox.checked = true; drawHunks(); },
            }, `${n} unchanged line${n === 1 ? "" : "s"} — show`));
          };
          for (const h of d.value.hunks) {
            if (h.status === "unchanged" && !showAll) {
              hiddenLines += h.before.split("\n").filter(Boolean).length;
              continue;
            }
            flushHidden();
            nodes.push(el("div", {class: "diff-cols"},
              el("div", {class: "diff-col"}, el("h4", {text: `${hunkTitle(h)} · ${h.status}`}), side(h, "before")),
              el("div", {class: "diff-col"}, side(h, "after"))));
          }
          flushHidden();
          fill(hunks, nodes);
        };

        const PDF_STYLES = [
          ["classic", "Classic"],
          ["modern", "Modern"],
          ["minimal", "Minimal"],
          ["compact", "Compact"],
          ["executive", "Executive"],
        ];
        const styleSelect = el("select", {id: "tailor-pdf-style", class: "pdf-style-select", "aria-label": "PDF style", title: "Choose PDF layout style"},
          PDF_STYLES.map(([v, label]) => el("option", {value: v, text: label})));
        styleSelect.value = window.Prefs?.get("pdfStyle") || "classic";
        const exportNote = el("div");
        const exportButton = el("button", {type: "button", class: "primary", title: "Export the tailored resume as a PDF file"}, "Export PDF");
        exportButton.addEventListener("click", async () => {
          exportButton.disabled = true;
          exportNote.replaceChildren();
          const r = await attempt(() => exportFile(format => api.tailored.export(t.value.id, format, styleSelect.value)));
          exportButton.disabled = false;
          if (r?.value?.saved) {
            exportNote.replaceChildren(el("div", {class: "exported"}, pico("20-pdf-exported.png"), savedNote(r.value.path)));
            play(exportNote, "enter", 500);
          }
        });
        const exportDocxButton = el("button", {type: "button", title: "Export the tailored resume as an ATS-safe DOCX file"}, "Export DOCX");
        exportDocxButton.addEventListener("click", async () => {
          exportDocxButton.disabled = true;
          exportNote.replaceChildren();
          const r = await attempt(() => api.tailored.export(t.value.id, "docx", styleSelect.value));
          exportDocxButton.disabled = false;
          if (r?.value?.saved) {
            exportNote.replaceChildren(el("div", {class: "exported"}, pico("20-pdf-exported.png"), savedNote(r.value.path)));
            play(exportNote, "enter", 500);
          }
        });

        //track the job with this tailored resume, or set it on the job's application
        const trackButton = el("button", {type: "button", title: "Add this job to the Tracker with this resume"}, "Track with this resume");
        trackButton.addEventListener("click", async () => {
          const tailoredResumeId = t.value.id;
          const list = await attempt(() => api.applications.list({archived: "include"}));
          if (!list) return;
          const existing = list.value.find(a => a.job_id === t.value.job_id);
          let done = null;
          if (!existing) {
            done = await attempt(() => api.applications.create({job_id: t.value.job_id, tailored_resume_id: tailoredResumeId}));
          } else if (existing.tailored_resume_id === tailoredResumeId) {
            done = {value: existing};
          } else {
            if (!await ask("This job is already tracked. Use this tailored resume for its application?", {confirm: "Use this resume", title: "Already tracked"})) return;
            done = await attempt(() => api.applications.update(existing.id, {tailored_resume_id: tailoredResumeId}));
          }
          if (done) say("Tracked with this resume.");
          if (done) trackButton.replaceWith(el("button", {type: "button", onclick: () => window.Tracker.open(done.value.id)}, "Open in tracker"));
        });

        //Changes or PDF, as tabs
        const panels = {};
        const tabs = [["changes", "Changes"], ["edit", "Edit resume"], ["pdf", "PDF preview"]];
        pdf = window.PdfPreview.create(async () => (await api.tailored.previewPdf(t.value.id, styleSelect.value)).data_base64, {title: "Tailored resume as a PDF"});
        styleSelect.addEventListener("change", () => {
          window.Prefs?.set("pdfStyle", styleSelect.value);
          if (!panels.pdf.hidden) pdf.load();
        });
        panels.changes = el("div", {class: "panel", id: "tailor-panel-changes", role: "tabpanel", "aria-labelledby": "tailor-tab-changes"},
          el("label", {class: "hint"}, showAllBox, " Show unchanged text"),
          diffHead,
          hunks);
        //your own edits: the whole resume as text, saved over the tailored version
        const editor = el("textarea", {id: "tailor-editor", rows: 28, spellcheck: "true", "aria-label": "Tailored resume text", value: t.value.content});
        const editNote = el("p", {class: "hint", role: "status", text: savedMessage});
        const saveEdit = el("button", {type: "button", class: "primary", disabled: true, onclick: async () => {
          saveEdit.disabled = true;
          const r = await attempt(() => api.tailored.update(t.value.id, editor.value));
          if (r) await drawTailored("changes", "Your edits are saved.");
          else saveEdit.disabled = false;
        }}, "Save edits");
        const revert = el("button", {type: "button", onclick: () => { editor.value = t.value.content; saveEdit.disabled = true; }}, "Discard changes");
        editor.addEventListener("input", () => { saveEdit.disabled = editor.value === t.value.content || !editor.value.trim(); });
        panels.edit = el("div", {class: "panel", id: "tailor-panel-edit", role: "tabpanel", "aria-labelledby": "tailor-tab-edit", hidden: true},
          el("p", {class: "hint", text: "Change anything in the tailored resume yourself. It is Markdown: # for the name, ## for sections, - for bullets. The Changes tab and the PDF follow what you save."}),
          editor,
          el("div", {class: "round-actions"}, saveEdit, revert),
          editNote);
        panels.pdf = el("div", {class: "panel", id: "tailor-panel-pdf", role: "tabpanel", "aria-labelledby": "tailor-tab-pdf", hidden: true},
          el("p", {class: "hint", text: "This is the PDF that Export PDF saves. Check it before you send it."}), pdf.node);
        const tabButtons = tabs.map(([key, label]) => el("button", {
          type: "button", role: "tab", id: `tailor-tab-${key}`, "aria-controls": `tailor-panel-${key}`,
          "aria-selected": String(key === "changes"), tabindex: key === "changes" ? "0" : "-1",
          onclick: () => pick(key),
        }, label));
        const pick = (key, animate = true) => {
          tabs.forEach(([k], i) => {
            tabButtons[i].setAttribute("aria-selected", String(k === key));
            tabButtons[i].tabIndex = k === key ? 0 : -1;
            panels[k].hidden = k !== key;
          });
          if (key === "pdf") pdf.load();
          else if (animate) play(panels[key], "enter", 220);
        };
        const tablist = el("div", {class: "tabs", role: "tablist", "aria-label": "Tailored resume view"}, tabButtons);
        tablist.addEventListener("keydown", event => {
          if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") return;
          const now = tabButtons.findIndex(b => b.getAttribute("aria-selected") === "true");
          const next = tabs[(now + (event.key === "ArrowRight" ? 1 : tabs.length - 1)) % tabs.length][0];
          pick(next);
          tabButtons[tabs.findIndex(([k]) => k === next)].focus();
        });

        fill(stage,
          el("div", {class: "round-head"},
            el("h2", {text: "Tailored resume"}),
            el("div", {class: "round-actions"},
              styleSelect,
              exportButton,
              exportDocxButton,
              trackButton,
              el("button", {type: "button", onclick: () => { tailoredId = null; drawRounds(); drawBody(); }}, t.value.revision_id ? "Back to suggestions" : "Close"),
              confirmFirst(el("button", {type: "button", class: "quiet"}, "Delete"), "Delete resume?", async () => {
                if (await attempt(() => api.tailored.delete(t.value.id))) { tailoredId = null; await loadRounds(); drawBody(); }
              }))),
          exportNote,
          (warned.get(tailoredId) || []).map(w => el("p", {class: "resume-status error", role: "alert", text: String(w)})),
          tablist,
          panels.changes,
          panels.edit,
          panels.pdf);
        drawHunks();
        if (startTab !== "changes") pick(startTab, false);
        play(stage, "enter", 220);
      };

      // ---------- entry, runs now and every time the view is shown again ----------
      //arriving on a different job: swap to it before the pickers load, so the old page is not seen first
      const sync = async () => {
        const want = window.Tailor.jobId;
        const changed = want && want !== jobId;
        if (changed) {
          jobId = want;
          revId = null;
          tailoredId = null;
          problem = null;
          jobSelect.value = String(jobId);
          generate.disabled = false;
        }
        const pickers = loadPickers();
        if (changed) await swapJob();
        await pickers;
        loadEngine();
      };
      loadPickers().then(() => swapJob());
      loadEngine();
      return sync;
    },
  };
})();

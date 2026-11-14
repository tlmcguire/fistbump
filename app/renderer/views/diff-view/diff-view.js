//the tailor view. pick a saved job, generate suggestions, review each one, build the tailored
//resume and see it next to its base
(() => {
    window.Tailor = {
      jobId: null,
  
      //jobs view calls this from "Tailor resume".
      open(id) {
        this.jobId = id;
        window.App.go("tailor");
      },
  
      mount(container) {
        const {el, fill} = window.ResumeBuilder;
        const api = window.FistBumpAPI;
        const SECTION = {summary: "Summary", experience: "Experience", skills: "Skills"};
        const STATUS = {queued: "Starting", running: "Writing", done: "Ready", failed: "Failed", canceled: "Canceled"};
        const ENGINE = {local: "Local AI", remote: "Remote AI", rules: "Rules"};
  
        let jobId = null;
        let revId = null;       //the round being reviewed
        let tailoredId = null;  //set while a tailored resume is open
        let warnings = [];
        let rounds = [];        //newest first, each with its number n
        let tailored = [];
        let poll = null;
  
        //shell
        const status = el("p", {class: "resume-status", role: "status"});
        const say = (message, isError = false) => {
          status.textContent = message;
          status.classList.toggle("error", isError);
        };
        //runs fn. a failure goes to the status line and gives null, success gives {value}
        const attempt = async fn => {
          try {
            return {value: await fn()};
          } catch (error) {
            say(error.message, true);
            return null;
          }
        };
  
        const jobSelect = el("select", {"aria-label": "Job"});
        const baseSelect = el("select", {"aria-label": "Start from"});
        const generate = el("button", {type: "button", class: "primary", onclick: () => startRound()}, "Generate suggestions");
        const roundsBar = el("div", {class: "tailor-rounds"});
        const stage = el("div", {class: "tailor-stage"});
        fill(container, el("div", {class: "pane tailor"},
          el("h1", {class: "view-title", text: "Tailor"}),
          status,
          el("div", {class: "tailor-picker"}, jobSelect, baseSelect, generate),
          roundsBar,
          stage));
  
        jobSelect.addEventListener("change", async () => {
          jobId = jobSelect.value ? Number(jobSelect.value) : null;
          window.Tailor.jobId = jobId;
          revId = null;
          tailoredId = null;
          await loadRounds();
          drawBody();
        });
  
        //data
        const loadPickers = async () => {
          const [jobs, versions] = await Promise.all([attempt(() => api.jobs.list("")), attempt(() => api.resumes.list())]);
          fill(jobSelect,
            el("option", {value: ""}, "Choose a saved job"),
            (jobs?.value || []).map(j => el("option", {value: j.id}, `${j.position_title} at ${j.company_name}`)));
          fill(baseSelect,
            el("option", {value: ""}, "Start from: master resume"),
            (versions?.value || []).map(v => el("option", {value: v.id}, `Start from: ${v.version_label}`)));
          jobSelect.value = jobId ? String(jobId) : "";
        };
  
        const loadRounds = async () => {
          generate.disabled = !jobId;
          if (!jobId) {
            rounds = [];
            tailored = [];
          } else {
            const [revs, tl] = await Promise.all([attempt(() => api.revisions.list(jobId)), attempt(() => api.tailored.list(jobId))]);
            rounds = (revs?.value || []).map((r, i, all) => ({...r, n: all.length - i})); //the API lists newest first
            tailored = tl?.value || [];
          }
          if (!rounds.some(r => r.id === revId)) revId = rounds[0]?.id ?? null;
          drawRounds();
        };
  
        const startRound = async () => {
          const request = {job_id: jobId, engine: "auto"};
          if (baseSelect.value) request.base_resume_id = Number(baseSelect.value);
          const created = await attempt(() => api.revisions.create(request));
          if (!created) return;
          say("");
          revId = created.value.id;
          tailoredId = null;
          await loadRounds();
          drawBody();
        };
  
        //polling, once a second until the round is done, failed or canceled
        const stopPolling = () => {
          clearInterval(poll);
          poll = null;
        };
        const watch = id => {
          stopPolling();
          poll = setInterval(async () => {
            const r = await attempt(() => api.revisions.get(id));
            if (r && ["queued", "running"].includes(r.value.status)) return;
            stopPolling();
            if (revId === id && !tailoredId) {
              await loadRounds();
              drawBody();
            }
          }, 1000);
        };
  
        //rounds list
        const drawRounds = () => fill(roundsBar, rounds.map(r => el("button", {
          type: "button",
          class: "tailor-round",
          "aria-pressed": String(r.id === revId && !tailoredId),
          onclick: () => { revId = r.id; tailoredId = null; drawRounds(); drawBody(); },
        }, `Round ${r.n} \u00b7 ${STATUS[r.status] || r.status} \u00b7 ${ENGINE[r.engine] || r.engine}`)));
  
        //review
        const drawBody = async () => {
          stopPolling();
          if (tailoredId) return drawTailored();
          if (!jobId || revId === null) {
            fill(stage, el("p", {class: "hint", text: jobId ? "No suggestions for this job yet. Press Generate suggestions." : "Choose a saved job to tailor your resume for."}));
            return;
          }
          const r = await attempt(() => api.revisions.get(revId));
          if (r && r.value.id === revId) show(r.value);
        };
  
        const show = rev => {
          const round = rounds.find(x => x.id === rev.id);
          const busy = rev.status === "queued" || rev.status === "running";
          const draft = tailored.find(t => t.revision_id === rev.id);
          if (busy) watch(rev.id);
          fill(stage,
            el("div", {class: "tailor-head"},
              el("strong", {text: `Round ${round?.n ?? ""}`}),
              `${STATUS[rev.status] || rev.status} \u00b7 ${ENGINE[rev.engine] || rev.engine}`,
              draft && el("button", {type: "button", class: "primary", onclick: () => { tailoredId = draft.id; drawRounds(); drawBody(); }}, "View tailored resume"),
              busy && el("button", {type: "button", onclick: async () => { await attempt(() => api.revisions.cancel(rev.id)); await loadRounds(); drawBody(); }}, "Cancel"),
              el("button", {type: "button", class: "danger", onclick: async () => {
                if (!window.confirm(`Delete round ${round?.n ?? ""} and its suggestions?`)) return;
                if (await attempt(() => api.revisions.delete(rev.id))) { revId = null; await loadRounds(); drawBody(); }
              }}, "Delete")),
            rev.error && el("p", {class: "resume-status error", text: `Could not write suggestions: ${rev.error}`}),
            busy && el("p", {class: "hint", text: "Writing suggestions\u2026"}),
            rev.status === "done" && suggestions(rev));
        };
  
        const suggestions = rev => {
          const list = rev.suggestions;
          if (!list.length) return el("p", {class: "hint", text: "Your resume already fits this job as well as rules can tell."});
          const pending = list.filter(s => s.state === "pending");
  
          const acceptAll = async () => {
            for (const s of pending) {
              const r = await attempt(() => api.suggestions.accept(rev.id, s.id));
              if (!r) break;
              Object.assign(s, r.value);
            }
            show(rev);
          };
          const build = el("button", {
            type: "button", class: "primary", disabled: pending.length > 0,
            onclick: async () => {
              const r = await attempt(() => api.tailored.create(rev.id));
              if (!r) return;
              tailoredId = r.value.id;
              warnings = r.value.warnings || [];
              await loadRounds();
              drawBody();
            },
          }, pending.length ? `Decide ${pending.length} more to build` : "Build tailored resume");
  
          return [
            list.map(s => card(rev, s)),
            el("div", {class: "tailor-head"},
              `${list.length - pending.length} of ${list.length} decided`,
              pending.length > 0 && el("button", {type: "button", onclick: acceptAll}, "Accept all remaining"),
              build),
          ];
        };
  
        const card = (rev, s) => {
          const final = s.state === "edited" ? s.edited_text : s.proposed_text;
          const words = window.WordDiff.diff(s.original_text, final);
          const act = (label, fn, on = false) => el("button", {
            type: "button", class: on ? "primary" : "",
            onclick: async () => {
              const r = await attempt(fn);
              if (r) { Object.assign(s, r.value); show(rev); }
            },
          }, label);
          const node = el("section", {class: "tailor-card"},
            el("div", {class: "tailor-head"}, el("strong", {text: SECTION[s.section] || s.section}), s.state === "pending" ? "to review" : s.state),
            el("div", {class: "diff-cols"},
              el("div", {class: "diff-col"}, el("h4", {text: "Current"}),
                s.original_text ? window.WordDiff.render(words, "before") : el("span", {class: "hint", text: "Nothing yet (new text)"})),
              el("div", {class: "diff-col"}, el("h4", {text: s.state === "edited" ? "Your edit" : "Suggested"}),
                window.WordDiff.render(words, "after"))),
            el("div", {class: "tailor-actions"},
              act(s.state === "accepted" ? "Accepted" : "Accept", () => api.suggestions.accept(rev.id, s.id), s.state === "accepted"),
              act(s.state === "rejected" ? "Rejected" : "Reject", () => api.suggestions.reject(rev.id, s.id)),
              el("button", {type: "button", onclick: () => edit(rev, s, node)}, "Edit")));
          return node;
        };
  
        const edit = (rev, s, node) => {
          const area = el("textarea", {rows: 6, value: s.state === "edited" ? s.edited_text : s.proposed_text});
          fill(node,
            el("strong", {text: `Edit ${SECTION[s.section] || s.section}`}),
            area,
            el("div", {class: "tailor-actions"},
              el("button", {type: "button", class: "primary", onclick: async () => {
                const r = await attempt(() => api.suggestions.edit(rev.id, s.id, area.value));
                if (r) { Object.assign(s, r.value); show(rev); }
              }}, "Save edit"),
              el("button", {type: "button", onclick: () => show(rev)}, "Cancel")));
          area.focus();
        };
  
        //tailored resume next to its base
        const side = (h, which) => {
          if (h.status === "changed") return window.WordDiff.render(h.words, which);
          const text = which === "before" ? h.before : h.after;
          const tag = h.status === "added" ? "ins" : h.status === "removed" ? "del" : null;
          return tag && text ? el(tag, {text}) : text;
        };
  
        const drawTailored = async () => {
          const [t, d] = await Promise.all([attempt(() => api.tailored.get(tailoredId)), attempt(() => api.tailored.diff(tailoredId))]);
          if (!t || !d) { tailoredId = null; return; }
          let showAll = false;
          const hunks = el("div", {class: "tailor-stage"});
          const drawHunks = () => fill(hunks, d.value.hunks.map(h => {
            if (h.status === "unchanged" && !showAll) {
              const lines = h.before.split("\n").filter(Boolean).length;
              return lines ? el("p", {class: "hint", text: `${lines} unchanged line${lines === 1 ? "" : "s"}`}) : null;
            }
            return el("div", {class: "diff-cols"},
              el("div", {class: "diff-col"}, el("h4", {text: `${SECTION[h.section] || h.section} \u00b7 ${h.status}`}), side(h, "before")),
              el("div", {class: "diff-col"}, side(h, "after")));
          }));
          fill(stage,
            el("div", {class: "tailor-head"},
              el("strong", {text: "Tailored resume"}),
              el("button", {type: "button", onclick: () => { tailoredId = null; warnings = []; drawRounds(); drawBody(); }}, "Back to suggestions"),
              el("button", {type: "button", class: "danger", onclick: async () => {
                if (!window.confirm("Delete this tailored resume?")) return;
                if (await attempt(() => api.tailored.delete(t.value.id))) { tailoredId = null; await loadRounds(); drawBody(); }
              }}, "Delete")),
            warnings.map(w => el("p", {class: "resume-status error", text: String(w)})),
            el("label", {class: "hint"},
              el("input", {type: "checkbox", onchange: event => { showAll = event.target.checked; drawHunks(); }}),
              " Show unchanged text"),
            el("div", {class: "diff-cols"}, el("h4", {text: "Your resume"}), el("h4", {text: "Tailored"})),
            hunks);
          drawHunks();
        };
  
        //entry, runs now and every time the view is shown again
        const sync = async () => {
          await loadPickers();
          const want = window.Tailor.jobId;
          if (want && want !== jobId) {
            jobId = want;
            revId = null;
            tailoredId = null;
            jobSelect.value = String(jobId);
            await loadRounds();
            drawBody();
          }
        };
        loadRounds().then(drawBody);
        sync();
        return sync;
      },
    };
  })();
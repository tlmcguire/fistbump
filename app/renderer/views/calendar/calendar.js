// The calendar: interviews, calls and follow-ups by week or by month, so plans never land on top of each other.
//  - Plans are kept by the backend (/v1/events). A plan can belong to a saved job.
//  - Follow-up dates from the tracker show as all-day notes on their day.
//  - Busy times from the person's own calendars (Settings > Calendar, off until they add one) show as hatched
//    blocks, and a plan that runs into one, or into another plan, says so before and after it is saved.
//  - Any plan can be added to Apple Calendar or Outlook (an .ics file), Google Calendar or Outlook.com, and every
//    plan can be exported at once.
// Times are the computer's local time; the backend keeps them with their offset.
window.CalendarView = (() => {
  const api = () => window.FistBumpAPI;
  const HOURS = [7, 22]; //the part of the day the week shows; earlier and later plans are pinned to its edges
  const KINDS = [["interview", "Interview"], ["call", "Call"], ["follow_up", "Follow-up"], ["other", "Other"]];
  const MAX_PLAN_MS = 7 * 24 * 60 * 60 * 1000;
  const BUSY_TTL = 15 * 60 * 1000;
  let busyCache = {at: 0, calendars: [], failed: 0, feeds: 0};
  let pending = null; //a plan to open once the view is ready (from another view)

  const pad = n => String(n).padStart(2, "0");
  const isoDay = d => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  const dayStart = d => new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const addDays = (d, n) => { const x = new Date(d); x.setDate(x.getDate() + n); return x; };
  const sameDay = (a, b) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
  const spansDay = (plan, day, nextDay) => new Date(plan.starts_at) < nextDay && new Date(plan.ends_at) > day;
  //an RFC 3339 time with this computer's offset, as the backend keeps it
  const localIso = d => {
    const off = -d.getTimezoneOffset();
    const sign = off >= 0 ? "+" : "-";
    return `${isoDay(d)}T${pad(d.getHours())}:${pad(d.getMinutes())}:00${sign}${pad(Math.floor(Math.abs(off) / 60))}:${pad(Math.abs(off) % 60)}`;
  };
  const fmt = (d, opts) => new Intl.DateTimeFormat(window.I18n?.locale?.() || undefined, opts).format(d);
  const time = d => fmt(d, {hour: "numeric", minute: "2-digit"});
  const weekStartOf = d => {
    const first = window.Prefs.get("weekStart") === "sunday" ? 0 : 1;
    const x = dayStart(d);
    return addDays(x, -((x.getDay() - first + 7) % 7));
  };
  const KIND_LABEL = Object.fromEntries(KINDS);

  // ---------- busy times from the person's calendars ----------
  async function loadBusy(force = false) {
    if (!force && Date.now() - busyCache.at < BUSY_TTL) return busyCache;
    try {
      const {count} = await api().calendar.feeds();
      busyCache.feeds = count || 0;
      if (!count) { busyCache = {at: Date.now(), calendars: [], failed: 0, feeds: 0}; return busyCache; }
      const res = await api().calendar.busy();
      busyCache = {at: Date.now(), calendars: res?.calendars || [], failed: res?.failed || 0, feeds: count};
    } catch {
      busyCache.at = Date.now();
    }
    return busyCache;
  }
  const busyIn = (from, to) => busyCache.calendars.flatMap(text => window.ICS.busy(text, from.getTime(), to.getTime()));

  // ---------- adding a plan to the person's own calendar ----------
  async function addToCalendar(plan, target = window.Prefs.get("calendarTarget")) {
    if (target === "google") return window.open(window.ICS.googleLink(plan), "_blank");
    if (target === "outlook") return window.open(window.ICS.outlookLink(plan), "_blank");
    const name = window.ICS.summaryOf(plan).slice(0, 50);
    const text = window.ICS.file([plan]);
    try {
      await api().calendar.openIcs(name, text);
    } catch {
      //no shell (a browser preview): the file downloads instead
      const a = Object.assign(document.createElement("a"), {href: URL.createObjectURL(new Blob([text], {type: "text/calendar"})), download: `${name}.ics`});
      a.click();
      setTimeout(() => URL.revokeObjectURL(a.href), 1000);
    }
  }
  const addButtons = (plan, el) => {
    const target = window.Prefs.get("calendarTarget");
    const named = {ics: "Add to Apple Calendar or Outlook", google: "Add to Google Calendar", outlook: "Add to Outlook.com"};
    if (target !== "ask") return [el("button", {type: "button", onclick: () => addToCalendar(plan, target)}, named[target])];
    return [
      el("button", {type: "button", title: "Opens the plan in the calendar app on this computer (Apple Calendar, Outlook)", onclick: () => addToCalendar(plan, "ics")}, "Calendar app"),
      el("button", {type: "button", onclick: () => addToCalendar(plan, "google")}, "Google"),
      el("button", {type: "button", onclick: () => addToCalendar(plan, "outlook")}, "Outlook.com"),
    ];
  };

  // ---------- the plan form ----------
  //prefill: {date, hour, minute, job_id, kind} for a new plan, or a saved plan to change
  async function openPlan(prefill = {}, {plans = [], onSaved} = {}) {
    const {el} = window.ResumeBuilder;
    document.querySelector("dialog.plan-dialog")?.remove();
    const saved = prefill.id ? prefill : null;
    const start = saved ? new Date(saved.starts_at) : (() => {
      const d = prefill.date ? new Date(`${prefill.date}T00:00:00`) : new Date();
      if (prefill.hour !== undefined) d.setHours(prefill.hour, prefill.minute || 0, 0, 0);
      else { d.setHours(d.getHours() + 1, 0, 0, 0); }
      return d;
    })();
    const end = saved ? new Date(saved.ends_at) : new Date(start.getTime() + 60 * 60 * 1000);
    const originalDuration = end - start;
    let kind = saved?.kind || prefill.kind || "interview";

    const jobs = await api().jobs.list("").catch(() => []);
    const jobSelect = el("select", {id: "plan-job"}, el("option", {value: "", text: "Not about a job"}),
      jobs.map(j => el("option", {value: String(j.id), text: `${j.position_title} at ${j.company_name}`})));
    jobSelect.value = String(saved?.job_id || prefill.job_id || "");
    const title = el("input", {id: "plan-title", type: "text", maxlength: "200", placeholder: "Left empty, it is named after the job", value: saved?.title || ""});
    title.value = saved?.title || "";
    const clockValue = d => `${pad(d.getHours())}:${pad(d.getMinutes())}`;
    const date = el("input", {id: "plan-date", type: "date", value: isoDay(start)});
    const at = el("input", {id: "plan-time", type: "time", value: clockValue(start)});
    const endDate = el("input", {id: "plan-end-date", type: "date", value: isoDay(end)});
    const endAt = el("input", {id: "plan-end-time", type: "time", value: clockValue(end)});
    const enteredTime = (day, clock) => new Date(`${day.value}T${clock.value}`);
    let endEdited = false;
    const where = el("input", {id: "plan-where", type: "text", maxlength: "300", placeholder: "An address, or the video link", value: saved?.location || ""});
    where.value = saved?.location || "";
    const notes = el("textarea", {id: "plan-notes", rows: "3", maxlength: "4000", placeholder: "Who you are meeting, what to bring"});
    notes.value = saved?.notes || "";
    const clash = el("div", {class: "plan-clash", role: "status", "aria-live": "polite"});
    const status = el("p", {class: "report-status", role: "status"});

    const kindButtons = KINDS.map(([value, label]) => el("button", {type: "button", role: "radio", "data-kind": value, "aria-checked": String(value === kind),
      onclick: () => { kind = value; kindButtons.forEach(b => b.setAttribute("aria-checked", String(b.dataset.kind === kind))); }}, label));

    const read = (s, e) => {
      const job = jobs.find(j => String(j.id) === jobSelect.value);
      return {
        id: saved?.id, kind, job_id: job ? job.id : null,
        title: title.value.trim() || [KIND_LABEL[kind], job?.company_name].filter(Boolean).join(" with ").replace("Follow-up with", "Follow up with"),
        starts_at: localIso(s), ends_at: localIso(e), location: where.value.trim(), notes: notes.value.trim(),
        company_name: job?.company_name, position_title: job?.position_title,
      };
    };
    //what the plan would run into, said as it is being chosen
    const check = () => {
      const s = enteredTime(date, at);
      const e = enteredTime(endDate, endAt);
      const elapsed = e - s;
      const hits = elapsed > 0 && elapsed <= MAX_PLAN_MS
        ? window.ICS.clashes(read(s, e), plans, busyIn(dayStart(s), addDays(dayStart(e), 1))) : [];
      clash.replaceChildren(...hits.slice(0, 3).map(h => el("p", {class: "plan-clash-line"},
        el("b", {text: "Overlaps "}),
        h.kind === "plan"
          ? `${window.ICS.summaryOf(h.plan)}, ${time(new Date(h.plan.starts_at))} to ${time(new Date(h.plan.ends_at))}`
          : `something on your calendar, ${time(new Date(h.busy.start))} to ${time(new Date(h.busy.end))}`)));
      clash.classList.toggle("has", hits.length > 0);
    };
    for (const input of [date, at]) input.addEventListener("input", () => {
      const s = enteredTime(date, at);
      if (!endEdited && Number.isFinite(s.getTime())) {
        const nextEnd = new Date(s.getTime() + originalDuration);
        endDate.value = isoDay(nextEnd);
        endAt.value = clockValue(nextEnd);
      }
      status.textContent = "";
      check();
    });
    for (const input of [endDate, endAt]) input.addEventListener("input", () => {
      endEdited = true;
      status.textContent = "";
      check();
    });

    const dialog = el("dialog", {class: "ask plan-dialog", "aria-labelledby": "plan-heading"});
    const close = () => { dialog.close(); dialog.remove(); };
    const save = el("button", {type: "button", class: "primary"}, saved ? "Save changes" : "Add to your calendar");
    save.addEventListener("click", async () => {
      const s = enteredTime(date, at);
      const e = enteredTime(endDate, endAt);
      const elapsed = e - s;
      if (!Number.isFinite(elapsed)) { status.textContent = "Choose a day and time for both the start and end."; return; }
      if (elapsed <= 0) { status.textContent = "The end must be after the start."; return; }
      if (elapsed > MAX_PLAN_MS) { status.textContent = "A plan can last a week at most."; return; }
      const p = read(s, e);
      save.disabled = true;
      try {
        const body = {kind: p.kind, job_id: p.job_id, title: p.title, starts_at: p.starts_at, ends_at: p.ends_at, location: p.location, notes: p.notes};
        const result = saved ? await api().events.update(saved.id, body) : await api().events.create(body);
        close();
        onSaved?.(result);
        window.dispatchEvent(new CustomEvent("fistbump:events"));
        if (!saved && window.Prefs.get("calendarAuto") && window.Prefs.get("calendarTarget") !== "ask") addToCalendar(result);
      } catch (err) {
        save.disabled = false;
        status.textContent = err.message;
      }
    });
    const remove = saved ? el("button", {type: "button", class: "quiet", onclick: async () => {
      if (!await window.UI.ask("Delete this plan? It is taken off the Fistbump calendar only.", {confirm: "Delete", danger: true, title: "Delete plan"})) return;
      try { await api().events.delete(saved.id); close(); onSaved?.(null); window.dispatchEvent(new CustomEvent("fistbump:events")); } catch (err) { status.textContent = err.message; }
    }}, "Delete") : null;

    const field = (id, label, input, cls = "") => el("label", {class: `plan-field ${cls}`.trim(), for: id}, el("span", {text: label}), input);
    const timeFields = (label, day, clock) => el("fieldset", {class: "plan-times"},
      el("legend", {text: label}), field(day.id, "Day", day), field(clock.id, "Time", clock));
    dialog.append(...[
      el("h2", {id: "plan-heading", text: saved ? "Change this plan" : "Plan something"}),
      el("div", {class: "st3-seg plan-kinds", role: "radiogroup", "aria-label": "Kind of plan"}, kindButtons),
      el("div", {class: "plan-grid"},
        field("plan-job", "Job", jobSelect, "wide"),
        timeFields("Starts", date, at),
        timeFields("Ends", endDate, endAt),
        field("plan-title", "Title", title, "wide"),
        field("plan-where", "Where", where, "wide"),
        field("plan-notes", "Notes", notes, "wide")),
      clash,
      status,
      saved ? el("div", {class: "plan-add"}, el("span", {text: "Add to"}), addButtons(saved, el)) : null,
      el("div", {class: "ask-actions"}, remove, el("span", {class: "plan-gap"}), el("button", {type: "button", onclick: close}, "Cancel"), save),
    ].filter(Boolean));
    window.UI.wireDialogClose(dialog, close);
    dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
    document.body.append(dialog);
    dialog.showModal();
    loadBusy().then(check);
    (saved ? title : jobSelect).focus();
  }

  // ---------- the view ----------
  function mount(container) {
    const {el, fill} = window.ResumeBuilder;
    const {icon} = window.UI;
    container.classList.add("v3", "cal3");
    let anchor = new Date();
    let plans = [];
    let followUps = [];
    let loadSeq = 0;

    const ink = name => {
      const ns = "http://www.w3.org/2000/svg";
      const svg = document.createElementNS(ns, "svg");
      svg.setAttribute("class", `ink ink-${name}`);
      svg.setAttribute("aria-hidden", "true");
      svg.setAttribute("preserveAspectRatio", "none");
      const use = document.createElementNS(ns, "use");
      use.setAttribute("href", `#ink-${name}`);
      svg.append(use);
      return svg;
    };

    const mode = () => window.Prefs.get("calendarView");
    const range = () => {
      if (mode() === "month") {
        const first = new Date(anchor.getFullYear(), anchor.getMonth(), 1);
        const from = weekStartOf(first);
        return {from, to: addDays(from, 42)};
      }
      const from = weekStartOf(anchor);
      return {from, to: addDays(from, 7)};
    };

    const summary = el("p", {class: "jl3-sum", role: "status"});
    const rangeLabel = el("h2", {class: "cal3-range", "aria-live": "polite"});
    const sync = el("div", {class: "cal3-sync"});
    const sheet = el("section", {class: "paper cal3-sheet", "aria-label": "Calendar"});
    const step = n => {
      anchor = mode() === "month" ? new Date(anchor.getFullYear(), anchor.getMonth() + n, 1) : addDays(anchor, 7 * n);
      load();
    };
    const viewSeg = el("div", {class: "jl3-view", role: "radiogroup", "aria-label": "Show"}, [["week", "Week"], ["month", "Month"]].map(([v, label]) =>
      el("button", {type: "button", role: "radio", "data-view": v, "aria-checked": String(mode() === v), onclick: () => { window.Prefs.set("calendarView", v); }}, label)));
    window.Prefs.onChange(key => {
      if (key === "calendarView") { viewSeg.querySelectorAll("button").forEach(b => b.setAttribute("aria-checked", String(b.dataset.view === mode()))); if (!container.hidden) load(); }
      if (key === "weekStart" && !container.hidden) load();
    });

    const exportAll = el("button", {type: "button", class: "tr3-tool", title: "Save every plan from today on as one calendar file, to import anywhere", onclick: async () => {
      const list = await api().events.list({from: localIso(dayStart(new Date()))}).catch(() => []);
      const text = window.ICS.file(list);
      const a = Object.assign(document.createElement("a"), {href: URL.createObjectURL(new Blob([text], {type: "text/calendar"})), download: "Fistbump plans.ics"});
      a.click();
      setTimeout(() => URL.revokeObjectURL(a.href), 1000);
    }}, "Export .ics");

    fill(container, el("div", {class: "jl3-scroll"},
      el("div", {class: "jl3-grid"},
        el("header", {class: "jl3-head drag", "data-tauri-drag-region": ""},
          el("h1", {class: "display-xl"}, "Your ", el("span", {class: "underlined"}, "calendar", ink("underline"))),
          summary),
        el("div", {class: "jl3-tools"},
          el("div", {class: "cal3-step"},
            el("button", {type: "button", class: "tr3-icon cal3-prev", title: "Earlier", "aria-label": "Earlier", onclick: () => step(-1)}, icon("chevron")),
            el("button", {type: "button", class: "tr3-mini", onclick: () => { anchor = new Date(); load(); }}, "Today"),
            el("button", {type: "button", class: "tr3-icon cal3-next", title: "Later", "aria-label": "Later", onclick: () => step(1)}, icon("chevron"))),
          rangeLabel,
          viewSeg,
          sync,
          exportAll,
          el("button", {type: "button", class: "btn btn-mark cal3-new", onclick: () => openPlan({}, {plans, onSaved: load})}, "Plan an interview")),
        sheet)));

    const planChip = (p, extra = {}) => {
      const s = new Date(p.starts_at);
      const variant = String(extra.class || "");
      const hits = window.ICS.clashes(p, plans, busyIn(range().from, range().to));
      return el("button", {type: "button", class: `cal3-plan kind-${p.kind}${hits.length ? " clash" : ""}`, ...extra,
        title: hits.length ? "Overlaps something else" : null,
        "aria-label": `${window.ICS.summaryOf(p)}, ${fmt(s, {weekday: "long"})} ${time(s)} to ${time(new Date(p.ends_at))}${hits.length ? ", overlaps something else" : ""}`,
        onclick: () => openPlan(p, {plans, onSaved: load})},
        el("b", {translate: "no", text: window.ICS.summaryOf(p)}),
        el("span", {class: "num", text: variant.includes("continues") ? "Continues" : variant.includes("mini") ? time(s) : `${time(s)} – ${time(new Date(p.ends_at))}`}),
        hits.length ? el("span", {class: "cal3-clash-tag", text: "Overlaps"}) : null);
    };
    const followChip = a => el("button", {type: "button", class: "cal3-follow", onclick: () => window.JobPage.open(a.job_id)},
      el("span", {translate: "no", text: `${a.status === "Interviewing" ? "Interview step" : "Follow up"}: ${a.company_name || a.position_title || "a job"}`}));

    function drawWeek() {
      const {from} = range();
      const days = [...Array(7)].map((_, i) => addDays(from, i));
      const today = new Date();
      const rowH = 52;
      const busy = busyIn(from, addDays(from, 7));
      const head = el("div", {class: "cal3-week-head"}, el("span"), days.map(d => el("div", {class: sameDay(d, today) ? "cal3-day today" : "cal3-day"},
        el("span", {class: "cal3-dow", text: fmt(d, {weekday: "short"})}),
        sameDay(d, today) ? el("span", {class: "circled num"}, String(d.getDate()), ink("circle")) : el("span", {class: "num", text: String(d.getDate())}))));
      const allDay = el("div", {class: "cal3-allday"}, el("span", {class: "cal3-gutter-label", text: "All day"}), days.map(d => el("div", {class: "cal3-allday-cell"},
        followUps.filter(a => a.next_step_date === isoDay(d)).map(followChip),
        busy.filter(b => b.allDay && b.start < addDays(d, 1).getTime() && b.end > d.getTime()).slice(0, 1).map(() => el("span", {class: "cal3-busy-day", text: "Busy"})))));
      const gutter = el("div", {class: "cal3-gutter", "aria-hidden": "true"},
        [...Array(HOURS[1] - HOURS[0])].map((_, i) => el("span", {text: time(new Date(2000, 0, 1, HOURS[0] + i))})));
      const cols = days.map(d => {
        const nextDay = addDays(d, 1);
        const col = el("div", {class: sameDay(d, today) ? "cal3-col today" : "cal3-col"});
        col.style.setProperty("--rows", String(HOURS[1] - HOURS[0]));
        for (let h = HOURS[0]; h < HOURS[1]; h++) {
          col.append(el("button", {type: "button", class: "cal3-slot", "aria-label": `Plan something ${fmt(d, {weekday: "long", month: "long", day: "numeric"})} at ${time(new Date(2000, 0, 1, h))}`,
            onclick: () => openPlan({date: isoDay(d), hour: h}, {plans, onSaved: load})}));
        }
        const place = (s, e) => {
          const totalHeight = (HOURS[1] - HOURS[0]) * rowH;
          const position = time => ((time.getHours() - HOURS[0]) * 60 + time.getMinutes()) / 60 * rowH;
          const top = s < d ? 0 : Math.min(totalHeight - 22, Math.max(0, position(s)));
          const bottom = e >= nextDay ? totalHeight : Math.min(totalHeight, position(e));
          return {top, height: Math.max(22, bottom - top - 3)};
        };
        for (const b of busy.filter(b => !b.allDay && b.start < nextDay.getTime() && b.end > d.getTime())) {
          const s = new Date(b.start);
          const e = new Date(b.end);
          const {top, height} = place(s, e);
          const block = el("div", {class: "cal3-busy", "aria-hidden": "true"}, el("span", {text: "Busy"}));
          block.style.setProperty("top", `${top}px`);
          block.style.setProperty("height", `${height}px`);
          col.append(block);
        }
        for (const p of plans.filter(p => spansDay(p, d, nextDay))) {
          const {top, height} = place(new Date(p.starts_at), new Date(p.ends_at));
          const chip = planChip(p);
          chip.style.setProperty("top", `${top}px`);
          chip.style.setProperty("height", `${height}px`);
          if (height < 44) chip.classList.add("short");
          col.append(chip);
        }
        if (sameDay(d, today)) {
          const mins = (today.getHours() - HOURS[0]) * 60 + today.getMinutes();
          if (mins > 0 && mins < (HOURS[1] - HOURS[0]) * 60) {
            const now = el("div", {class: "cal3-now", "aria-hidden": "true"});
            now.style.setProperty("top", `${mins / 60 * rowH}px`);
            col.append(now);
          }
        }
        return col;
      });
      const body = el("div", {class: "cal3-week-body"}, gutter, cols);
      body.style.setProperty("--row", `${rowH}px`);
      const header = el("div", {class: "cal3-week-header"}, head, allDay);
      const box = el("div", {class: "cal3-week cal3-scrollbox"}, header, body);
      fill(sheet, box);
      box.style.scrollPaddingBlockStart = `${header.offsetHeight}px`;
      //open at the first plan of the week, or at 8 am, so the morning is never scrolled past
      const first = Math.min(8, ...plans.filter(p => new Date(p.starts_at) >= from).map(p => new Date(p.starts_at).getHours()));
      box.scrollTop = Math.max(0, (first - HOURS[0]) * rowH - 8);
    }

    function drawMonth() {
      const {from} = range();
      const today = new Date();
      const month = anchor.getMonth();
      const busy = busyIn(from, addDays(from, 42));
      const head = el("div", {class: "cal3-month-head"}, [...Array(7)].map((_, i) => el("span", {text: fmt(addDays(from, i), {weekday: "short"})})));
      const cells = [...Array(42)].map((_, i) => {
        const d = addDays(from, i);
        const nextDay = addDays(d, 1);
        const dayPlans = plans.filter(p => spansDay(p, d, nextDay));
        const follows = followUps.filter(a => a.next_step_date === isoDay(d));
        const busyHours = busy.filter(b => !b.allDay && b.start < addDays(d, 1).getTime() && b.end > d.getTime()).length;
        return el("div", {class: ["cal3-mday", d.getMonth() !== month ? "out" : "", sameDay(d, today) ? "today" : ""].join(" ").trim()},
          el("button", {type: "button", class: "cal3-mday-n num", "aria-label": `${fmt(d, {weekday: "long", month: "long", day: "numeric"})}: open the week`,
            onclick: () => { anchor = d; window.Prefs.set("calendarView", "week"); }},
            sameDay(d, today) ? el("span", {class: "circled"}, String(d.getDate()), ink("circle")) : String(d.getDate())),
          el("div", {class: "cal3-mday-list"},
            dayPlans.slice(0, 3).map(p => planChip(p, {class: `cal3-plan kind-${p.kind} mini${sameDay(new Date(p.starts_at), d) ? "" : " continues"}`})),
            follows.slice(0, 2).map(followChip),
            dayPlans.length > 3 ? el("span", {class: "cal3-more", text: `${dayPlans.length - 3} more`}) : null,
            busyHours ? el("span", {class: "cal3-busy-note", text: busyHours === 1 ? "1 busy time" : `${busyHours} busy times`}) : null));
      });
      fill(sheet, el("div", {class: "cal3-month"}, head, el("div", {class: "cal3-month-grid"}, cells)));
    }

    function drawSync() {
      const n = busyCache.feeds;
      const refresh = el("button", {type: "button", class: "tr3-tool", onclick: async () => { refresh.disabled = true; refresh.textContent = "Checking…"; await loadBusy(true); draw(); }}, "Refresh");
      fill(sync, n
        ? [el("span", {class: "cal3-sync-dot", "aria-hidden": "true"}), el("span", {text: n === 1 ? "Checking 1 calendar for clashes" : `Checking ${n} calendars for clashes`}),
          busyCache.failed ? el("span", {class: "cal3-sync-warn", text: " · one did not answer"}) : null, refresh]
        : el("button", {type: "button", class: "tr3-tool", title: "Show your own calendar's busy times here, so plans never overlap", onclick: () => {
          window.App.go("jobs");
          window.App.openTab("tab-settings");
          setTimeout(() => document.getElementById("set-calendar")?.scrollIntoView({block: "start", behavior: "smooth"}), 120);
        }}, "Check against your calendar"));
    }

    function draw() {
      const {from, to} = range();
      rangeLabel.textContent = mode() === "month" ? fmt(anchor, {month: "long", year: "numeric"})
        : `${fmt(from, {month: "short", day: "numeric"})} – ${fmt(addDays(to, -1), {month: "short", day: "numeric", year: "numeric"})}`;
      const now = Date.now();
      const thisWeek = plans.filter(p => new Date(p.starts_at).getTime() >= weekStartOf(new Date()).getTime() && new Date(p.starts_at).getTime() < addDays(weekStartOf(new Date()), 7).getTime());
      const interviews = thisWeek.filter(p => p.kind === "interview").length;
      const next = plans.filter(p => new Date(p.ends_at).getTime() > now).sort((a, b) => a.starts_at.localeCompare(b.starts_at))[0];
      const clashing = plans.filter(p => window.ICS.clashes(p, plans, busyIn(from, to)).length).length;
      summary.replaceChildren(...[
        el("b", {text: interviews === 1 ? "1 interview this week" : `${interviews} interviews this week`}),
        next ? [" · ", el("span", {text: `next: ${fmt(new Date(next.starts_at), {weekday: "short"})} ${time(new Date(next.starts_at))}, ${window.ICS.summaryOf(next)}`})] : null,
        clashing ? [" · ", el("span", {class: "cal3-sum-clash", text: clashing === 1 ? "1 plan overlaps something" : `${clashing} plans overlap something`})] : null,
      ].flat().filter(Boolean));
      drawSync();
      mode() === "month" ? drawMonth() : drawWeek();
    }

    async function load() {
      const mine = ++loadSeq;
      const {from, to} = range();
      const [list, apps] = await Promise.all([
        api().events.list({from: localIso(addDays(from, -1)), to: localIso(addDays(to, 1))}).catch(() => []),
        api().applications.list({}).catch(() => []),
        loadBusy(),
      ]);
      if (mine !== loadSeq) return;
      plans = list || [];
      followUps = (apps || []).filter(a => a.next_step_date && !a.archived_at && a.next_step_date >= isoDay(from) && a.next_step_date < isoDay(to));
      draw();
      if (pending) { const p = pending; pending = null; openPlan(p, {plans, onSaved: load}); }
    }
    window.addEventListener("fistbump:events", () => { if (!container.hidden) load(); });
    window.I18n?.onChange?.(() => load()); //weekdays, months and times follow the language
    window.addEventListener("fistbump:applications", () => { if (!container.hidden) load(); });
    load();
    return load;
  }

  return {
    mount,
    //from anywhere: open the calendar with the plan form ready, e.g. {job_id, kind: "interview"}
    plan(prefill = {}) { pending = prefill; window.App.go("calendar"); },
    openPlan,
    addToCalendar,
    refreshBusy: () => loadBusy(true),
  };
})();

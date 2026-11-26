// Desktop integration, each part off until turned on in Settings:
//  - a global shortcut (Cmd/Ctrl+Alt+Space) that brings Fistbump forward with the search box focused,
//  - a menu-bar item that says what is due today,
//  - notifications for follow-ups that are due and interviews today or tomorrow, once each,
// plus, always: fistbump:// deep links, and files dropped anywhere on the window (a posting opens in Paste a job,
// a resume goes to the Resume view to import). The shell side lives in src-tauri/src/desktop.rs.
(() => {
  const desk = window.api?.desktop;
  const api = () => window.FistBumpAPI;
  const NOTIFIED_KEY = "fistbump.notified"; //what has been notified, by day, so nothing is said twice

  const pad = n => String(n).padStart(2, "0");
  const isoDay = d => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  const today = () => isoDay(new Date());
  const tomorrow = () => { const d = new Date(); d.setDate(d.getDate() + 1); return isoDay(d); };
  const who = a => [a.position_title, a.company_name].filter(Boolean).join(" at ") || "A job";

  //What needs you now: steps due today or overdue, and interviews today or tomorrow.
  async function dueNow() {
    const apps = await api().applications.list({}).catch(() => []);
    const t = today();
    const items = [];
    for (const a of apps || []) {
      if (!a.next_step_date || a.archived_at || a.status === "Rejected") continue;
      if (a.status === "Interviewing" && (a.next_step_date === t || a.next_step_date === tomorrow())) {
        items.push({key: `interview:${a.id}:${a.next_step_date}`, title: a.next_step_date === t ? "Interview today" : "Interview tomorrow", body: who(a), id: a.job_id});
      } else if (a.next_step_date <= t) {
        items.push({key: `due:${a.id}:${a.next_step_date}`, title: a.next_step_date < t ? "Next step overdue" : "Follow up today", body: who(a), id: a.job_id});
      }
    }
    return items;
  }

  const readNotified = () => {
    try {
      const v = JSON.parse(localStorage.getItem(NOTIFIED_KEY) || "{}");
      return v.day === today() ? v : {day: today(), keys: []};
    } catch { return {day: today(), keys: []}; }
  };
  const saveNotified = v => { try { localStorage.setItem(NOTIFIED_KEY, JSON.stringify(v)); } catch {} };

  let syncing = null;
  async function sync() {
    if (!desk) return;
    if (syncing) return syncing;
    syncing = (async () => {
      const items = await dueNow();
      const n = items.length;
      const status = n === 0 ? "Nothing due today" : n === 1 ? "1 thing needs you today" : `${n} things need you today`;
      await desk.setTray(window.Prefs.get("menuBarItem"), {status, count: n}).catch(() => {});
      if (window.Prefs.get("notifyDue")) {
        const done = readNotified();
        for (const item of items.filter(i => !done.keys.includes(i.key))) {
          await desk.notify(item.title, item.body).catch(() => {});
          done.keys.push(item.key);
        }
        saveNotified(done);
      }
    })().finally(() => { syncing = null; });
    return syncing;
  }

  //The shortcut can be taken by another app; Settings shows what the shell said.
  window.Desktop = {problem: "", sync};
  async function applyShortcut() {
    if (!desk) return;
    const r = await desk.setShortcut(window.Prefs.get("globalShortcut")).catch(error => ({error: {message: error.message}}));
    window.Desktop.problem = r?.error?.message || "";
    window.dispatchEvent(new CustomEvent("fistbump:desktop"));
  }

  if (desk) {
    applyShortcut();
    setTimeout(sync, 3000); //after the first view has settled
    setInterval(sync, 30 * 60 * 1000);
    window.addEventListener("fistbump:applications", () => sync());
    window.Prefs.onChange(key => {
      if (key === "globalShortcut") applyShortcut();
      if (key === "menuBarItem" || key === "notifyDue") sync();
    });
    desk.onFocusSearch(() => window.Omnibox?.focus());
    desk.onDeepLink(link => openLink(link));
  }

  // ---------- fistbump:// links ----------
  //fistbump://open?url=<job link>   opens a job posting, as if pasted in the search box
  //fistbump://job/<id>              opens a saved job
  //fistbump://search?q=<words>      runs a search
  function openLink(link) {
    let url;
    try { url = new URL(link); } catch { return; }
    if (url.protocol !== "fistbump:") return;
    const where = (url.hostname || url.pathname.replace(/^\/+/, "").split("/")[0]).toLowerCase();
    const rest = url.pathname.replace(/^\/+/, "").split("/").filter(Boolean);
    if (where === "open") {
      const target = url.searchParams.get("url") || "";
      if (/^https?:\/\//i.test(target)) window.Omnibox?.route(target);
    } else if (where === "job") {
      const id = Number(rest[rest.length - 1]);
      if (Number.isInteger(id) && id > 0) window.JobPage?.open(id);
    } else if (where === "search") {
      const q = (url.searchParams.get("q") || "").trim();
      if (q) window.Omnibox?.route(q);
    }
  }

  // ---------- files dropped on the window ----------
  const POSTING = ["txt", "md", "markdown", "html", "htm"];
  const RESUME = ["pdf", "docx", "odt"];
  document.addEventListener("drop", async event => {
    const file = event.dataTransfer?.files?.[0];
    if (!file) return;
    //the Resume view and the omnibox take their own drops
    if (event.target.closest?.("#view-resume, #omnibox")) return;
    const ext = (/\.([^.]+)$/.exec(file.name) || [])[1]?.toLowerCase();
    if (RESUME.includes(ext)) {
      window.App.go("resume");
      setTimeout(() => window.ResumeBuilder.importFile?.(file), 0);
    } else if (POSTING.includes(ext) && file.size < 2 * 1024 * 1024) {
      let text = await file.text();
      if (/^html?$/.test(ext)) text = new DOMParser().parseFromString(text, "text/html").body?.innerText || text;
      window.App.openTab("tab-paste");
      const area = document.getElementById("paste-text");
      area.value = text;
      area.dispatchEvent(new Event("input", {bubbles: true}));
      area.focus();
    }
  });

  window.Desktop.openLink = openLink;

  // ---------- the menu bar ----------
  //Each item runs the same code as the button it stands for (src-tauri/src/menus.rs names them).
  const SIZES = ["small", "default", "large", "larger"];
  const step = by => {
    const at = SIZES.indexOf(window.Prefs.get("textSize"));
    window.Prefs.set("textSize", SIZES[Math.max(0, Math.min(SIZES.length - 1, at + by))]);
  };
  const openSettings = section => {
    window.App.go("jobs");
    window.App.openTab("tab-settings");
    if (section) setTimeout(() => document.querySelector(`.st3-jump a[href="#${section}"]`)?.click(), 60);
  };
  const findHere = () => {
    const view = document.querySelector(".app > .main:not([hidden])");
    const box = view?.querySelector('input[type="search"]:not([hidden])');
    if (box && box.offsetParent) { box.focus(); box.select(); } else window.Omnibox?.focus();
  };
  const MENU = {
    "check-updates": () => { openSettings("set-updates"); setTimeout(() => document.getElementById("check-updates")?.click(), 120); },
    "new-search": () => window.Omnibox?.openFind(),
    "paste-job": () => { window.App.openTab("tab-paste"); document.getElementById("paste-text")?.focus(); },
    "import-resume": () => { window.App.go("resume"); setTimeout(() => window.ResumeBuilder.openImport?.(), 0); },
    "export-csv": () => { window.App.go("tracker"); setTimeout(() => window.Tracker.exportCsv?.(), 0); },
    find: findHere,
    search: () => window.Omnibox?.focus(),
    "mode-night": () => window.Prefs.set("mode", "night"),
    "mode-darker": () => window.Prefs.set("mode", "darker"),
    "mode-day": () => window.Prefs.set("mode", "day"),
    "mode-system": () => window.Prefs.set("mode", "system"),
    "bar-top": () => window.Prefs.set("barPosition", window.Prefs.get("barPosition") === "top" ? "bottom" : "top"),
    "text-bigger": () => step(1),
    "text-smaller": () => step(-1),
    "text-actual": () => window.Prefs.set("textSize", "default"),
    back: () => history.back(),
    forward: () => history.forward(),
    "go-home": () => document.getElementById("nav-home").click(),
    "go-jobs": () => document.getElementById("nav-jobs").click(),
    "go-resume": () => document.getElementById("nav-resume").click(),
    "go-tracker": () => document.getElementById("nav-tracker").click(),
    "go-tailor": () => document.getElementById("nav-tailor").click(),
    "go-calendar": () => document.getElementById("nav-calendar").click(),
    shortcuts: () => openSettings("set-keys"),
    report: () => window.Report?.open(),
  };
  window.Desktop.menu = action => MENU[action]?.();
  if (desk?.onMenu) {
    desk.onMenu(action => window.Desktop.menu(action));
    //the ticks in View follow Settings
    const tell = () => desk.setMenuState?.({mode: window.Prefs.get("mode"), barTop: window.Prefs.get("barPosition") === "top"}).catch(() => {});
    tell();
    window.Prefs.onChange(key => { if (key === "mode" || key === "barPosition") tell(); });
  }
})();

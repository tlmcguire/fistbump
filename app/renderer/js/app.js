// Mark the platform on the page so the styles can follow its conventions, then start the router
document.documentElement.classList.add(`platform-${window.api?.platform || "web"}`);

// A file dropped on the window is ignored instead of opening in it.
for (const type of ["dragover", "drop"]) {
  document.addEventListener(type, event => event.preventDefault());
}

// Selections grey out while the window is in the background.
window.addEventListener("blur", () => document.documentElement.classList.add("window-inactive"));
window.addEventListener("focus", () => document.documentElement.classList.remove("window-inactive"));

// Everything below new in phase 3
// Hash routes. Each view is mounted the first time it is shown and is only hidden after that, so
// unsaved edits, an open import, results and scroll positions are still there when you come back.
// The Jobs view (from phases 1 and 2) is written into index.html and binds to it once.
// Home holds nothing to lose and shows what the other views keep, so it is drawn again each time it is shown.
const VIEWS = {
  home: {
    container: "view-home",
    mount: container => window.Home.mount(container),
    redraw: true,
  },
  jobs: {
    container: "main",
    mount: () => window.JobParser.mount(),
  },
  resume: {
    container: "view-resume",
    mount: container => window.ResumeBuilder.mount(container),
  },

  tailor: {
    container: "view-tailor",
    mount: container => window.Tailor.mount(container),
    redraw: true,
    selfAnimated: true,
  },

  //phase 7: the application tracker, loaded again each time it is shown
  tracker: {
    container: "view-tracker",
    mount: container => window.Tracker.mount(container),
    redraw: true,
  },

  //v3: one saved job, at #/job/<id>. Drawn again each time, and when the id changes.
  job: {
    container: "view-job",
    mount: container => window.JobPage.mount(container),
    redraw: true,
  },
};

// A hidden pane forgets how far it was scrolled, so the offsets are noted before hiding and put back after showing.
window.ViewState = {
  saved: new WeakMap(),
  hide(root) {
    const offsets = [];
    for (const node of [root, ...root.querySelectorAll("*")]) {
      if (node.scrollTop || node.scrollLeft) offsets.push([node, node.scrollTop, node.scrollLeft]);
    }
    this.saved.set(root, offsets);
    root.hidden = true;
  },
  show(root) {
    root.hidden = false;
    for (const [node, top, left] of this.saved.get(root) || []) {
      node.scrollTop = top;
      node.scrollLeft = left;
    }
    this.saved.delete(root);
  },
};

const app = document.querySelector(".app");
const mounted = new Map(); // view name -> its redraw function, for views drawn again when shown
let current = null;
const jobsMounted = () => mounted.has("jobs");

const routeName = () => {
  const name = location.hash.replace(/^#\/?/, "").split(/[/?]/)[0];
  return Object.hasOwn(VIEWS, name) ? name : "";
};

function show(name) {
  if (current === name) {
    if (name === "job") mounted.get(name)?.(); //another job opened over this one
    return;
  }
  document.querySelector(".pdf-preview.expanded")?.collapse?.();
  for (const [key, v] of Object.entries(VIEWS)) {
    const container = document.getElementById(v.container);
    if (key !== name && !container.hidden) window.ViewState.hide(container);
  }
  const view = VIEWS[name];
  const container = document.getElementById(view.container);
  window.ViewState.show(container);
  //Tailor animates its own stage and sidebar as they fill, so the page does not also fade in around them
  if (current !== null && !view.selfAnimated) window.UI.play(container, "view-enter", 320);
  if (!mounted.has(name)) {
    const redraw = view.mount(container);
    mounted.set(name, view.redraw ? redraw : null);
  } else {
    mounted.get(name)?.();
  }
  app.dataset.route = name;
  if (JOBS_AREA.has(name)) lastJobsRoute = name;
  markCurrent();
  current = name;
}

// The bottom bar marks Home, Jobs or Resume; Jobs stays marked on Tracker and Tailor, which live under it for now.
// Settings is a tab of the Jobs view, so while it is open nothing in the bar but the gear is marked.
const JOBS_AREA = new Set(["jobs", "tracker", "tailor", "job"]);
let lastJobsRoute = "jobs";
function markCurrent() {
  const name = app.dataset.route;
  const settingsOpen = name === "jobs" && document.getElementById("tab-settings").getAttribute("aria-selected") === "true";
  app.classList.toggle("settings-open", settingsOpen);
  const marks = [["nav-home", name === "home"], ["nav-jobs", JOBS_AREA.has(name) && !settingsOpen], ["nav-resume", name === "resume"],
    ["nav-tracker", name === "tracker"], ["nav-tailor", name === "tailor"]];
  for (const [id, on] of marks) {
    const item = document.getElementById(id);
    if (on) item.setAttribute("aria-current", "page");
    else item.removeAttribute("aria-current");
  }
}

// Every launch starts on Home; it shows new users how to begin.
const defaultRoute = () => "home";

//Runs on start and every hashchange
function route() {
  const name = routeName();
  if (name) {
    show(name);
    return;
  }
  history.replaceState(null, "", `#/${defaultRoute()}`);
  show(routeName());
}

window.addEventListener("hashchange", route);

//Home and Resume open their views, and the Jobs tabs and Settings return to the Jobs view (the view then picks the tab).
//The Jobs view adds its tab listeners when it first mounts, so a click that mounts it is passed on again.
function openJobsTab(tab) {
  if (routeName() === "jobs") return;
  const firstMount = !jobsMounted();
  location.hash = "#/jobs";
  show("jobs");
  if (firstMount) tab.click();
}
let lastJobsTab = "tab-saved"; //the Jobs section Jobs returns to: Saved until another is chosen
for (const id of ["tab-find", "tab-paste", "tab-saved", "tab-settings"]) {
  const tab = document.getElementById(id);
  tab.addEventListener("click", () => {
    if (id !== "tab-settings") lastJobsTab = id;
    openJobsTab(tab);
    setTimeout(markCurrent); //after the Jobs view has marked the tab it selected
  });
}
//Jobs goes back to where you were under it: Tracker, Tailor, or the last Jobs section.
document.getElementById("nav-jobs").addEventListener("click", () => {
  if (lastJobsRoute === "tracker" || lastJobsRoute === "tailor") {
    location.hash = `#/${lastJobsRoute}`;
    return;
  }
  const tab = document.getElementById(lastJobsTab);
  if (routeName() !== "jobs") openJobsTab(tab);
  tab.click();
});
document.getElementById("nav-home").addEventListener("click", () => { location.hash = "#/home"; });
document.getElementById("nav-resume").addEventListener("click", () => { location.hash = "#/resume"; });
document.getElementById("nav-tailor").addEventListener("click", () => { location.hash = "#/tailor"; });
document.getElementById("nav-tracker").addEventListener("click", () => { location.hash = "#/tracker"; });

// What Home needs to open the other views: go("resume") shows a view at once, openTab("tab-find") a Jobs tab.
window.App = {
  go(name) {
    if (routeName() !== name) location.hash = `#/${name}`;
    show(name);
  },
  openTab: id => document.getElementById(id).click(),
};

// The menu's Settings item (Cmd/Ctrl+,) works from any view.
window.api?.app?.onOpenSettings?.(() => {
  const tab = document.getElementById("tab-settings");
  if (routeName() === "jobs") tab.click();
  else openJobsTab(tab);
});

// Keyboard: Cmd/Ctrl+1..5 go to Home, Jobs, Resume, Tracker and Tailor; "/" goes to the search box on the view.
const SHORTCUTS = {1: "nav-home", 2: "nav-jobs", 3: "nav-resume", 4: "nav-tracker", 5: "nav-tailor"};
document.addEventListener("keydown", event => {
  if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && SHORTCUTS[event.key]) {
    event.preventDefault();
    document.getElementById(SHORTCUTS[event.key]).click();
    return;
  }
  if (event.key === "/" && !event.metaKey && !event.ctrlKey && !/^(INPUT|TEXTAREA|SELECT)$/.test(event.target.tagName)) {
    const view = document.getElementById(VIEWS[routeName()]?.container);
    const box = view?.querySelector('input[type="search"]');
    if (box) {
      event.preventDefault();
      box.focus();
      box.select();
    }
  }
});

// Tooltips name each place and its shortcut.
{
  const mod = window.api?.platform === "darwin" ? "\u2318" : "Ctrl+";
  for (const [id, key, label] of [["nav-home", 1, "Home"], ["nav-jobs", 2, "Jobs"], ["nav-resume", 3, "Master resume"],
    ["nav-tracker", 4, "Tracker"], ["nav-tailor", 5, "Tailor"]]) {
    const item = document.getElementById(id);
    item.title = label;
    item.dataset.shortcut = `${mod}${key}`;
  }
  const settingsTab = document.getElementById("tab-settings");
  settingsTab.title = "Settings";
  settingsTab.dataset.shortcut = `${mod},`;
  const filters = document.getElementById("filters-toggle");
  filters.title = "Show or hide search filters";
  document.getElementById("history-toggle").dataset.tip = "Recent searches you can run again";
}

window.Settings.mount();
route();

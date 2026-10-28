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
  if (current === name) return;
  for (const [key, v] of Object.entries(VIEWS)) {
    const container = document.getElementById(v.container);
    if (key !== name && !container.hidden) window.ViewState.hide(container);
  }
  const view = VIEWS[name];
  const container = document.getElementById(view.container);
  window.ViewState.show(container);
  if (!mounted.has(name)) {
    const redraw = view.mount(container);
    mounted.set(name, view.redraw ? redraw : null);
  } else {
    mounted.get(name)?.();
  }
  app.dataset.route = name;
  for (const [id, route] of [["nav-home", "home"], ["nav-resume", "resume"]]) {
    const item = document.getElementById(id);
    if (name === route) item.setAttribute("aria-current", "page");
    else item.removeAttribute("aria-current");
  }
  current = name;
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

//Sidebar: Home and Resume open their views, and the Jobs tabs and Settings return to the Jobs view (the view then picks the tab).
//The Jobs view adds its tab listeners when it first mounts, so a click that mounts it is passed on again.
function openJobsTab(tab) {
  if (routeName() === "jobs") return;
  const firstMount = !jobsMounted();
  location.hash = "#/jobs";
  show("jobs");
  if (firstMount) tab.click();
}
for (const id of ["tab-find", "tab-link", "tab-settings"]) {
  const tab = document.getElementById(id);
  tab.addEventListener("click", () => openJobsTab(tab));
}
document.getElementById("nav-home").addEventListener("click", () => { location.hash = "#/home"; });
document.getElementById("nav-resume").addEventListener("click", () => { location.hash = "#/resume"; });

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

window.Settings.mount();
route();

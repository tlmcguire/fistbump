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
// Hash routes. Each view has mount(container), which returns a cleanup function the router calls before the next view mounts, so views don't leak listeners. Tailor, tracker, and settings will be added later.
// The Jobs view (from phases 1 and 2) is written into index.html and binds to it once, so its "mount" shows that markup and its cleanup hides it.
let jobsMounted = false;
const VIEWS = {
  jobs: {
    container: "main",
    mount(container) {
      if (!jobsMounted) {
        window.JobParser.mount();
        jobsMounted = true;
      }
      container.hidden = false;
      return () => { container.hidden = true; };
    },
  },
  resume: {
    container: "view-resume",
    mount: container => window.ResumeBuilder.mount(container),
  },
};

const app = document.querySelector(".app");
let current = null; // { name, cleanup }

const routeName = () => {
  const name = location.hash.replace(/^#\/?/, "").split(/[/?]/)[0];
  return Object.hasOwn(VIEWS, name) ? name : "";
};

function show(name) {
  if (current?.name === name) return;
  if (current) {
    try {
      current.cleanup?.();
    } catch (err) {
      console.error(`cleanup of ${current.name} failed:`, err);
    }
  }
  const view = VIEWS[name];
  const container = document.getElementById(view.container);
  for (const v of Object.values(VIEWS)) document.getElementById(v.container).hidden = v !== view;
  app.dataset.route = name;
  const resumeItem = document.getElementById("nav-resume");
  if (name === "resume") resumeItem.setAttribute("aria-current", "page");
  else resumeItem.removeAttribute("aria-current");
  current = { name, cleanup: view.mount(container) };
}

// New users start on the Resume view; anyone with a profile starts on Jobs.
async function defaultRoute() {
  try {
    const master = await window.FistBumpAPI.resume.get();
    return master?.profile ? "jobs" : "resume";
  } catch {
    return "jobs";
  }
}

//Runs on start and every hashchange
async function route() {
  const name = routeName();
  if (name) {
    show(name);
    return;
  }
  history.replaceState(null, "", `#/${await defaultRoute()}`);
  show(routeName());
}

window.addEventListener("hashchange", route);

//Sidebar: the Jobs tabs return to the Jobs view (the view then picks the tab), and Resume opens the Resume view.
for (const id of ["tab-find", "tab-link"]) {
  document.getElementById(id).addEventListener("click", () => {
    if (routeName() !== "jobs") location.hash = "#/jobs";
  });
}
document.getElementById("nav-resume").addEventListener("click", () => { location.hash = "#/resume"; });

route();
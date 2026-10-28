// The sidebar's tree: Jobs and Resume fold open and closed like folders, and under them sit the recent
// searches and the saved versions, each one click from where it opens. Which groups are folded is
// remembered on this computer.
(() => {
  const FOLDED_KEY = "fistbump.sidebar-folded";
  const SHOWN = 5;
  const $ = id => document.getElementById(id);

  let folded = [];
  try {
    folded = JSON.parse(localStorage.getItem(FOLDED_KEY) || "[]");
  } catch {
    //starts with every group open
  }

  for (const group of document.querySelectorAll(".tree-group")) {
    const head = group.querySelector(".tree-head");
    const setOpen = open => {
      head.setAttribute("aria-expanded", String(open));
      group.classList.toggle("folded", !open);
    };
    setOpen(!folded.includes(group.dataset.group));
    head.addEventListener("click", () => {
      const open = head.getAttribute("aria-expanded") !== "true";
      setOpen(open);
      folded = open ? folded.filter(g => g !== group.dataset.group) : [...folded, group.dataset.group];
      try {
        localStorage.setItem(FOLDED_KEY, JSON.stringify(folded));
      } catch {
        //folding still works for this session
      }
    });
  }

  //A leaf is one line of text; the full description is its tooltip.
  const leaf = (text, title, onclick) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "tree-leaf";
    button.textContent = text;
    button.title = title;
    button.addEventListener("click", onclick);
    const item = document.createElement("li");
    item.append(button);
    return item;
  };
  const fillList = (list, items) => {
    list.replaceChildren(...items);
    list.hidden = items.length === 0;
  };

  const drawSearches = () => fillList($("tree-searches"), window.Home.recentSearches(SHOWN).map(s =>
    leaf(s.title, `${s.title}, ${s.detail}`, s.onclick)));

  const drawVersions = versions => fillList($("tree-versions"), versions.slice(0, SHOWN).map(v =>
    leaf(v.version_label, v.target_position ? `${v.version_label}, for ${v.target_position} roles` : v.version_label,
      () => window.Home.openResume({version: v.id}))));

  window.addEventListener("fistbump:history", drawSearches);
  window.addEventListener("fistbump:versions", event => drawVersions(event.detail));
  window.addEventListener("storage", drawSearches);

  drawSearches();
  window.FistBumpAPI.resumes.list().then(drawVersions).catch(() => drawVersions([]));
})();

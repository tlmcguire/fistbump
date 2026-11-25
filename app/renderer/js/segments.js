// Segmented controls (Text/PDF, Changes/Edit/PDF, Active/Archived/All, Active/Archived/Trash,
// Instant/AI, and draft version tabs) share one sliding pill thumb that glides to the chosen
// option instead of jumping. It follows aria-selected or aria-pressed, however the view changes them.
(() => {
  const GROUPS = ".tabs, .tracker-scopes, .draft-mode-toggle, .draft-versions, .segmented";
  const CHOSEN = '[aria-selected="true"], [aria-pressed="true"]';

  const sync = group => {
    const chosen = group.querySelector(CHOSEN);
    if (!chosen || !chosen.offsetWidth) { group.classList.remove("has-pill"); return; }
    group.style.setProperty("--pill-x", `${chosen.offsetLeft}px`);
    group.style.setProperty("--pill-w", `${chosen.offsetWidth}px`);
    if (!group.classList.contains("has-pill")) {
      //the first placement does not slide in from the left edge
      group.classList.add("pill-still", "has-pill");
      requestAnimationFrame(() => requestAnimationFrame(() => group.classList.remove("pill-still")));
    }
  };

  const sizes = new ResizeObserver(entries => entries.forEach(e => sync(e.target)));
  const watch = group => {
    if (group.dataset.pill) { sync(group); return; }
    group.dataset.pill = "1";
    sizes.observe(group);
    sync(group);
  };

  new MutationObserver(records => {
    for (const r of records) {
      if (r.type === "attributes" || r.type === "characterData") {
        const node = r.type === "characterData" ? r.target.parentElement : r.target;
        const group = node?.closest?.(GROUPS);
        if (group) sync(group);
      } else {
        r.addedNodes.forEach(n => {
          if (n.nodeType !== 1) return;
          if (n.matches(GROUPS)) watch(n);
          n.querySelectorAll?.(GROUPS).forEach(watch);
        });
        const parentGroup = r.target.closest?.(GROUPS);
        if (parentGroup) sync(parentGroup);
      }
    }
  }).observe(document.body, {subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ["aria-selected", "aria-pressed"]});
  document.querySelectorAll(GROUPS).forEach(watch);
})();

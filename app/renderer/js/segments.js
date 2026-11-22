// Segmented controls (the Text/PDF and Changes/PDF tabs, Active/Archived/All) get a pill that slides to the chosen
// option instead of jumping. It follows aria-selected or aria-pressed, however the view changes them.
(() => {
  const GROUPS = ".tabs, .tracker-scopes";
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
    if (group.dataset.pill) return;
    group.dataset.pill = "1";
    sizes.observe(group);
    sync(group);
  };

  new MutationObserver(records => {
    for (const r of records) {
      if (r.type === "attributes") {
        const group = r.target.closest?.(GROUPS);
        if (group) sync(group);
      } else {
        r.addedNodes.forEach(n => {
          if (n.nodeType !== 1) return;
          if (n.matches(GROUPS)) watch(n);
          n.querySelectorAll?.(GROUPS).forEach(watch);
        });
      }
    }
  }).observe(document.body, {subtree: true, childList: true, attributes: true, attributeFilter: ["aria-selected", "aria-pressed"]});
  document.querySelectorAll(GROUPS).forEach(watch);
})();

// Tooltips in the app's own style, for anything with a title (or data-tip). They wait a moment before showing,
// show at once while you move from one control to the next, follow keyboard focus too, and keep to the window.
// A data-shortcut on the control is shown beside the words as a key. The native tooltip is held back while ours
// shows, and the title is put back after, so translation and screen readers still read it.
(() => {
  const DELAY = 450;   //before the first tooltip
  const WARM = 400;    //after one hides, the next shows at once within this time
  const tip = document.createElement("div");
  tip.className = "tip";
  tip.id = "app-tip";
  tip.setAttribute("role", "tooltip");
  tip.hidden = true;
  document.body.append(tip);

  let target = null;
  let timer = null;
  let lastHidden = 0;

  const textOf = el => el.getAttribute("title") || el.dataset.tipTitle || el.dataset.tip || "";

  const place = el => {
    const r = el.getBoundingClientRect();
    const t = tip.getBoundingClientRect();
    const gap = 8;
    let side = "below";
    let top = r.bottom + gap;
    if (top + t.height > window.innerHeight - 6) { top = r.top - gap - t.height; side = "above"; }
    const left = Math.max(6, Math.min(window.innerWidth - t.width - 6, r.left + r.width / 2 - t.width / 2));
    tip.style.top = `${Math.round(top)}px`;
    tip.style.left = `${Math.round(left)}px`;
    tip.dataset.side = side;
    tip.style.setProperty("--arrow", `${Math.round(r.left + r.width / 2 - left)}px`);
  };

  const show = el => {
    const text = textOf(el);
    if (!text || !el.isConnected) return;
    if (el.hasAttribute("title")) {
      el.dataset.tipTitle = el.getAttribute("title");
      el.removeAttribute("title"); //keeps the system's tooltip from showing on top of ours
    }
    tip.replaceChildren(document.createTextNode(text));
    if (el.dataset.shortcut) {
      const key = document.createElement("kbd");
      key.textContent = el.dataset.shortcut;
      tip.append(" ", key);
    }
    tip.hidden = false;
    tip.classList.remove("in");
    place(el);
    void tip.offsetWidth;
    tip.classList.add("in");
    el.setAttribute("aria-describedby", tip.id);
    target = el;
  };

  const hide = () => {
    clearTimeout(timer);
    if (!target) return;
    if (target.dataset.tipTitle) {
      target.setAttribute("title", target.dataset.tipTitle);
      delete target.dataset.tipTitle;
    }
    if (target.getAttribute("aria-describedby") === tip.id) target.removeAttribute("aria-describedby");
    target = null;
    tip.hidden = true;
    tip.classList.remove("in");
    lastHidden = performance.now();
  };

  const owner = node => node?.closest?.("[title], [data-tip], [data-tip-title]");

  const arm = (el, wait) => {
    if (el === target) return;
    hide();
    clearTimeout(timer);
    if (!textOf(el)) return;
    const warm = performance.now() - lastHidden < WARM;
    timer = setTimeout(() => show(el), warm ? 0 : wait);
  };

  document.addEventListener("mouseover", event => {
    const el = owner(event.target);
    if (!el || el.closest(".tip")) return;
    arm(el, DELAY);
  });
  document.addEventListener("mouseout", event => {
    const el = owner(event.target);
    if (el && !el.contains(event.relatedTarget)) hide();
  });
  document.addEventListener("focusin", event => {
    const el = owner(event.target);
    if (el && event.target.matches(":focus-visible")) arm(el, 200);
  });
  document.addEventListener("focusout", hide);
  document.addEventListener("pointerdown", hide, true);
  document.addEventListener("keydown", event => { if (event.key === "Escape" && target) hide(); });
  window.addEventListener("scroll", hide, true);
  window.addEventListener("blur", hide);
})();

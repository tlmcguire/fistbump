// Small shared pieces for the views: a confirm and alert that live in the page, the confirm-by-pressing-twice
// button, drawn icons, and the one place that decides whether motion plays.
window.UI = (() => {
  const root = document.documentElement;

  //Motion is off when the Reduce motion setting says so (or the system does). Script-driven animation asks here;
  //the stylesheets are switched off by the same .reduce-motion class.
  const motion = () => !root.classList.contains("reduce-motion");

  const make = (tag, attrs = {}, ...kids) => {
    const node = document.createElement(tag);
    for (const [key, value] of Object.entries(attrs)) {
      if (value === null || value === undefined || value === false) continue;
      if (key === "text") node.textContent = value;
      else if (key.startsWith("on")) node.addEventListener(key.slice(2), value);
      else node.setAttribute(key, value === true ? "" : value);
    }
    node.append(...kids.flat().filter(kid => kid !== null && kid !== undefined && kid !== false));
    return node;
  };

  //An icon from the sprite in index.html
  const icon = (name, className = "icon") => {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("class", className);
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", `#i-${name}`);
    svg.append(use);
    return svg;
  };

  //Asks a question in a dialog and gives true or false. Esc and Cancel give false.
  //{confirm, cancel, danger, title}; cancel: null makes it a notice with one button.
  const ask = (message, {confirm = "OK", cancel = "Cancel", danger = false, title = null} = {}) => new Promise(resolve => {
    const previous = document.activeElement;
    const dialog = make("dialog", {class: "ask", "aria-label": title || "Confirm"});
    let done = false;
    const finish = answer => {
      if (done) return;
      done = true;
      resolve(answer);
      //it leaves faster than it came: a short fade and sink, then it is gone
      dialog.classList.add("closing");
      setTimeout(() => {
        dialog.close();
        dialog.remove();
        if (previous && previous.isConnected) previous.focus();
      }, motion() ? 140 : 90);
    };
    const yes = make("button", {type: "button", class: danger ? "danger-solid" : "primary", text: confirm, onclick: () => finish(true)});
    const no = cancel === null ? null : make("button", {type: "button", text: cancel, onclick: () => finish(false)});
    dialog.append(
      title ? make("h2", {text: title}) : null,
      make("p", {text: message}),
      make("div", {class: "ask-actions"}, no, yes));
    dialog.addEventListener("cancel", event => { event.preventDefault(); finish(false); });
    document.body.append(dialog);
    dialog.showModal();
    (danger && no ? no : yes).focus();
  });

  const alert = (message, title = null) => ask(message, {cancel: null, title});

  //A button for something that cannot be undone asks to be pressed again within a few seconds.
  const confirmFirst = (button, label, action) => {
    let original = button.textContent;
    let timer = null;
    const reset = () => {
      clearTimeout(timer);
      button.classList.remove("confirming");
      button.textContent = original;
    };
    button.addEventListener("click", async event => {
      if (!button.classList.contains("confirming")) {
        event.stopImmediatePropagation();
        original = button.textContent;
        button.classList.add("confirming");
        button.textContent = label;
        timer = setTimeout(reset, 4000);
        return;
      }
      reset();
      button.disabled = true;
      try {
        await action();
      } finally {
        button.disabled = false;
      }
    });
    button.addEventListener("blur", reset);
    return button;
  };

  //The mascot, drawn as an image with its own pop-in class
  const pico = (file, className = "mascot") => make("img", {class: className, src: `views/graphics/mascots/${file}`, alt: ""});

  //Plays a one-shot class on a node, then takes it off again. Does nothing when motion is off.
  const play = (node, className, ms = 600) => {
    if (!motion() || !node) return;
    node.classList.remove(className);
    void node.offsetWidth; //restart if it is already running
    node.classList.add(className);
    setTimeout(() => node.classList.remove(className), ms);
  };

  //Makes a list's children arrive one after another. Each child gets --i for the CSS delay.
  const stagger = (parent, step = 28, max = 12) => {
    if (!motion()) return;
    [...parent.children].forEach((child, i) => child.style.setProperty("--i", String(Math.min(i, max) * step) + "ms"));
    parent.classList.remove("stagger");
    void parent.offsetWidth;
    parent.classList.add("stagger");
  };

  //Counts a number up to its value (progress, scores). Plain text under reduced motion.
  const countTo = (node, value, ms = 500) => {
    if (!motion()) { node.textContent = String(value); return; }
    const from = Number(node.dataset.value || 0);
    node.dataset.value = String(value);
    const start = performance.now();
    const tick = now => {
      const t = Math.min(1, (now - start) / ms);
      node.textContent = String(Math.round(from + (value - from) * (1 - Math.pow(1 - t, 3))));
      if (t < 1) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  };

  //Runs a change that redraws much of the window as a view transition: the old picture fades out as the new
  //one comes in. type picks the style in motion.css ("language"); reduced motion gets a short plain crossfade.
  const transition = (update, type) => {
    if (!document.startViewTransition || document.hidden) { update(); return; }
    root.dataset.vt = type;
    const t = document.startViewTransition(update);
    t.finished.finally(() => { delete root.dataset.vt; });
  };

  return {motion, transition, make, icon, ask, alert, confirmFirst, pico, play, stagger, countTo};
})();

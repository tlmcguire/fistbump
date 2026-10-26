// Preferences for this computer: accessibility, search defaults and updates. Kept in local storage and
// applied to the page at once, so the app looks right from its first paint.
window.Prefs = (() => {
  const KEY = "fistbump.prefs";
  const DEFAULTS = {
    textSize: "default",     // small | default | large | larger
    reduceMotion: "system",  // system | on | off
    highContrast: false,
    underlineLinks: false,
    pageSize: 20,            // 20 | 30 | 50 results a page
    defaultSort: "relevance",
    showLogos: true,
    checkUpdatesOnLaunch: false,
  };
  const ZOOM = {small: 0.9, default: 1, large: 1.15, larger: 1.3};
  const ALLOWED = {
    textSize: Object.keys(ZOOM),
    reduceMotion: ["system", "on", "off"],
    pageSize: [20, 30, 50],
    defaultSort: ["relevance", "newest"],
  };

  let values = {...DEFAULTS};
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) || "{}");
    for (const [key, value] of Object.entries(saved)) {
      if (!(key in DEFAULTS)) continue;
      if (ALLOWED[key] ? ALLOWED[key].includes(value) : typeof value === typeof DEFAULTS[key]) values[key] = value;
    }
  } catch {
    //unreadable preferences fall back to the defaults
  }

  const listeners = new Set();

  //applies the preferences that change how the whole page looks
  const apply = () => {
    const root = document.documentElement;
    const reduce = values.reduceMotion === "on" ||
      (values.reduceMotion === "system" && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
    root.classList.toggle("reduce-motion", reduce);
    root.classList.toggle("high-contrast", values.highContrast);
    root.classList.toggle("underline-links", values.underlineLinks);
    //Electron zooms the whole page, layout included; a browser preview has no such call and stays at 100%
    window.api?.setZoom?.(ZOOM[values.textSize]);
  };

  return {
    DEFAULTS,
    get: key => values[key],
    set(key, value) {
      if (!(key in DEFAULTS)) return;
      values = {...values, [key]: value};
      try {
        localStorage.setItem(KEY, JSON.stringify(values));
      } catch {
        //the change still applies for this session
      }
      apply();
      listeners.forEach(fn => fn(key, value));
    },
    onChange: fn => listeners.add(fn),
    apply,
  };
})();
window.Prefs.apply();

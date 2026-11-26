// Preferences for this computer: accessibility, search defaults and updates. Kept in local storage and
// applied to the page at once, so the app looks right from its first paint.
window.Prefs = (() => {
  const KEY = "fistbump.prefs";
  const DEFAULTS = {
    theme: "classic",        // classic | bubblegum | techno | cyber | retro | auto-season | january..december
    accent: "theme",         // theme (the theme's own) | gold | coral | mint | sky | violet | rose
    barPosition: "bottom",   // where the navigation bar sits: bottom | top
    globalShortcut: false,   // Cmd/Ctrl+Alt+Space brings Fistbump forward with the search box, from any app
    menuBarItem: false,      // a menu-bar (tray) item that shows what is due
    notifyDue: false,        // notifications for follow-ups due and interviews coming up
    language: "system",      // system | en | es | fr | de
    textSize: "default",     // small | default | large | larger
    reduceMotion: "system",  // system | on | off
    highContrast: false,
    underlineLinks: false,
    pageSize: 20,            // 20 | 30 | 50 results a page
    defaultSort: "relevance",
    showLogos: true,
    pdfStyle: "classic",     // classic | modern | minimal | compact | executive
    resumePages: 1,          // the length the resume should fit, in pages: 1, 2, or a custom number up to 10
    notifyUpdates: true,     // look for a new version when Fistbump opens, and say so
    skippedUpdate: "",       // a version the user chose not to hear about again
  };
  const ZOOM = {small: 0.9, default: 1, large: 1.15, larger: 1.3};
  const SEASONAL_THEMES = [
    "january", "february", "march", "april", "may", "june",
    "july", "august", "september", "october", "november", "december",
  ];
  const STYLE_THEMES = [
    "classic", "bubblegum", "techno", "cyber", "retro",
    "vaporwave", "dracula", "matcha", "midnight-gold", "ocean-abyss", "arcade",
  ];
  const ALLOWED = {
    theme: [...STYLE_THEMES, "auto-season", ...SEASONAL_THEMES],
    accent: ["theme", "gold", "coral", "mint", "sky", "violet", "rose"],
    barPosition: ["bottom", "top"],
    language: ["system", "en", "es", "fr", "de"],
    textSize: Object.keys(ZOOM),
    reduceMotion: ["system", "on", "off"],
    pageSize: [20, 30, 50],
    defaultSort: ["relevance", "newest"],
    pdfStyle: ["classic", "modern", "minimal", "compact", "executive"],
    resumePages: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],
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
    root.classList.toggle("bar-top", values.barPosition === "top");
    //at the top, the bar is also where the window is dragged from
    const bar = document.querySelector?.(".bottombar");
    if (bar) values.barPosition === "top" ? bar.setAttribute("data-tauri-drag-region", "") : bar.removeAttribute("data-tauri-drag-region");
    if (root.dataset) {
      if (values.accent && values.accent !== "theme") root.dataset.accent = values.accent;
      else delete root.dataset.accent;
    }
    const resolvedTheme = values.theme === "auto-season"
      ? SEASONAL_THEMES[new Date().getMonth()]
      : values.theme;
    if (root.dataset) {
      if (resolvedTheme && resolvedTheme !== "classic") root.dataset.theme = resolvedTheme;
      else delete root.dataset.theme;
    }
    //The desktop shell zooms the whole page, layout included; a browser preview has no such call and stays at 100%
    window.api?.setZoom?.(ZOOM[values.textSize]);
  };

  return {
    DEFAULTS,
    get: key => values[key],
    set(key, value) {
      if (!(key in DEFAULTS)) return;
      if (ALLOWED[key] && !ALLOWED[key].includes(value)) return;
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

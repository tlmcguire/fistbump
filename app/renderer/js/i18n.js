// Interface language. The page's own text is the source: each language file maps an English string to its
// translation, and the page is translated as it is drawn, including text the views add later. A string with
// {0} {1} in it matches text with changing parts ("Round {0}"). "A · B" is translated part by part.
// Unknown strings stay in English, so a missing translation is never blank. Your own text (resumes, job
// postings) is never translated: those places carry translate="no".
window.I18n = (() => {
  const LANGUAGES = [["en", "English"], ["es", "Español"], ["fr", "Français"], ["de", "Deutsch"]];
  const catalogs = {};
  let lang = "en";
  let exact = new Map();
  let patterns = [];
  let busy = false;

  const add = (code, entries) => { catalogs[code] = {...(catalogs[code] || {}), ...entries}; };

  const compile = entries => {
    exact = new Map();
    patterns = [];
    for (const [source, target] of Object.entries(entries)) {
      if (!/\{\d\}/.test(source)) { exact.set(source, target); continue; }
      const order = [];
      const body = source.replace(/[.*+?^$()|[\]\\]/g, "\\$&").replace(/\{(\d)\}/g, (_, n) => { order.push(Number(n)); return "(.+?)"; });
      patterns.push({re: new RegExp(`^${body}$`), order, target});
    }
  };

  const translate = text => {
    if (lang === "en" || typeof text !== "string") return text;
    const lead = text.match(/^\s*/)[0];
    const tail = text.match(/\s*$/)[0];
    const core = text.trim();
    if (!core) return text;
    if (exact.has(core)) return lead + exact.get(core) + tail;
    for (const {re, order, target} of patterns) {
      const m = core.match(re);
      if (m) return lead + target.replace(/\{(\d)\}/g, (_, n) => translate(m[order.indexOf(Number(n)) + 1] ?? "")) + tail;
    }
    if (core.includes(" · ")) return lead + core.split(" · ").map(translate).join(" · ") + tail;
    return text;
  };

  const SKIP = "script, style, textarea, [translate='no'], .listing, .result-body, .preview-text, .diff-col ins, .diff-col del, .pdf-frame";
  const ATTRS = ["placeholder", "aria-label", "title", "alt"];

  const node = n => {
    if (n.nodeType === Node.TEXT_NODE) {
      if (n.parentElement && (n.parentElement.closest(SKIP) || n.parentElement.classList.contains("diff-col"))) return; //your resume text
      const t = translate(n.nodeValue);
      if (t !== n.nodeValue) n.nodeValue = t;
    } else if (n.nodeType === Node.ELEMENT_NODE) {
      if (n.matches(SKIP) || n.closest("[translate='no']")) return;
      for (const a of ATTRS) {
        const v = n.getAttribute(a);
        if (v) { const t = translate(v); if (t !== v) n.setAttribute(a, t); }
      }
      for (const child of n.childNodes) node(child);
    }
  };

  const observer = new MutationObserver(records => {
    if (busy) return;
    busy = true;
    for (const r of records) {
      if (r.type === "childList") r.addedNodes.forEach(node);
      else if (r.type === "characterData") node(r.target);
      else if (r.type === "attributes") node(r.target);
    }
    busy = false;
  });

  const pick = setting => {
    const wanted = setting === "system" ? (navigator.language || "en") : setting;
    const code = String(wanted).slice(0, 2).toLowerCase();
    return LANGUAGES.some(([c]) => c === code) ? code : "en";
  };

  //starts translating the page. Called once, after the page is drawn and the language files are loaded.
  const init = setting => {
    lang = pick(setting);
    document.documentElement.lang = lang;
    if (lang === "en") return;
    compile(catalogs[lang] || {});
    busy = true;
    node(document.body);
    busy = false;
    observer.observe(document.body, {childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ATTRS});
  };

  return {LANGUAGES, add, init, translate, locale: () => lang, t: (s, ...args) => translate(s).replace(/\{(\d)\}/g, (_, n) => args[n] ?? "")};
})();

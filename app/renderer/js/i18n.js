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
  const sourceText = new WeakMap();  //text node -> its English text
  const sourceAttrs = new WeakMap(); //element -> {attribute: English value}

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
    if (!exact.size && !patterns.length) return text;
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

  //Translates one node and what is inside it. The English is kept, so the language can change again later.
  const node = n => {
    if (n.nodeType === Node.TEXT_NODE) {
      if (n.parentElement && (n.parentElement.closest(SKIP) || n.parentElement.classList.contains("diff-col"))) return; //your resume text
      //an <option> with no value attribute takes its value from its text, so translating it would change the data sent
      if (n.parentElement?.tagName === "OPTION" && !n.parentElement.hasAttribute("value")) return;
      const known = sourceText.get(n);
      //text a script wrote after the last translation is the new English
      const source = known && known.shown === n.nodeValue ? known.source : n.nodeValue;
      const t = translate(source);
      sourceText.set(n, {source, shown: t});
      if (t !== n.nodeValue) n.nodeValue = t;
    } else if (n.nodeType === Node.ELEMENT_NODE) {
      if (n.closest("[translate='no']")) return;
      const kept = sourceAttrs.get(n) || {};
      for (const a of ATTRS) {
        const v = n.getAttribute(a);
        if (!v) continue;
        const source = kept[a] && kept[a].shown === v ? kept[a].source : v;
        const t = translate(source);
        kept[a] = {source, shown: t};
        if (t !== v) n.setAttribute(a, t);
      }
      sourceAttrs.set(n, kept);
      //a textarea or your own text keeps its content, but its placeholder and labels are the app's
      if (n.matches(SKIP)) return;
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

  const apply = code => {
    lang = code;
    document.documentElement.lang = lang;
    compile(lang === "en" ? {} : (catalogs[lang] || {}));
    busy = true;
    node(document.body);
    busy = false;
  };

  //starts translating the page. Called once, before the views draw, after the language files load.
  const init = setting => {
    apply(pick(setting));
    observer.observe(document.body, {childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ATTRS});
  };

  //Changes the language in place: every string goes back to its English and on to the new language.
  //Listeners hear about it, for text a view builds from parts (dates, counts).
  const listeners = new Set();
  const change = setting => {
    const code = pick(setting);
    if (code === lang) return;
    apply(code);
    listeners.forEach(fn => fn(code));
  };

  return {LANGUAGES, add, init, change, onChange: fn => listeners.add(fn), translate, locale: () => lang, t: (s, ...args) => translate(s).replace(/\{(\d)\}/g, (_, n) => args[n] ?? "")};
})();

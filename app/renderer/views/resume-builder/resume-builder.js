// The Resume view: edit the master resume (profile, experience, education), preview it as the
// Markdown later phases tailor and print, and save it as a named version.
//
// The forms are built by ResumeBuilder.forms so the import review (step 19) can reuse them.
// All text is set with textContent; nothing from the backend or the user is parsed as HTML.
(() => {
  const api = () => window.FistBumpAPI;
 
  //el("p", {class: "note"}, "text", child) builds an element. Strings become text nodes.
  const el = (tag, attrs = {}, ...children) => {
    const node = document.createElement(tag);
    for (const [key, value] of Object.entries(attrs)) {
      if (value === undefined || value === null || value === false) continue;
      if (key === "class") node.className = value;
      else if (key === "text") node.textContent = value;
      else if (key.startsWith("on")) node.addEventListener(key.slice(2), value);
      else if (key === "value") node.value = value;
      else if (key === "checked" || key === "disabled" || key === "hidden") node[key] = Boolean(value);
      else node.setAttribute(key, value === true ? "" : value);
    }
    for (const child of children.flat(Infinity)) {
      if (child === null || child === undefined || child === false) continue;
      node.append(child instanceof Node ? child : document.createTextNode(String(child)));
    }
    return node;
  };
 
  const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
 
  //"2022-06-01" -> "Jun 2022", as render.Master writes it
  const formatMonth = date => {
    const m = /^(\d{4})-(\d{2})/.exec(date || "");
    if (!m) return date || "";
    const code = window.I18n?.locale();
    if (code && code !== "en") return new Date(Number(m[1]), Number(m[2]) - 1, 1).toLocaleDateString(code, {month: "short", year: "numeric"});
    return `${MONTHS[Number(m[2]) - 1] || ""} ${m[1]}`.trim();
  };
 
  const dateRange = (start, end) => {
    if (!start && !end) return "";
    if (!start) return end ? formatMonth(end) : "";
    const t = window.I18n?.translate || (s => s);
    return `${formatMonth(start)} ${t("to")} ${end ? formatMonth(end) : t("Present")}`;
  };
 
  //A month input holds "2024-06". Stored dates are "2024-06-01"; the API accepts either.
  const toMonth = date => (/^\d{4}-\d{2}/.exec(date || "") || [""])[0];

  // Safari / macOS WKWebView and WebKitGTK do not support <input type="month"> (they degrade to a bare text box).
  const supportsMonthInput = (() => {
    try {
      const probe = document.createElement("input");
      probe.type = "month";
      return probe.type === "month";
    } catch {
      return false;
    }
  })();

  function makeMonthPicker(id, initial) {
    const mSelect = el("select", {id, "aria-label": "Month"},
      el("option", {value: "", text: "Month"}), ...MONTHS.map((name, i) => el("option", {value: String(i + 1).padStart(2, "0"), text: name})));
    const yInput = el("input", {type: "number", min: "1950", max: "2100", step: "1", placeholder: "YYYY", "aria-label": "Year"});
    const wrap = el("div", {class: "month-picker"}, mSelect, yInput);
    const setParts = val => {
      const match = /^(\d{4})-(\d{2})/.exec(val || "");
      yInput.value = match ? match[1] : "";
      mSelect.value = match ? match[2] : "";
    };
    setParts(initial);
    Object.defineProperty(wrap, "value", {
      get() {
        const y = yInput.value.trim();
        const m = mSelect.value;
        if (!y && !m) return "";
        if (/^\d{4}$/.test(y) && /^\d{2}$/.test(m)) return `${y}-${m}`;
        return `${y}-${m}`;
      },
      set(val) { setParts(val); },
    });
    Object.defineProperty(wrap, "disabled", {
      get() { return mSelect.disabled; },
      set(on) { mSelect.disabled = yInput.disabled = Boolean(on); },
    });
    wrap.focus = () => mSelect.focus();
    const origSetAttr = wrap.setAttribute.bind(wrap);
    const origRemAttr = wrap.removeAttribute.bind(wrap);
    wrap.setAttribute = (k, v) => {
      if (k === "aria-invalid" || k === "aria-describedby") { mSelect.setAttribute(k, v); yInput.setAttribute(k, v); }
      else origSetAttr(k, v);
    };
    wrap.removeAttribute = (k) => {
      if (k === "aria-invalid" || k === "aria-describedby") { mSelect.removeAttribute(k); yInput.removeAttribute(k); }
      else origRemAttr(k);
    };
    return wrap;
  }
 
  //Lists are typed as comma-separated text: "sql, excel" <-> ["sql", "excel"]
  const listToText = list => (list || []).join(", ");
  const textToList = text => text.split(",").map(s => s.trim()).filter(Boolean);
 
  //replaceChildren with el()'s rules: arrays are flattened, and null, undefined and false are skipped
  //lists may be nested (a view can return its cards and a footer together), so they are flattened all the way
  const fill = (node, ...children) => node.replaceChildren(...children.flat(Infinity).filter(c => c !== null && c !== undefined && c !== false));
 
  const chips = list => el("ul", {class: "chips"}, list.map(item => el("li", {class: "chip", text: item})));
 
  // ---------- Forms ----------
 
  // A form spec is a list of fields: {name, label, type, required, options, placeholder, wide, hint}.
  // Types: text, email, tel, url, textarea, month, select, list (comma-separated, shown as chips),
  // number. A month field with `current` adds a "current" checkbox that sends null.
  //
  // makeForm returns {el, read(), showError(err), clearErrors(), focus()}. read() returns the request
  // body: blank text becomes null, lists become arrays. Fields the form does not show but the
  // backend needs back unchanged (a full replace) go in `keep`.
  function makeForm(spec, values = {}, {keep = {}, idPrefix = "f"} = {}) {
    const inputs = {};
    const errors = {};
    const formError = el("p", {class: "form-error", role: "alert", hidden: true});
    const uid = `${idPrefix}-${Math.random().toString(36).slice(2, 8)}`;
 
    const rows = spec.map(field => {
      const id = `${uid}-${field.name}`;
      let input;
      let extra = null;
      if (field.type === "textarea") {
        input = el("textarea", {id, rows: field.rows || 4, placeholder: field.placeholder});
        input.value = values[field.name] || "";
      } else if (field.type === "select") {
        input = el("select", {id}, field.options.map(([value, label]) => el("option", {value, text: label})));
        input.value = values[field.name] || "";
      } else if (field.type === "list") {
        input = el("input", {id, type: "text", placeholder: field.placeholder, value: listToText(values[field.name])});
        const preview = el("div", {class: "chip-row"});
        const update = () => preview.replaceChildren(chips(textToList(input.value)));
        input.addEventListener("input", update);
        update();
        extra = preview;
      } else if (field.type === "checkbox") {
        input = el("input", {id, type: "checkbox", checked: Boolean(values[field.name])});
      } else if (field.type === "month") {
        input = supportsMonthInput
          ? el("input", {id, type: "month", value: toMonth(values[field.name])})
          : makeMonthPicker(id, toMonth(values[field.name]));
        if (field.current) {
          const box = el("input", {type: "checkbox", checked: Boolean(values[field.current.when]) && !values[field.name] && !values.date_na});
          const sync = () => {
            if (inputs.date_na?.checked) return;
            input.disabled = box.checked;
            if (box.checked) input.value = "";
          };
          box.addEventListener("change", sync);
          sync();
          input.currentBox = box;
          extra = el("label", {class: "check"}, box, field.current.label);
        }
      } else if (field.type === "number") {
        input = el("input", {id, type: "number", step: field.step || "any", min: field.min, max: field.max, placeholder: field.placeholder});
        input.value = values[field.name] ?? "";
      } else {
        input = el("input", {id, type: field.type || "text", placeholder: field.placeholder, value: values[field.name] || ""});
      }
      if (field.required) input.required = true;
      inputs[field.name] = input;
      errors[field.name] = el("p", {class: "field-error", id: `${id}-error`, hidden: true});
      input.setAttribute("aria-describedby", `${id}-error`);
      if (field.type === "checkbox") {
        return el("div", {class: `form-row${field.wide ? " wide" : ""}`},
          el("label", {class: "check", for: id}, input, field.label),
          field.hint ? el("p", {class: "hint", text: field.hint}) : null,
          errors[field.name]);
      }
      return el("div", {class: `form-row${field.wide ? " wide" : ""}`},
        el("label", {for: id}, field.label, field.required ? el("span", {class: "required", "aria-hidden": "true", text: " *"}) : null),
        input,
        extra,
        field.hint ? el("p", {class: "hint", text: field.hint}) : null,
        errors[field.name]);
    });

    // When "Date range not applicable" is toggled, disable and clear Start/End date inputs.
    if (inputs.date_na && inputs.start_date && inputs.end_date) {
      const syncDateNA = () => {
        const na = inputs.date_na.checked;
        inputs.start_date.disabled = na;
        if (na) inputs.start_date.value = "";
        if (inputs.end_date.currentBox) {
          inputs.end_date.currentBox.disabled = na;
          if (na) inputs.end_date.currentBox.checked = false;
        }
        inputs.end_date.disabled = na || Boolean(inputs.end_date.currentBox?.checked);
        if (na) inputs.end_date.value = "";
      };
      inputs.date_na.addEventListener("change", syncDateNA);
      syncDateNA();
    }
 
    const clearErrors = () => {
      formError.hidden = true;
      for (const name of Object.keys(errors)) {
        errors[name].hidden = true;
        inputs[name].removeAttribute("aria-invalid");
      }
    };
 
    return {
      el: el("div", {class: "form-grid"}, rows, formError),
      inputs,
      read() {
        const body = {...keep};
        const dateNA = Boolean(inputs.date_na?.checked);
        for (const field of spec) {
          const input = inputs[field.name];
          if (field.type === "checkbox") { body[field.name] = Boolean(input.checked); continue; }
          const raw = input.value.trim();
          if (field.type === "list") body[field.name] = textToList(input.value);
          else if (field.type === "number") body[field.name] = raw === "" ? null : Number(raw);
          else if (field.type === "month") body[field.name] = dateNA || input.currentBox?.checked || raw === "" ? null : raw;
          // The name is plain text on the backend, so a blank name is sent as "" rather than null.
          else if (field.required || field.name === "full_name") body[field.name] = raw;
          else body[field.name] = raw === "" ? null : (field.type === "textarea" ? input.value.replace(/\s+$/, "") : raw);
        }
        return body;
      },
      //Shows the backend's message next to the field it names (details.field), else under the form.
      showError(err) {
        clearErrors();
        const name = err?.details?.field;
        if (name && errors[name]) {
          errors[name].textContent = err.message;
          errors[name].hidden = false;
          inputs[name].setAttribute("aria-invalid", "true");
          inputs[name].focus();
          return;
        }
        formError.textContent = err?.message || "Something went wrong.";
        formError.hidden = false;
      },
      clearErrors,
      focus() {
        inputs[spec[0].name].focus();
      },
    };
  }
 
  const PROFILE_FIELDS = [
    {name: "full_name", label: "Full name"},
    {name: "email", label: "Email", type: "email"},
    {name: "phone", label: "Phone", type: "tel"},
    {name: "home_location", label: "Location", placeholder: "e.g. Charleston, SC"},
    {name: "links", label: "Links", type: "list", wide: true, placeholder: "e.g. github.com/ada, linkedin.com/in/ada", hint: "Separate links with commas."},
    {name: "target_positions", label: "Target positions", type: "list", placeholder: "e.g. Data Analyst, Business Analyst"},
    {name: "preferred_mode", label: "Preferred work mode", type: "select",
      options: [["", "No preference"], ["Remote", "Remote"], ["Hybrid", "Hybrid"], ["On-site", "On-site"], ["Any", "Any"]]},
    {name: "summary", label: "Summary", type: "textarea", wide: true, rows: 4},
    {name: "skills", label: "General skills", type: "list", wide: true, placeholder: "e.g. sql, excel, python", hint: "Separate skills with commas."},
  ];
 
  const EXPERIENCE_FIELDS = [
    {name: "job_title", label: "Job title", required: true},
    {name: "company_name", label: "Company", required: true},
    {name: "date_na", label: "Date range not applicable", type: "checkbox", wide: true, hint: "Check this for projects, volunteer work, or entries where dates do not apply."},
    {name: "start_date", label: "Start", type: "month"},
    {name: "end_date", label: "End", type: "month", current: {label: "I work here now", when: "start_date"}},
    {name: "location", label: "Location"},
    {name: "description", label: "Description", type: "textarea", wide: true, rows: 4},
    {name: "impact", label: "Impact", type: "textarea", wide: true, rows: 3, hint: "Results you can measure, such as time saved or revenue."},
    {name: "skills", label: "Skills used", type: "list", wide: true, placeholder: "e.g. sql, tableau"},
  ];
 
  const EDUCATION_FIELDS = [
    {name: "institution", label: "Institution", required: true},
    {name: "degree_level", label: "Degree", placeholder: "e.g. BS"},
    {name: "discipline", label: "Field of study", placeholder: "e.g. Computer Science"},
    {name: "graduation_date", label: "Graduation", type: "month"},
    {name: "gpa", label: "GPA", type: "number", step: "0.01", min: "0", max: "5"},
    {name: "honors", label: "Honors"},
    {name: "details", label: "Details", type: "textarea", wide: true, rows: 3},
  ];
 
  //Profile fields the form does not show; a profile PUT replaces the row, so they are sent back as they are.
  const profileKeep = p => ({min_desired_pay: p?.min_desired_pay ?? null, clearance_certs: p?.clearance_certs || []});
 
  const forms = {
    profile: (values, opts) => makeForm(PROFILE_FIELDS, values || {}, {keep: profileKeep(values), idPrefix: "profile", ...opts}),
    experience: (values, opts) => makeForm(EXPERIENCE_FIELDS, values || {}, {idPrefix: "exp", ...opts}),
    education: (values, opts) => makeForm(EDUCATION_FIELDS, values || {}, {idPrefix: "edu", ...opts}),
  };
 
  // ---------- Export ----------

  //Exports with run(format), PDF first. When the PDF cannot be made, asks before saving Markdown.
  //Gives {saved, path} from main; errors throw to the caller.
  async function exportFile(run) {
    try {
      return await run("pdf");
    } catch (err) {
      if (err.details?.fallback !== "md") throw err;
      if (!await window.UI.ask("PDF export failed. Save as Markdown instead?", {confirm: "Save Markdown", title: "PDF export failed"})) return {saved: false};
      return run("md");
    }
  }

  //"Saved to <path>" with a Show in folder button
  const savedNote = path => {
    const show = el("button", {type: "button", class: "link-button"}, "Show in folder");
    show.addEventListener("click", async () => {
      try {
        await window.FistBumpAPI.shell.showItem(path);
      } catch (err) {
        show.textContent = err.message;
        show.disabled = true;
      }
    });
    return el("p", {class: "export-saved", role: "status"}, el("span", {text: `Saved to ${path}`}), show);
  };

  //The resume's Markdown read as a document: headings, bullets and lines, not symbols. Text only, never HTML.
  const readable = markdown => {
    const unescape = line => line.replace(/\\([#*\-_])/g, "$1").replace(/\*\*(.+?)\*\*/g, "$1");
    const out = [];
    let list = null;
    for (const raw of markdown.split("\n")) {
      const line = unescape(raw.trimEnd());
      let m;
      if (!line.trim()) { list = null; continue; }
      if ((m = line.match(/^(#{1,6})\s+(.*)$/))) {
        list = null;
        out.push(el(m[1].length === 1 ? "h3" : "h4", {class: "doc-h", text: m[2]}));
      } else if ((m = line.match(/^\s*[-*]\s+(.*)$/))) {
        if (!list) { list = el("ul", {class: "doc-list"}); out.push(list); }
        list.append(el("li", {text: m[1]}));
      } else {
        list = null;
        out.push(el("p", {class: "doc-p", text: line}));
      }
    }
    return out;
  };

  // ---------- The view ----------
 
  function mount(container) {
    let disposed = false;
    let master = {profile: null, experiences: [], education: []};
 
    const status = el("p", {class: "resume-status", role: "status"});
    let statusTimer = null;
    const say = (message, isError = false) => {
      status.textContent = message;
      status.classList.toggle("error", isError);
      clearTimeout(statusTimer);
      if (message && !isError) statusTimer = setTimeout(() => { status.textContent = ""; }, 3000);
    };
 
    //Runs fn and reports its error in the status line; errors with a form are shown by the caller.
    const attempt = async (fn, onError) => {
      try {
        return await fn();
      } catch (err) {
        if (disposed) return undefined;
        if (onError) onError(err);
        else say(err.message, true);
        return undefined;
      }
    };
 
    const importButton = el("button", {type: "button"}, "Import file");
    const pasteButton = el("button", {type: "button"}, "Paste text");
    const saveVersionButton = el("button", {type: "button", class: "primary"}, "Save as version");
    container.classList.add("v3", "rs3");
    const inkMark = id => {
      const ns = "http://www.w3.org/2000/svg";
      const svg = document.createElementNS(ns, "svg");
      svg.setAttribute("class", `ink ink-${id}`);
      svg.setAttribute("aria-hidden", "true");
      const use = document.createElementNS(ns, "use");
      use.setAttribute("href", `#ink-${id}`);
      svg.append(use);
      return svg;
    };
    const toolbar = el("header", {class: "toolbar rs3-head"},
      el("div", {class: "toolbar-form resume-toolbar"},
        el("h1", {class: "view-title display-xl"}, "Your ", el("span", {class: "underlined"}, "resume", inkMark("underline"))),
        status,
        el("div", {class: "toolbar-actions"}, importButton, pasteButton, saveVersionButton)));
 
    const profileSection = el("section", {class: "resume-section paper paper-adapt", "aria-labelledby": "resume-profile-h"});
    const experienceSection = el("section", {class: "resume-section paper paper-adapt", "aria-labelledby": "resume-exp-h"});
    const educationSection = el("section", {class: "resume-section paper paper-adapt", "aria-labelledby": "resume-edu-h"});
    const customSectionsPanel = el("section", {class: "resume-section paper paper-adapt", "aria-labelledby": "resume-custom-h"});
    const importSection = el("section", {class: "resume-section import-review paper paper-adapt", "aria-labelledby": "resume-import-h", hidden: true});
    const editor = el("div", {class: "pane resume-editor"}, importSection, profileSection, experienceSection, educationSection, customSectionsPanel);
 
    const previewText = el("div", {class: "resume-doc"});
    const exportNote = el("div");
    const PDF_STYLES = [
      ["classic", "Classic"],
      ["modern", "Modern"],
      ["minimal", "Minimal"],
      ["compact", "Compact"],
      ["executive", "Executive"],
    ];
    const styleSelect = el("select", {id: "resume-pdf-style", class: "pdf-style-select", "aria-label": "PDF style", title: "Choose PDF layout style"},
      PDF_STYLES.map(([v, label]) => el("option", {value: v, text: label})));
    styleSelect.value = window.Prefs?.get("pdfStyle") || "classic";
    const fitBadge = el("span", {class: "fit-pill", role: "status", hidden: true});
    const exportButton = el("button", {type: "button"}, "Export PDF");
    const exportDocxButton = el("button", {type: "button"}, "Export DOCX");
    const versionsList = el("ul", {class: "versions"});
    const versionForm = el("form", {class: "version-form", hidden: true});

    const refreshAudit = () => {
      if (!window.api?.resume?.audit) return;
      api().resume.audit(styleSelect.value).then(a => {
        if (disposed || !a || !a.word_count) { fitBadge.textContent = ""; fitBadge.hidden = true; return; }
        const pages = a.page_count === 1 ? "1 page" : `${a.page_count} pages`;
        const hasWarnings = Boolean(a.warnings?.length);
        fitBadge.textContent = hasWarnings ? `${pages} · ${a.warnings.length} tip${a.warnings.length === 1 ? "" : "s"}` : `${pages} · Ready`;
        fitBadge.dataset.state = hasWarnings ? "warn" : "good";
        fitBadge.title = hasWarnings ? a.warnings.join("\n") : `${a.word_count} words, ${a.quantified_count}/${a.bullet_count} quantified bullets`;
        fitBadge.hidden = false;
      }).catch(() => {});
    };

    //the preview as text or as the PDF it exports
    const pdf = window.PdfPreview.create(async () => (await api().resume.previewPdf(styleSelect.value)).data_base64, {title: "Master resume as a PDF"});
    styleSelect.addEventListener("change", () => {
      window.Prefs?.set("pdfStyle", styleSelect.value);
      refreshAudit();
      if (previewKind === "pdf") pdf.load();
    });
    window.Prefs?.onChange((key, value) => {
      if (key === "pdfStyle" && styleSelect.value !== value) {
        styleSelect.value = value;
        if (previewKind === "pdf") pdf.load();
      }
    });
    const previewPanels = {
      text: el("div", {class: "panel", id: "resume-panel-text", role: "tabpanel", "aria-labelledby": "resume-tab-text"},
        el("p", {class: "hint", text: "This text is what saved versions, tailoring and PDF export start from."}), previewText),
      pdf: el("div", {class: "panel", id: "resume-panel-pdf", role: "tabpanel", "aria-labelledby": "resume-tab-pdf", hidden: true}, pdf.node),
    };
    const previewTabs = [["text", "Text"], ["pdf", "PDF"]].map(([key, label]) => el("button", {
      type: "button", role: "tab", id: `resume-tab-${key}`, "aria-controls": `resume-panel-${key}`,
      "aria-selected": String(key === "text"), tabindex: key === "text" ? "0" : "-1",
      onclick: () => showPreview(key),
    }, label));
    let previewKind = "text";
    const showPreview = key => {
      previewKind = key;
      previewTabs.forEach((tab, i) => {
        const on = ["text", "pdf"][i] === key;
        tab.setAttribute("aria-selected", String(on));
        tab.tabIndex = on ? 0 : -1;
      });
      previewPanels.text.hidden = key !== "text";
      previewPanels.pdf.hidden = key !== "pdf";
      if (key === "pdf") pdf.load();
      else window.UI.play(previewPanels.text, "enter", 400);
    };

    //v3: how ready the master resume is, with each missing piece one click from its field
    const readySection = el("section", {class: "resume-section paper paper-adapt rs3-ready", "aria-labelledby": "resume-ready-h"});
    const drawReady = async () => {
      const m = await api().resume.get().catch(() => null);
      if (disposed || !m) return;
      const p = m.profile;
      const gaps = window.Home?.resumeGaps ? window.Home.resumeGaps(m) : [];
      const done = [];
      if (p?.full_name && (p.email || p.phone)) done.push("Name and contact");
      if (p?.summary) done.push("Summary");
      if (m.experiences.length || m.education.length) {
        const roles = m.experiences.length === 1 ? "1 role" : `${m.experiences.length} roles`;
        const schools = m.education.length === 1 ? "1 school" : `${m.education.length} schools`;
        done.push([m.experiences.length ? roles : "", m.education.length ? schools : ""].filter(Boolean).join(", "));
      }
      const total = done.length + gaps.length;
      const pct = total ? Math.round((done.length / total) * 100) : 0;
      fill(readySection,
        el("div", {class: "paper-head"}, el("h2", {id: "resume-ready-h", text: "Ready to tailor?"})),
        el("div", {class: "rs3-big"}, el("b", {class: "num", text: p ? `${pct}%` : "0%"}), el("span", {text: "complete"})),
        done.map(d => el("div", {class: "rs3-check"}, inkMark("check"), el("span", {text: d}))),
        gaps.map(g => el("div", {class: "rs3-check"}, inkMark("open"), el("span", {text: g.title}),
          el("button", {type: "button", class: "btn", text: "Add", onclick: () => window.ResumeBuilder.reveal?.(g.target)}))),
        !p ? el("p", {class: "ink-2 rs3-note", text: "Start with your name and how to reach you, or import a resume you already have."}) : null);
    };

    const side = el("div", {class: "pane resume-side"},
      readySection,
      el("section", {class: "resume-section paper paper-adapt"},
        el("div", {class: "section-head preview-head"}, el("h2", {text: "Preview"}), el("div", {class: "pdf-controls"}, styleSelect, exportButton, exportDocxButton)),
        el("div", {class: "preview-bar"},
          el("div", {class: "tabs", role: "tablist", "aria-label": "Preview format"}, previewTabs),
          fitBadge),
        exportNote,
        previewPanels.text,
        previewPanels.pdf),
      el("section", {class: "resume-section paper paper-adapt"},
        el("h2", {text: "Saved versions"}),
        versionForm,
        versionsList));
 
    container.replaceChildren(toolbar, el("div", {class: "content resume-content"}, editor, side));
 
    // --- Preview ---
 
    const refreshPreview = () => attempt(async () => {
      const {markdown} = await api().resume.preview();
      if (disposed) return;
      fill(previewText, markdown.trim() ? readable(markdown) : el("p", {class: "hint", text: "Your resume appears here once you save your profile."}));
      refreshAudit();
      drawReady();
      if (previewKind === "pdf") pdf.load(); //the PDF follows the text
    });
 
    // --- Export ---

    exportButton.addEventListener("click", async () => {
      exportButton.disabled = true;
      exportNote.replaceChildren();
      const result = await attempt(() => exportFile(format => api().resume.export(format, styleSelect.value)));
      if (disposed) return;
      exportButton.disabled = false;
      if (result?.saved) exportNote.replaceChildren(savedNote(result.path));
    });

    exportDocxButton.addEventListener("click", async () => {
      exportDocxButton.disabled = true;
      exportNote.replaceChildren();
      const result = await attempt(() => api().resume.export("docx", styleSelect.value));
      if (disposed) return;
      exportDocxButton.disabled = false;
      if (result?.saved) exportNote.replaceChildren(savedNote(result.path));
    });

    // --- Profile ---
 
    let profileForm = null;
    function renderProfile() {
      const form = forms.profile(master.profile);
      profileForm = form;
      const save = el("button", {type: "submit", class: "primary"}, master.profile ? "Save profile" : "Create profile");

      // Clear all empties every field at once. Nothing is saved until Save profile, and until the next
      // edit the button offers to put the cleared values back.
      const clear = el("button", {type: "button", class: "form-actions-start"}, "Clear all");
      let cleared = null;
      const setValue = (input, value) => {
        input.value = value;
        input.dispatchEvent(new Event("input", {bubbles: true})); //list fields redraw their chips
      };
      const resetClear = () => {
        cleared = null;
        clear.textContent = "Clear all";
      };
      clear.addEventListener("click", () => {
        const inputs = Object.values(form.inputs);
        if (cleared) {
          inputs.forEach((input, i) => setValue(input, cleared[i]));
          resetClear();
          say("");
          return;
        }
        const values = inputs.map(input => input.value);
        if (values.every(v => v === "")) return;
        inputs.forEach(input => setValue(input, ""));
        form.clearErrors();
        cleared = values;
        clear.textContent = "Undo clear";
        say("Fields cleared. Save profile to keep it this way.");
        form.focus();
      });
      const formEl = el("form", {class: "resume-form", novalidate: true},
        form.el, el("div", {class: "form-actions"}, clear, save));
      formEl.addEventListener("input", event => {
        if (event.isTrusted && cleared) resetClear();
      });
      formEl.addEventListener("submit", async event => {
        event.preventDefault();
        save.disabled = true;
        const created = !master.profile;
        const profile = await attempt(() => api().resume.putProfile(form.read()), err => form.showError(err));
        save.disabled = false;
        if (!profile || disposed) return;
        master.profile = profile;
        form.clearErrors();
        resetClear();
        say("Profile saved.");
        if (created) {
          renderProfile();
          renderEntries();
        }
        refreshPreview();
      });
      fill(profileSection,
        el("h2", {id: "resume-profile-h", text: "Profile"}),
        master.profile ? null : el("p", {class: "hint", text: "Start with your name and contact details. Experience and education come next."}),
        formEl);
    }
 
    // --- Experience and education lists ---
 
    const KINDS = {
      experience: {
        section: experienceSection, heading: "Experience", headingId: "resume-exp-h", add: "Add experience",
        list: () => master.experiences, set: list => { master.experiences = list; },
        form: forms.experience,
        create: body => api().resume.createExperience(body),
        update: (id, body) => api().resume.updateExperience(id, body),
        remove: id => api().resume.deleteExperience(id),
        reload: async () => { master.experiences = (await api().resume.get()).experiences; },
        fields: EXPERIENCE_FIELDS,
        title: e => [e.job_title, e.company_name].filter(Boolean).join(", "),
        meta: e => [e.date_na ? "" : dateRange(e.start_date, e.end_date), e.location].filter(Boolean).join(" · "),
        body: e => [e.description, e.impact].filter(Boolean),
        skills: e => e.skills || [],
      },
      education: {
        section: educationSection, heading: "Education", headingId: "resume-edu-h", add: "Add education",
        list: () => master.education, set: list => { master.education = list; },
        form: forms.education,
        create: body => api().resume.createEducation(body),
        update: (id, body) => api().resume.updateEducation(id, body),
        remove: id => api().resume.deleteEducation(id),
        reload: async () => { master.education = (await api().resume.get()).education; },
        fields: EDUCATION_FIELDS,
        title: e => {
          const degree = [e.degree_level, e.discipline].filter(Boolean).join(" in ");
          return degree ? `${degree}, ${e.institution}` : e.institution;
        },
        meta: e => [formatMonth(e.graduation_date), e.gpa != null ? `GPA ${Number(e.gpa).toFixed(2)}` : "", e.honors].filter(Boolean).join(" · "),
        body: e => [e.details].filter(Boolean),
        skills: () => [],
      },
    };
 
    //The request body for a full replace of an entry: its writable fields as stored.
    const bodyOf = (kind, entry) => {
      const body = {};
      for (const f of KINDS[kind].fields) body[f.name] = entry[f.name] ?? (f.type === "list" ? [] : f.type === "checkbox" ? false : null);
      body.sort_order = entry.sort_order;
      return body;
    };
 
    //Which entry is open in a form: {kind, id} for an edit, {kind, id: null} for a new one.
    let editing = null;
 
    function entryForm(kind, entry) {
      const k = KINDS[kind];
      const form = k.form(entry || {});
      const save = el("button", {type: "submit", class: "primary"}, entry ? "Save" : k.add);
      const cancel = el("button", {type: "button"}, "Cancel");
      const formEl = el("form", {class: "resume-form entry-form", novalidate: true},
        form.el, el("div", {class: "form-actions"}, cancel, save));
      cancel.addEventListener("click", () => {
        editing = null;
        renderEntries();
      });
      formEl.addEventListener("submit", async event => {
        event.preventDefault();
        save.disabled = true;
        const body = form.read();
        const saved = await attempt(
          () => entry ? k.update(entry.id, {...body, sort_order: entry.sort_order}) : k.create(body),
          err => form.showError(err));
        save.disabled = false;
        if (!saved || disposed) return;
        k.set(entry ? k.list().map(e => (e.id === saved.id ? saved : e)) : [...k.list(), saved]);
        editing = null;
        say(entry ? "Saved." : "Added.");
        renderEntries();
        refreshPreview();
      });
      queueMicrotask(() => form.focus());
      return formEl;
    }
 
    function entryCard(kind, entry, index, count) {
      const k = KINDS[kind];
      const title = k.title(entry);
      const button = (label, onclick, attrs = {}) => el("button", {type: "button", onclick, ...attrs}, label);
      return el("article", {class: "entry"},
        el("div", {class: "entry-head"},
          el("div", {},
            el("h3", {text: title}),
            k.meta(entry) ? el("p", {class: "entry-meta", text: k.meta(entry)}) : null),
          el("div", {class: "entry-actions"},
            button("Up", () => move(kind, index, -1), {disabled: index === 0, "aria-label": `Move ${title} up`}),
            button("Down", () => move(kind, index, 1), {disabled: index === count - 1, "aria-label": `Move ${title} down`}),
            button("Edit", () => { editing = {kind, id: entry.id}; renderEntries(); }, {"aria-label": `Edit ${title}`}),
            button("Delete", () => removeEntry(kind, entry), {class: "danger", "aria-label": `Delete ${title}`}))),
        k.body(entry).map(text => el("p", {class: "entry-text", text})),
        k.skills(entry).length ? chips(k.skills(entry)) : null);
    }
 
    function renderEntries() {
      for (const kind of Object.keys(KINDS)) {
        const k = KINDS[kind];
        const list = k.list();
        const adding = editing?.kind === kind && editing.id === null;
        const addButton = el("button", {type: "button", disabled: !master.profile || adding}, k.add);
        addButton.addEventListener("click", () => {
          editing = {kind, id: null};
          renderEntries();
        });
        const items = list.map((entry, i) =>
          editing?.kind === kind && editing.id === entry.id ? entryForm(kind, entry) : entryCard(kind, entry, i, list.length));
        fill(k.section,
          el("div", {class: "section-head"}, el("h2", {id: k.headingId, text: k.heading}), addButton),
          !master.profile ? el("p", {class: "hint", text: "Save your profile first."}) : null,
          master.profile && list.length === 0 && !adding ? el("p", {class: "empty", text: `No ${k.heading.toLowerCase()} entries yet.`}) : null,
          items,
          adding ? entryForm(kind, null) : null);
      }
    }
 
    async function removeEntry(kind, entry) {
      const k = KINDS[kind];
      if (!await window.UI.ask(`Delete "${k.title(entry)}"? This cannot be undone.`, {confirm: "Delete", danger: true, title: "Delete entry"})) return;
      const ok = await attempt(() => k.remove(entry.id));
      if (!ok || disposed) return;
      k.set(k.list().filter(e => e.id !== entry.id));
      say("Deleted.");
      renderEntries();
      refreshPreview();
    }
 
    // Moving an entry is a PUT with a new sort_order on the two rows that swap. Rows with the same
    // sort_order cannot swap, so then the whole list is renumbered. If a write fails, the list is
    // reloaded so it shows what was saved.
    async function move(kind, index, dir) {
      const k = KINDS[kind];
      const list = k.list();
      const a = list[index];
      const b = list[index + dir];
      if (!a || !b) return;
      const writes = [];
      if (a.sort_order !== b.sort_order) {
        writes.push([a, b.sort_order], [b, a.sort_order]);
      } else {
        const order = [...list];
        [order[index], order[index + dir]] = [b, a];
        order.forEach((entry, i) => { if (entry.sort_order !== i) writes.push([entry, i]); });
      }
      try {
        const saved = [];
        for (const [entry, sortOrder] of writes) saved.push(await k.update(entry.id, {...bodyOf(kind, entry), sort_order: sortOrder}));
        if (disposed) return;
        const byId = new Map(saved.map(e => [e.id, e]));
        k.set(list.map(e => byId.get(e.id) || e).sort((x, y) => x.sort_order - y.sort_order || x.id - y.id));
      } catch (err) {
        if (disposed) return;
        say(`${err.message} The list was reloaded.`, true);
        await attempt(k.reload);
      }
      if (disposed) return;
      renderEntries();
      refreshPreview();
    }
 
    // --- Saved versions ---
 
    const formatSaved = ts => {
      const d = new Date(ts);
      return Number.isNaN(d.getTime()) ? ts : d.toLocaleString(undefined, {dateStyle: "medium", timeStyle: "short"});
    };
 
    let versions = [];
    let renaming = null; //id of the version being renamed
    let showing = null;  //{id, content} of the version whose text is open
 
    function renderVersions() {
      window.dispatchEvent(new CustomEvent("fistbump:versions", {detail: versions}));
      if (versions.length === 0) {
        versionsList.replaceChildren(el("li", {class: "empty", text: "No saved versions yet. Save one to keep this resume as it is now."}));
        return;
      }
      versionsList.replaceChildren(...versions.map(v => {
        if (renaming === v.id) {
          const input = el("input", {type: "text", value: v.version_label, "aria-label": "Version name"});
          const error = el("p", {class: "field-error", hidden: true});
          const form = el("form", {class: "version-rename"}, input,
            el("button", {type: "submit", class: "primary"}, "Rename"),
            el("button", {type: "button", onclick: () => { renaming = null; renderVersions(); }}, "Cancel"), error);
          form.addEventListener("submit", async event => {
            event.preventDefault();
            const out = await attempt(() => api().resumes.update(v.id, {version_label: input.value}), err => {
              error.textContent = err.message;
              error.hidden = false;
            });
            if (!out || disposed) return;
            versions = versions.map(x => (x.id === v.id ? {...x, version_label: out.version_label, updated_at: out.updated_at} : x));
            renaming = null;
            renderVersions();
          });
          queueMicrotask(() => input.select());
          return el("li", {class: "version"}, form);
        }
        const open = showing?.id === v.id;
        return el("li", {class: "version"},
          el("div", {class: "version-head"},
            el("div", {},
              el("strong", {text: v.version_label}),
              el("small", {text: [v.target_position, `saved ${formatSaved(v.created_at)}`].filter(Boolean).join(" · ")})),
            el("div", {class: "entry-actions"},
              el("button", {type: "button", "aria-expanded": String(open), onclick: () => toggleVersion(v)}, open ? "Hide" : "Show"),
              el("button", {type: "button", onclick: () => { renaming = v.id; renderVersions(); }}, "Rename"),
              el("button", {type: "button", class: "danger", onclick: () => deleteVersion(v)}, "Delete"))),
          open ? el("pre", {class: "preview-text", text: showing.content}) : null);
      }));
    }
 
    async function toggleVersion(v) {
      if (showing?.id === v.id) {
        showing = null;
        renderVersions();
        return;
      }
      const full = await attempt(() => api().resumes.get(v.id));
      if (!full || disposed) return;
      showing = {id: v.id, content: full.content};
      renderVersions();
    }
 
    async function deleteVersion(v) {
      if (!await window.UI.ask(`Delete the saved version "${v.version_label}"? This cannot be undone.`, {confirm: "Delete", danger: true, title: "Delete version"})) return;
      const ok = await attempt(() => api().resumes.delete(v.id));
      if (!ok || disposed) return;
      versions = versions.filter(x => x.id !== v.id);
      if (showing?.id === v.id) showing = null;
      say("Version deleted.");
      renderVersions();
    }
 
    const loadVersions = () => attempt(async () => {
      const list = await api().resumes.list();
      if (disposed) return;
      versions = list;
      renderVersions();
    });
 
    // "Save as version" asks for a name, then saves the master resume as it is now (from_master).
    saveVersionButton.addEventListener("click", () => {
      if (!master.profile) {
        say("Save your profile first.", true);
        return;
      }
      const today = new Date().toLocaleDateString(undefined, {dateStyle: "medium"});
      const form = makeForm([
        {name: "version_label", label: "Version name", required: true},
        {name: "target_position", label: "Target position", placeholder: "Optional"},
      ], {version_label: `Master resume, ${today}`}, {idPrefix: "version"});
      const save = el("button", {type: "submit", class: "primary"}, "Save version");
      const cancel = el("button", {type: "button", onclick: () => { versionForm.hidden = true; }}, "Cancel");
      versionForm.replaceChildren(form.el, el("div", {class: "form-actions"}, cancel, save));
      versionForm.hidden = false;
      versionForm.onsubmit = async event => {
        event.preventDefault();
        save.disabled = true;
        const out = await attempt(() => api().resumes.create({...form.read(), from_master: true}), err => form.showError(err));
        save.disabled = false;
        if (!out || disposed) return;
        versionForm.hidden = true;
        versions = [out, ...versions];
        say(`Saved "${out.version_label}".`);
        renderVersions();
      };
      form.inputs.version_label.select();
    });
 
    // --- Import ---
    //
    // A file (picker or drop) or pasted text becomes a draft from POST /v1/resume/import. The draft
    // is shown in the same forms as the builder; nothing is saved until Apply.

    const IMPORT_EXTENSIONS = ["pdf", "docx", "odt", "txt", "md"];
    const MAX_IMPORT_BYTES = 5 * 1024 * 1024;

    let importing = false;

    const showEditor = shown => {
      importSection.hidden = shown;
      for (const section of [profileSection, experienceSection, educationSection, customSectionsPanel]) section.hidden = !shown;
      importButton.disabled = pasteButton.disabled = importing;
    };

    const closeImport = () => {
      importSection.replaceChildren();
      showEditor(true);
    };

    //"37 words of hidden text were found and left out (same color as the background)"
    const hiddenNotice = hidden => {
      const words = hidden.words === 1 ? "1 word" : `${hidden.words} words`;
      const reasons = (hidden.reasons || []).join(", ");
      return el("div", {class: "import-notice", role: "note"},
        el("p", {text: `${words} of hidden text ${hidden.words === 1 ? "was" : "were"} found and left out${reasons ? ` (${reasons})` : ""}.`}),
        hidden.sample ? el("details", {}, el("summary", {text: "Show what was left out"}), el("pre", {class: "preview-text", text: hidden.sample})) : null);
    };

    const runImport = async (call, source) => {
      if (importing) return;
      importing = true;
      importButton.disabled = pasteButton.disabled = true;
      say("Reading your resume…");
      let result;
      try {
        result = await attempt(call);
      } finally {
        importing = false;
        if (!disposed) importButton.disabled = pasteButton.disabled = false;
      }
      if (disposed || !result) return;
      say("");
      if (result.canceled) return;
      renderReview(result, source);
    };

    function renderReview(result, source) {
      const draft = result.draft || {};
      const profileForm = forms.profile(draft.profile || {}, {idPrefix: "import-profile"});
      const entries = {experience: [], education: []};
      const lists = {experience: el("div", {class: "import-entries"}), education: el("div", {class: "import-entries"})};

      const addEntry = (kind, values) => {
        const form = KINDS[kind].form(values, {idPrefix: `import-${kind}`});
        const item = {form};
        const remove = el("button", {type: "button", class: "danger"}, "Remove");
        item.el = el("article", {class: "entry"},
          el("div", {class: "entry-head"}, el("h3", {text: KINDS[kind].title(values) || `New ${kind}`}), el("div", {class: "entry-actions"}, remove)),
          form.el);
        remove.addEventListener("click", () => {
          entries[kind] = entries[kind].filter(x => x !== item);
          item.el.remove();
        });
        entries[kind].push(item);
        lists[kind].append(item.el);
      };
      (draft.experiences || []).forEach(e => addEntry("experience", e));
      (draft.education || []).forEach(e => addEntry("education", e));

      const warnings = [...(result.size_warnings || []), ...(draft.warnings || [])];
      const replace = el("input", {type: "checkbox"});
      const keepCopy = el("input", {type: "checkbox", checked: true});
      const error = el("p", {class: "form-error", role: "alert", hidden: true});
      const apply = el("button", {type: "submit", class: "primary"}, "Apply");
      const cancel = el("button", {type: "button", onclick: closeImport}, "Discard");
      const formEl = el("form", {class: "resume-form", novalidate: true},
        el("h3", {text: "Profile"}), profileForm.el,
        el("div", {class: "section-head"}, el("h3", {text: "Experience"}),
          el("button", {type: "button", onclick: () => addEntry("experience", {})}, "Add experience")),
        lists.experience,
        el("div", {class: "section-head"}, el("h3", {text: "Education"}),
          el("button", {type: "button", onclick: () => addEntry("education", {})}, "Add education")),
        lists.education,
        el("label", {class: "check"}, replace, "Replace my current entries"),
        el("label", {class: "check"}, keepCopy, "Keep a copy of the original"),
        error,
        el("div", {class: "form-actions"}, cancel, apply));

      formEl.addEventListener("submit", async event => {
        event.preventDefault();
        error.hidden = true;
        const today = new Date().toLocaleDateString(undefined, {dateStyle: "medium"});
        const body = {
          draft: {
            profile: profileForm.read(),
            experiences: entries.experience.map(x => x.form.read()),
            education: entries.education.map(x => x.form.read()),
            skills: draft.skills || [],
          },
          replace: replace.checked,
        };
        if (keepCopy.checked) body.save_copy = {version_label: `Imported ${source || "resume"}, ${today}`, content: result.text};
        apply.disabled = true;
        const saved = await attempt(() => api().resume.importApply(body), err => {
          // A 422 names the entry at fault; show the message on that entry's form.
          const d = err?.details || {};
          const target = d.experience_index != null ? entries.experience[d.experience_index]?.form
            : d.education_index != null ? entries.education[d.education_index]?.form
            : d.field ? profileForm : null;
          if (target) target.showError(err);
          else {
            error.textContent = err?.message || "Something went wrong.";
            error.hidden = false;
          }
        });
        apply.disabled = false;
        if (!saved || disposed) return;
        master = saved;
        editing = null;
        closeImport();
        say("Import applied.");
        renderProfile();
        renderEntries();
        refreshPreview();
        if (body.save_copy) loadVersions();
      });

      fill(importSection,
        el("h2", {id: "resume-import-h", text: "Review import"}),
        el("p", {class: "hint", text: result.notice || "Review every field before applying. Nothing is saved yet."}),
        warnings.length ? el("ul", {class: "import-warnings"}, warnings.map(w => el("li", {text: w}))) : null,
        result.hidden_text ? hiddenNotice(result.hidden_text) : null,
        formEl);
      showEditor(false);
      importSection.scrollIntoView?.({block: "start"});
    }

    importButton.addEventListener("click", () => runImport(() => api().resume.importFile(), null));

    pasteButton.addEventListener("click", () => {
      const text = el("textarea", {rows: 14, "aria-label": "Resume text", placeholder: "Paste the text of your resume here."});
      const read = el("button", {type: "submit", class: "primary"}, "Read text");
      const formEl = el("form", {class: "resume-form"}, text,
        el("div", {class: "form-actions"}, el("button", {type: "button", onclick: closeImport}, "Cancel"), read));
      formEl.addEventListener("submit", event => {
        event.preventDefault();
        runImport(() => api().resume.importText(text.value), "pasted text");
      });
      fill(importSection, el("h2", {id: "resume-import-h", text: "Paste your resume"}), formEl);
      showEditor(false);
      queueMicrotask(() => text.focus());
    });

    // A file dropped anywhere on the view. The extension and size are checked here for a quick
    // message; the backend checks the bytes again.
    container.addEventListener("dragover", event => {
      if (importing || !event.dataTransfer?.types?.includes("Files")) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = "copy";
    });
    container.addEventListener("drop", async event => {
      const file = event.dataTransfer?.files?.[0];
      if (!file || importing) return;
      event.preventDefault();
      const ext = (/\.([^.]+)$/.exec(file.name) || [])[1]?.toLowerCase();
      if (!IMPORT_EXTENSIONS.includes(ext)) return say(`${file.name} is not a PDF, .docx, .odt, .txt or .md file.`, true);
      if (file.size > MAX_IMPORT_BYTES) return say("The file is larger than 5 MB. Export it again without images, or paste the text instead.", true);
      const bytes = await file.arrayBuffer();
      runImport(() => api().resume.importData(file.name, bytes), file.name);
    });

    // --- Custom sections & section ordering ---
    const DEFAULT_ORDER = ["summary", "experience", "education", "skills", "custom"];
    const ORDER_LABELS = {summary: "Summary", experience: "Experience", education: "Education", skills: "Skills", custom: "Custom sections"};
    let sectionOrder = [...DEFAULT_ORDER];
    let customSections = [];

    const saveCustomSettings = async (patch, msg) => {
      if (!window.api?.settings?.put) return;
      try {
        await window.FistBumpAPI.request(() => window.api.settings.put(patch));
        say(msg);
        refreshPreview();
      } catch (err) {
        say(err.message, true);
      }
    };

    function renderCustomSections() {
      const orderList = el("div", {class: "chips"}, sectionOrder.map((key, idx) => el("span", {class: "chip"},
        ORDER_LABELS[key] || key, " ",
        el("button", {type: "button", class: "link-button", disabled: idx === 0, "aria-label": `Move ${ORDER_LABELS[key]} up`, onclick: async () => {
          [sectionOrder[idx - 1], sectionOrder[idx]] = [sectionOrder[idx], sectionOrder[idx - 1]];
          renderCustomSections();
          await saveCustomSettings({"resume.section_order": sectionOrder}, "Section order updated.");
        }}, "↑"), " ",
        el("button", {type: "button", class: "link-button", disabled: idx === sectionOrder.length - 1, "aria-label": `Move ${ORDER_LABELS[key]} down`, onclick: async () => {
          [sectionOrder[idx + 1], sectionOrder[idx]] = [sectionOrder[idx], sectionOrder[idx + 1]];
          renderCustomSections();
          await saveCustomSettings({"resume.section_order": sectionOrder}, "Section order updated.");
        }}, "↓"))));

      const titleInput = el("input", {type: "text", placeholder: "e.g. Projects, Certifications, Awards"});
      const bodyInput = el("textarea", {rows: 3, placeholder: "- Built an open-source CLI tool in Go..."});
      const addBtn = el("button", {type: "button", class: "primary", onclick: async () => {
        const t = titleInput.value.trim();
        const c = bodyInput.value.trim();
        if (!t || !c) { say("Enter both a section title and content.", true); return; }
        customSections = [...customSections, {id: String(Date.now()), title: t, content: c, visible: true}];
        titleInput.value = "";
        bodyInput.value = "";
        renderCustomSections();
        await saveCustomSettings({"resume.custom_sections": JSON.stringify(customSections)}, `Added "${t}" section.`);
      }}, "Add custom section");

      const cards = customSections.map((cs, i) => el("article", {class: "entry"},
        el("div", {class: "entry-head"},
          el("div", {}, el("h3", {text: cs.title}), el("p", {class: "entry-meta", text: cs.visible ? "Shown in resume" : "Hidden"})),
          el("div", {class: "entry-actions"},
            el("button", {type: "button", onclick: async () => {
              customSections[i] = {...cs, visible: !cs.visible};
              renderCustomSections();
              await saveCustomSettings({"resume.custom_sections": JSON.stringify(customSections)}, cs.visible ? "Section hidden." : "Section shown.");
            }}, cs.visible ? "Hide" : "Show"),
            el("button", {type: "button", class: "danger", onclick: async () => {
              customSections = customSections.filter((_, idx) => idx !== i);
              renderCustomSections();
              await saveCustomSettings({"resume.custom_sections": JSON.stringify(customSections)}, "Custom section removed.");
            }}, "Delete"))),
        el("p", {class: "entry-text", text: cs.content})));

      fill(customSectionsPanel,
        el("h2", {id: "resume-custom-h", text: "Custom sections & section order"}),
        el("p", {class: "hint", text: "Reorder how sections appear in your preview, PDF, and DOCX exports."}),
        orderList,
        cards,
        el("div", {class: "entry-form"},
          el("div", {class: "form-grid"},
            el("div", {class: "form-row wide"}, el("label", {text: "Section title"}), titleInput),
            el("div", {class: "form-row wide"}, el("label", {text: "Content (Markdown bullets supported)"}), bodyInput)),
          el("div", {class: "form-actions"}, addBtn)));
    }

    // --- Load ---
 
    const ready = (async () => {
      const loaded = await attempt(() => api().resume.get());
      if (disposed) return;
      if (loaded) master = loaded;
      if (window.api?.settings?.get) {
        try {
          const st = await window.FistBumpAPI.request(() => window.api.settings.get());
          if (Array.isArray(st?.["resume.section_order"]) && st["resume.section_order"].length) sectionOrder = [...st["resume.section_order"]];
          if (typeof st?.["resume.custom_sections"] === "string") {
            const parsed = JSON.parse(st["resume.custom_sections"] || "[]");
            if (Array.isArray(parsed)) customSections = parsed;
          }
        } catch {}
      }
      renderProfile();
      renderEntries();
      renderCustomSections();
      refreshPreview();
      await loadVersions();
    })();

    // Home opens this view at something to fill in or look at: {field} in the profile, {kind, id, field}
    // for an entry, {add: kind} for a new one, or {version: id}. Work in progress is never thrown away:
    // during an import review, or with another entry's form open, the view is just shown as it is.
    window.ResumeBuilder.reveal = async target => {
      await ready;
      if (disposed || !target || !importSection.hidden) return;
      const focus = input => {
        input?.scrollIntoView({block: "center"});
        input?.focus();
      };
      if (target.version) {
        const v = versions.find(x => x.id === target.version);
        if (v && showing?.id !== v.id) await toggleVersion(v);
        versionsList.querySelector('[aria-expanded="true"]')?.closest(".version")?.scrollIntoView({block: "nearest"});
        return;
      }
      const kind = target.add || target.kind;
      if (kind) {
        const id = target.add ? null : target.id;
        if (editing && !(editing.kind === kind && editing.id === id)) {
          KINDS[editing.kind].section.scrollIntoView({block: "start"});
          return;
        }
        editing = {kind, id};
        renderEntries();
        await new Promise(queueMicrotask); //the form focuses its first field in a microtask
        const formEl = KINDS[kind].section.querySelector(".entry-form");
        focus(target.field && formEl?.querySelector(`[id$="-${target.field}"]`) || formEl?.querySelector("input, textarea"));
        return;
      }
      focus(profileForm?.inputs[target.field]);
    };
 
    return () => {
      disposed = true;
      clearTimeout(statusTimer);
      pdf.dispose();
      container.replaceChildren();
    };
  }
 
  //the skill breakdown shared by the saved-job view and the tailor view
  const skillMatch = (a, titles) => {
    const group = (heading, split) => {
            const matched = [...split.tech.matched, ...split.soft.matched];
            const missing = [...split.tech.missing, ...split.soft.missing];
            // Skills the resume does not list but probably covers, because it shows a related one. They are
            // not gaps and count for half in the score, so they are shown rather than left unexplained.
            const likely = [...(split.tech.likely || []), ...(split.soft.likely || [])];
            // Soft skills the resume does not name are not held against the person: a resume
            // rarely lists them. They are shown apart, so it is clear what the posting asked for.
            const unmentioned = [...(split.tech.unmentioned || []), ...(split.soft.unmentioned || [])];
            if (!matched.length && !likely.length && !missing.length && !unmentioned.length) return null;
            //Every skill the posting asks for at a glance, colored by where it stands, then the breakdown.
            //Each chip also carries a mark, so the colors are not the only way to tell them apart.
            const skill = (kind, mark, text, title) => el("li", {class: `chip skill skill-${kind}`, title: title || null},
              el("span", {class: "skill-mark", "aria-hidden": "true", text: mark}), text);
            const evidence = m => m.experience_ids.map(i => titles[i]).filter(Boolean).join("; ");
            const overview = el("ul", {class: "chips skill-overview", "aria-label": `${heading} skills`},
              matched.map(m => skill("have", "\u2713", m.skill, evidence(m) ? `On your resume: ${evidence(m)}` : "On your resume")),
              likely.map(l => skill("likely", "~", l.skill, l.because ? `Not listed, but you list ${l.because}` : "Not listed, but a related skill is")),
              missing.map(x => skill("gap", "\u00d7", x, "Not on your resume")),
              unmentioned.map(x => skill("soft", "\u00b7", x, "A soft skill your resume does not name. It is not held against you.")));
            const count = (kind, label, n) => n ? el("span", {class: `skill-key skill-key-${kind}`, text: `${n} ${label}`}) : null;
            return el("div", {class: "skill-group"},
              el("h4", {text: heading}),
              overview,
              el("p", {class: "skill-keys"},
                count("have", "on your resume", matched.length),
                count("likely", "likely", likely.length),
                count("gap", "missing", missing.length),
                count("soft", "not mentioned", unmentioned.length)),
              el("details", {class: "skill-breakdown"}, el("summary", {text: "Details"}),
                matched.length ? [el("p", {text: "On your resume"}),
                  matched.map(m => el("p", {class: "skill-evidence"}, `\u2713 ${m.skill}`,
                    evidence(m) ? el("small", {text: ` \u2014 ${evidence(m)}`}) : null))] : null,
                likely.length ? [el("p", {text: "Likely (not listed, but related skills on your resume suggest it)"}),
                  el("ul", {class: "chips"}, likely.map(l => skill("likely", "~", l.because ? `${l.skill} (you list ${l.because})` : l.skill, "")))] : null,
                missing.length ? [el("p", {text: "Missing"}), el("ul", {class: "chips"}, missing.map(x => skill("gap", "\u00d7", x, "")))] : null,
                unmentioned.length ? [el("p", {text: "Not mentioned (soft skills a resume rarely lists, not held against you)"}),
                  el("ul", {class: "chips"}, unmentioned.map(x => skill("soft", "\u00b7", x, "")))] : null));
          };
    const noRequired = [a.required.tech, a.required.soft].every(g => !g.matched.length && !(g.likely || []).length && !g.missing.length && !(g.unmentioned || []).length);
    const none = text => el("div", {class: "skill-group"}, el("h4", {text}), el("p", {class: "hint", text: "The posting lists none."}));
    return {
      heading: noRequired ? "This posting lists no required skills" : `Skill match: ${Math.round(a.score * 100)}% of required skills`,
      groups: [group("Required", a.required) || none("Required"), group("Nice to have", a.preferred) || none("Nice to have")],
    };
  };

  window.ResumeBuilder = {skillMatch, mount, forms, el, fill, chips, makeForm, exportFile, savedNote};
})();
 
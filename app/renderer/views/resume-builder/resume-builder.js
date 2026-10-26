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
    for (const child of children.flat()) {
      if (child === null || child === undefined || child === false) continue;
      node.append(child instanceof Node ? child : document.createTextNode(String(child)));
    }
    return node;
  };
 
  const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
 
  //"2022-06-01" -> "Jun 2022", as render.Master writes it
  const formatMonth = date => {
    const m = /^(\d{4})-(\d{2})/.exec(date || "");
    return m ? `${MONTHS[Number(m[2]) - 1] || ""} ${m[1]}`.trim() : date || "";
  };
 
  const dateRange = (start, end) => {
    if (!start && !end) return "";
    if (!start) return end ? formatMonth(end) : "";
    return `${formatMonth(start)} to ${end ? formatMonth(end) : "Present"}`;
  };
 
  //A month input holds "2024-06". Stored dates are "2024-06-01"; the API accepts either.
  const toMonth = date => (/^\d{4}-\d{2}/.exec(date || "") || [""])[0];
 
  //Lists are typed as comma-separated text: "sql, excel" <-> ["sql", "excel"]
  const listToText = list => (list || []).join(", ");
  const textToList = text => text.split(",").map(s => s.trim()).filter(Boolean);
 
  //replaceChildren with el()'s rules: arrays are flattened, and null, undefined and false are skipped
  const fill = (node, ...children) => node.replaceChildren(...children.flat().filter(c => c !== null && c !== undefined && c !== false));
 
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
      } else if (field.type === "month") {
        input = el("input", {id, type: "month", value: toMonth(values[field.name])});
        if (field.current) {
          const box = el("input", {type: "checkbox", checked: Boolean(values[field.current.when]) && !values[field.name]});
          const sync = () => {
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
      return el("div", {class: `form-row${field.wide ? " wide" : ""}`},
        el("label", {for: id}, field.label, field.required ? el("span", {class: "required", "aria-hidden": "true", text: " *"}) : null),
        input,
        extra,
        field.hint ? el("p", {class: "hint", text: field.hint}) : null,
        errors[field.name]);
    });
 
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
        for (const field of spec) {
          const input = inputs[field.name];
          const raw = input.value.trim();
          if (field.type === "list") body[field.name] = textToList(input.value);
          else if (field.type === "number") body[field.name] = raw === "" ? null : Number(raw);
          else if (field.type === "month") body[field.name] = input.currentBox?.checked || raw === "" ? null : raw;
          else if (field.required) body[field.name] = raw;
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
    {name: "full_name", label: "Full name", required: true},
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
 
    const saveVersionButton = el("button", {type: "button", class: "primary"}, "Save as version");
    const toolbar = el("header", {class: "toolbar drag"},
      el("div", {class: "toolbar-form resume-toolbar"},
        el("h1", {class: "view-title", text: "Master resume"}),
        status,
        el("div", {class: "toolbar-actions"}, saveVersionButton)));
 
    const profileSection = el("section", {class: "resume-section", "aria-labelledby": "resume-profile-h"});
    const experienceSection = el("section", {class: "resume-section", "aria-labelledby": "resume-exp-h"});
    const educationSection = el("section", {class: "resume-section", "aria-labelledby": "resume-edu-h"});
    const editor = el("div", {class: "pane resume-editor"}, profileSection, experienceSection, educationSection);
 
    const previewText = el("pre", {class: "preview-text"});
    const versionsList = el("ul", {class: "versions"});
    const versionForm = el("form", {class: "version-form", hidden: true});
    const side = el("div", {class: "pane resume-side"},
      el("section", {class: "resume-section"},
        el("h2", {text: "Preview"}),
        el("p", {class: "hint", text: "This text is what saved versions, tailoring and PDF export start from."}),
        previewText),
      el("section", {class: "resume-section"},
        el("h2", {text: "Saved versions"}),
        versionForm,
        versionsList));
 
    container.replaceChildren(toolbar, el("div", {class: "content resume-content"}, editor, side));
 
    // --- Preview ---
 
    const refreshPreview = () => attempt(async () => {
      const {markdown} = await api().resume.preview();
      if (disposed) return;
      previewText.textContent = markdown.trim() ? markdown : "Your resume appears here once you save your profile.";
    });
 
    // --- Profile ---
 
    function renderProfile() {
      const form = forms.profile(master.profile);
      const save = el("button", {type: "submit", class: "primary"}, master.profile ? "Save profile" : "Create profile");
      const formEl = el("form", {class: "resume-form", novalidate: true},
        form.el, el("div", {class: "form-actions"}, save));
      formEl.addEventListener("submit", async event => {
        event.preventDefault();
        save.disabled = true;
        const created = !master.profile;
        const profile = await attempt(() => api().resume.putProfile(form.read()), err => form.showError(err));
        save.disabled = false;
        if (!profile || disposed) return;
        master.profile = profile;
        form.clearErrors();
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
        meta: e => [dateRange(e.start_date, e.end_date), e.location].filter(Boolean).join(" · "),
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
      for (const f of KINDS[kind].fields) body[f.name] = entry[f.name] ?? (f.type === "list" ? [] : null);
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
      if (!window.confirm(`Delete "${k.title(entry)}"? This cannot be undone.`)) return;
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
      if (!window.confirm(`Delete the saved version "${v.version_label}"? This cannot be undone.`)) return;
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
 
    // --- Load ---
 
    (async () => {
      const loaded = await attempt(() => api().resume.get());
      if (disposed) return;
      if (loaded) master = loaded;
      renderProfile();
      renderEntries();
      refreshPreview();
      loadVersions();
    })();
 
    return () => {
      disposed = true;
      clearTimeout(statusTimer);
      container.replaceChildren();
    };
  }
 
  window.ResumeBuilder = {mount, forms, el, fill, chips};
})();
 
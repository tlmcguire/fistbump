// Report an issue, for people who have never opened GitHub. One short form: what kind, a few words, what happened.
// While they type, open reports that look the same are shown, and "That's mine" adds to one instead of filing
// another. Before anything is sent they can read exactly what will go, and nothing personal goes: no name, no email,
// no resume, no jobs. The relay (relay/) files it on GitHub, or adds it to the report it duplicates.
//
//   window.Report.open()                                  from Help > Report an Issue… or Settings
//   window.Report.open({title, details, kind: "bug"})     prefilled, from an error panel
window.Report = (() => {
  const api = () => window.FistBumpAPI;
  const KINDS = [["bug", "Something is broken"], ["idea", "An idea"], ["question", "A question"]];
  const PLACEHOLDER = {
    bug: "What did you do, and what happened instead? For example: I picked a date in the Tracker and it went back to empty.",
    idea: "What would make Fistbump better for you? For example: let me sort the Tracker by company.",
    question: "What would you like to know?",
  };
  const OS = {darwin: "macOS", win32: "Windows", linux: "Linux"};

  const make = (tag, props = {}, ...kids) => {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(props)) {
      if (v === null || v === undefined || v === false) continue;
      if (k === "class") node.className = v;
      else if (k === "text") node.textContent = v;
      else if (k.startsWith("on")) node.addEventListener(k.slice(2), v);
      else node.setAttribute(k, v === true ? "" : v);
    }
    node.append(...kids.flat().filter(k => k !== null && k !== undefined && k !== false));
    return node;
  };

  //the same scrubbing the relay does, so what is shown is what is sent
  const scrub = (text, max) => String(text ?? "")
    .replace(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/gi, "[email removed]")
    .replace(/\+?\d[\d\s().-]{8,}\d/g, "[number removed]")
    .replace(/(\/Users\/|\/home\/|C:\\Users\\)[^\s/\\]+/gi, "$1[name]")
    .trim()
    .slice(0, max);

  let version = "";
  const where = () => ({
    version,
    os: OS[window.api?.platform] || window.api?.platform || "",
    view: document.querySelector(".app")?.dataset.route || "",
  });

  function open(prefill = {}) {
    document.querySelector("dialog.report-dialog")?.remove();
    api().request?.(() => window.api.app.info()).then(info => { version = info?.version || ""; }).catch(() => {});
    let kind = KINDS.some(([k]) => k === prefill.kind) ? prefill.kind : "bug";
    let sameAs = null;
    let similarTimer = null;
    let asked = "";

    const dialog = make("dialog", {class: "ask report-dialog", "aria-labelledby": "report-title"});
    const kindButtons = KINDS.map(([value, label]) => make("button", {
      type: "button", role: "radio", "data-kind": value, "aria-checked": String(value === kind),
      onclick: () => {
        kind = value;
        kindButtons.forEach(b => b.setAttribute("aria-checked", String(b.dataset.kind === kind)));
        details.placeholder = PLACEHOLDER[kind];
        detailsLabel.textContent = kind === "bug" ? "What happened?" : "Tell us more";
        preview();
      },
    }, label));
    const title = make("input", {id: "report-what", type: "text", maxlength: "120", autocomplete: "off", placeholder: "In a few words"});
    title.value = prefill.title || "";
    const details = make("textarea", {id: "report-details", rows: "5", maxlength: "4000", placeholder: PLACEHOLDER[kind]});
    details.value = prefill.details || "";
    const detailsLabel = make("span", {text: kind === "bug" ? "What happened?" : "Tell us more"});
    const steps = make("textarea", {id: "report-steps", rows: "3", maxlength: "2000", placeholder: "1. Open the Tracker\n2. Pick a date\n3. …"});
    const alike = make("section", {class: "report-alike", "aria-live": "polite", hidden: true});
    const sent = make("pre", {class: "report-preview", translate: "no"});
    const status = make("p", {class: "report-status", role: "status"});
    const send = make("button", {type: "button", class: "primary"}, "Send report");
    const cancel = make("button", {type: "button"}, "Cancel");
    const close = () => { clearTimeout(similarTimer); dialog.close(); dialog.remove(); };
    cancel.addEventListener("click", close);

    const body = () => ({kind, title: scrub(title.value, 120), details: scrub(details.value, 4000), steps: scrub(steps.value, 2000), app: where(), ...(sameAs ? {same_as: sameAs.number} : {})});
    function preview() {
      const b = body();
      sent.textContent = [`${KINDS.find(([k]) => k === b.kind)[1]}: ${b.title || "…"}`, "", b.details || "…", b.steps ? `\nHow to make it happen:\n${b.steps}` : null,
        `\nFistbump ${b.app.version || ""} on ${b.app.os}${b.app.view ? `, ${b.app.view} page` : ""}`].filter(x => x !== null).join("\n");
    }

    //open reports that read the same, so a person can add to one instead of filing another
    const drawAlike = list => {
      alike.hidden = !list.length && !sameAs;
      alike.replaceChildren(...[
        make("h3", {text: sameAs ? "Adding to this report" : "Is it one of these?"}),
        make("ul", {}, (sameAs ? [sameAs] : list).map(r => make("li", {},
          make("span", {class: "report-alike-words"}, make("b", {text: r.title}), make("span", {text: `#${r.number}${r.comments ? ` · ${r.comments === 1 ? "1 person added to it" : `${r.comments} people added to it`}` : ""}`})),
          sameAs
            ? make("button", {type: "button", onclick: () => { sameAs = null; send.textContent = "Send report"; drawAlike(list); preview(); }}, "No, it's new")
            : make("button", {type: "button", onclick: () => { sameAs = r; send.textContent = "Add to this report"; drawAlike(list); preview(); }}, "That's mine")))),
        sameAs ? make("p", {class: "report-note", text: "Your words are added to it, so the people working on it see how many it affects."}) : null].filter(Boolean));
    };
    const lookAlike = () => {
      clearTimeout(similarTimer);
      similarTimer = setTimeout(async () => {
        const words = `${title.value} ${details.value.slice(0, 120)}`.trim();
        if (words.split(/\s+/).length < 3 || words === asked) return;
        asked = words;
        try {
          const res = await api().community.similar(words);
          drawAlike(res?.reports || []);
        } catch { /* looking is a help, not a step: the report can still be sent */ }
      }, 600);
    };
    title.addEventListener("input", () => { preview(); lookAlike(); });
    details.addEventListener("input", () => { preview(); lookAlike(); });
    steps.addEventListener("input", preview);

    //if the service cannot be reached, the report can still be copied, or filed on GitHub by someone with an account
    const fallback = () => {
      const b = body();
      const url = `https://github.com/tlmcguire/fistbump/issues/new?${new URLSearchParams({title: b.title, body: sent.textContent, labels: b.kind})}`;
      return make("div", {class: "report-fallback"},
        make("button", {type: "button", onclick: async () => { try { await navigator.clipboard.writeText(sent.textContent); status.textContent = "Copied. You can paste it into an email or a message."; } catch {} }}, "Copy report"),
        make("button", {type: "button", onclick: () => window.open(url, "_blank")}, "Open on GitHub"));
    };

    send.addEventListener("click", async () => {
      const b = body();
      if (b.title.length < 4) { status.textContent = "Give it a short title, a few words is enough."; title.focus(); return; }
      if (b.details.length < 8 && !sameAs) { status.textContent = "Say a little about what happened."; details.focus(); return; }
      send.disabled = true;
      send.setAttribute("aria-busy", "true");
      status.textContent = sameAs ? "Adding your note…" : "Sending…";
      try {
        const res = await api().community.report(b);
        const added = res?.status === "added";
        dialog.replaceChildren(
          make("div", {class: "report-done"},
            window.UI?.pico?.("07-results-found.png", "mascot pico") || null,
            make("h2", {id: "report-title", text: added ? "Added to an open report" : "Thank you, it's filed"}),
            make("p", {text: added
              ? "Someone already reported this, so your note was added to theirs. That tells the people fixing it how many it affects."
              : "Your report is in. The people who make Fistbump read every one."}),
            res?.url ? make("p", {}, make("a", {href: res.url, target: "_blank", rel: "noopener noreferrer", text: `See report #${res.number}`})) : null,
            make("div", {class: "ask-actions"}, make("button", {type: "button", class: "primary", onclick: close}, "Done"))));
      } catch (err) {
        send.disabled = false;
        send.removeAttribute("aria-busy");
        status.textContent = err.message;
        status.after(fallback());
      }
    });

    dialog.append(
      make("h2", {id: "report-title", text: "Report an issue"}),
      make("p", {class: "report-lede", text: "Tell us what went wrong or what you'd like. No account needed."}),
      make("div", {class: "st3-seg report-kinds", role: "radiogroup", "aria-label": "Kind of report"}, kindButtons),
      make("label", {class: "report-field", for: "report-what"}, make("span", {text: "Title"}), title),
      alike,
      make("label", {class: "report-field", for: "report-details"}, detailsLabel, details),
      make("details", {class: "report-more"}, make("summary", {text: "How to make it happen (optional)"}), steps),
      make("details", {class: "report-more"}, make("summary", {text: "What will be sent"}),
        make("p", {class: "report-note", text: "Only this. Not your name, email, resume or jobs. Email addresses and phone numbers are removed."}), sent),
      status,
      make("div", {class: "ask-actions"}, cancel, send));
    window.UI?.wireDialogClose?.(dialog, close);
    dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
    document.body.append(dialog);
    dialog.showModal();
    preview();
    if (prefill.title) lookAlike();
    (title.value ? details : title).focus();
  }

  return {open};
})();

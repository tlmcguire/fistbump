// Add / submit a job listing (modeled after Report an issue):
//  - If the link is from Greenhouse, Lever, or Ashby, Fistbump fetches and parses the posting automatically —
//    no description or manual fields needed.
//  - For any other site (LinkedIn, Indeed, Workday, Glassdoor, company career pages, etc.), the user
//    enters the title, company, and pastes the listing content.
//  - Users can save the listing to their own jobs and/or share it with the Fistbump community (scrubbed,
//    no account needed, appears in Search under Community listings once reviewed).
//
//   window.SubmitListing.open()
//   window.SubmitListing.open({listing_url, company_name, position_title, raw_text})
window.SubmitListing = (() => {
  const WORK_MODES = [["", "Not stated"], ["Remote", "Remote"], ["Hybrid", "Hybrid"], ["On-site", "On-site"]];

  const make = (tag, props = {}, ...kids) => {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(props)) {
      if (v === null || v === undefined || v === false) continue;
      if (k === "class") node.className = v;
      else if (k === "text") node.textContent = v;
      else if (k.startsWith("on")) node.addEventListener(k.slice(2), v);
      else if (k === "checked" || k === "hidden" || k === "disabled") node[k] = Boolean(v);
      else node.setAttribute(k, v === true ? "" : v);
    }
    node.append(...kids.flat().filter(k => k !== null && k !== undefined && k !== false));
    return node;
  };

  // Same scrubbing the relay performs, so the preview matches what is sent.
  const scrub = (text, max) => String(text ?? "")
    .replace(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/gi, "[email removed]")
    .replace(/\+?\d[\d\s().-]{8,}\d/g, "[number removed]")
    .replace(/(\/Users\/|\/home\/|C:\\Users\\)[^\s/\\]+/gi, "$1[name]")
    .trim()
    .slice(0, max);

  const normalizeUrl = raw => {
    const t = String(raw || "").trim();
    if (!t) return "";
    try {
      const u = new URL(/^https?:\/\//i.test(t) ? t : `https://${t}`);
      return u.protocol === "https:" || u.protocol === "http:" ? u.href : "";
    } catch {
      return "";
    }
  };

  const isDirectATSLink = raw => {
    const t = String(raw || "").trim();
    if (!t || /\s/.test(t)) return false;
    return /^(?:https?:\/\/)?(?:(?:job-)?boards\.greenhouse\.io\/|grnh\.se\/|jobs\.(?:eu\.)?lever\.co\/|jobs\.ashbyhq\.com\/)/i.test(t);
  };

  const siteFromUrl = value => {
    if (!value) return "";
    try {
      const host = new URL(value).hostname.replace(/^www\./i, "").toLowerCase();
      if (host.includes("greenhouse.io") || host === "grnh.se") return "Greenhouse";
      if (host.includes("lever.co")) return "Lever";
      if (host.includes("ashbyhq.com")) return "Ashby";
      if (host === "linkedin.com" || host.endsWith(".linkedin.com")) return "LinkedIn";
      if (host === "indeed.com" || host.endsWith(".indeed.com")) return "Indeed";
      if (host === "glassdoor.com" || host.endsWith(".glassdoor.com")) return "Glassdoor";
      if (host.includes("workday")) return "Workday";
      if (host === "wellfound.com" || host === "angel.co") return "Wellfound";
      if (host === "ziprecruiter.com" || host.endsWith(".ziprecruiter.com")) return "ZipRecruiter";
      if (host.includes("smartrecruiters")) return "SmartRecruiters";
      if (host.includes("icims")) return "iCIMS";
      if (host.includes("jobvite")) return "Jobvite";
      return host;
    } catch {
      return "";
    }
  };

  function open(prefill = {}) {
    document.querySelector("dialog.submit-listing-dialog")?.remove();

    const dialog = make("dialog", {class: "ask report-dialog submit-listing-dialog", "aria-labelledby": "submit-listing-title"});
    const listingUrl = make("input", {id: "sl-url", type: "url", maxlength: "500", autocomplete: "off", placeholder: "https://www.linkedin.com/jobs/view/... or Greenhouse / Lever / Ashby link"});
    listingUrl.value = prefill.listing_url || "";
    const atsNote = make("p", {class: "sl-ats-note", role: "status", hidden: true});

    const title = make("input", {id: "sl-title", type: "text", maxlength: "160", autocomplete: "off", placeholder: "e.g. Senior Software Engineer"});
    title.value = prefill.position_title || "";
    const company = make("input", {id: "sl-company", type: "text", maxlength: "120", autocomplete: "off", placeholder: "e.g. Acme Corp"});
    company.value = prefill.company_name || "";
    const location = make("input", {id: "sl-location", type: "text", maxlength: "120", autocomplete: "off", placeholder: "e.g. Austin, TX or Remote - US"});
    location.value = prefill.location || "";
    const mode = make("select", {id: "sl-mode"}, WORK_MODES.map(([v, label]) => make("option", {value: v, text: label})));
    mode.value = prefill.work_mode || "";
    const companyUrl = make("input", {id: "sl-company-url", type: "url", maxlength: "300", autocomplete: "off", placeholder: "https://company.com (optional)"});
    companyUrl.value = prefill.company_url || "";
    const description = make("textarea", {id: "sl-text", rows: "7", maxlength: "25000", placeholder: "Copy and paste the job description, requirements, pay, and benefits here…"});
    description.value = prefill.raw_text || "";

    const manualFields = make("div", {class: "sl-manual"},
      make("div", {class: "sl-grid"},
        make("label", {class: "report-field", for: "sl-title"}, make("span", {text: "Position title *"}), title),
        make("label", {class: "report-field", for: "sl-company"}, make("span", {text: "Company *"}), company),
        make("label", {class: "report-field", for: "sl-location"}, make("span", {text: "Location"}), location),
        make("label", {class: "report-field", for: "sl-mode"}, make("span", {text: "Work mode"}), mode)),
      make("label", {class: "report-field", for: "sl-text"}, make("span", {text: "Listing description & requirements *"}), description),
      make("details", {class: "report-more"}, make("summary", {text: "Company website (optional)"}),
        make("label", {class: "report-field", for: "sl-company-url"}, companyUrl)));

    const saveLocal = make("input", {id: "sl-save-local", type: "checkbox", checked: true});
    const shareCommunity = make("input", {id: "sl-share-community", type: "checkbox", checked: true});
    const sent = make("pre", {class: "report-preview", translate: "no"});
    const previewWrap = make("details", {class: "report-more"}, make("summary", {text: "What will be shared with the community"}),
      make("p", {class: "report-note", text: "Only this listing. Not your name, email, resume or saved jobs. Email addresses and phone numbers are removed."}), sent);
    const status = make("p", {class: "report-status", role: "status"});
    const send = make("button", {type: "button", class: "primary"}, "Add listing");
    const cancel = make("button", {type: "button"}, "Cancel");
    const close = () => { dialog.close(); dialog.remove(); };
    cancel.addEventListener("click", close);

    const payload = () => {
      const url = normalizeUrl(listingUrl.value);
      const coUrl = normalizeUrl(companyUrl.value);
      const source = siteFromUrl(url) || "Community";
      return {
        source,
        company_name: scrub(company.value, 120),
        position_title: scrub(title.value, 160),
        location: scrub(location.value, 120),
        work_mode: mode.value,
        listing_url: url,
        company_url: coUrl && coUrl.startsWith("https://") ? coUrl : "",
        raw_text: scrub(description.value, 25000),
      };
    };

    const syncMode = () => {
      const direct = isDirectATSLink(listingUrl.value);
      const url = normalizeUrl(listingUrl.value);
      const site = siteFromUrl(url);
      manualFields.hidden = direct;
      atsNote.hidden = !direct;
      if (direct) {
        atsNote.textContent = `${site || "Supported ATS"} link detected — Fistbump will fetch the title, company, and full description automatically. No need to paste content.`;
      }
      previewWrap.hidden = !shareCommunity.checked;
      preview();
    };

    const preview = () => {
      const direct = isDirectATSLink(listingUrl.value);
      const p = payload();
      if (direct) {
        sent.textContent = `Board from ${p.source}: ${p.listing_url || listingUrl.value.trim()}\n(Fistbump will fetch the posting automatically and share the company's public ATS board if community sharing is checked.)`;
        return;
      }
      const lines = [
        `${p.position_title || "…"} at ${p.company_name || "…"}`,
        [p.source, p.location, p.work_mode].filter(Boolean).join(" · "),
        p.listing_url ? `Link: ${p.listing_url}` : null,
        p.company_url ? `Company site: ${p.company_url}` : null,
        "",
        p.raw_text ? (p.raw_text.length > 600 ? `${p.raw_text.slice(0, 600)}…` : p.raw_text) : "…",
      ].filter(x => x !== null);
      sent.textContent = lines.join("\n");
    };

    listingUrl.addEventListener("input", syncMode);
    shareCommunity.addEventListener("change", syncMode);
    for (const field of [title, company, location, mode, companyUrl, description]) {
      field.addEventListener("input", preview);
      field.addEventListener("change", preview);
    }

    send.addEventListener("click", async () => {
      status.textContent = "";
      if (!saveLocal.checked && !shareCommunity.checked) {
        status.textContent = "Choose to save the listing to your jobs, share it with the community, or both.";
        return;
      }

      const direct = isDirectATSLink(listingUrl.value);
      const normalized = normalizeUrl(listingUrl.value);

      if (direct) {
        if (!normalized) {
          status.textContent = "Enter a valid Greenhouse, Lever, or Ashby job link.";
          listingUrl.focus();
          return;
        }
        send.disabled = true;
        send.setAttribute("aria-busy", "true");
        status.textContent = "Fetching listing…";
        try {
          const scraped = await window.FistBumpAPI.link(normalized);
          let savedJob = null;
          if (saveLocal.checked && scraped?.board && scraped?.external_id) {
            const {imported, refreshed} = await window.FistBumpAPI.importJobs([{
              source: scraped.source || "greenhouse",
              board: scraped.board,
              external_id: scraped.external_id,
            }]);
            savedJob = imported?.[0] || refreshed?.[0] || null;
            window.dispatchEvent(new CustomEvent("fistbump:jobs", {detail: {}}));
          }
          if (shareCommunity.checked && scraped?.board && window.FistBumpAPI?.community?.shareBoards) {
            const boardKey = `${(scraped.source || "greenhouse").toLowerCase()}:${scraped.board}`;
            await window.FistBumpAPI.community.shareBoards({boards: [boardKey]}).catch(() => {});
          }
          dialog.replaceChildren(
            make("div", {class: "report-done"},
              window.UI?.pico?.("07-results-found.png", "mascot pico") || null,
              make("h2", {id: "submit-listing-title", text: "Listing added"}),
              make("p", {text: `${scraped.position_title || "Job"} at ${scraped.company_name || "company"} was fetched automatically${shareCommunity.checked ? " and its board was shared with the community" : ""}.`}),
              make("div", {class: "ask-actions"},
                savedJob ? make("button", {type: "button", onclick: () => { close(); window.JobPage?.open?.(savedJob.id); }}, "Open job") : null,
                make("button", {type: "button", class: "primary", onclick: close}, "Done"))));
        } catch (err) {
          send.disabled = false;
          send.removeAttribute("aria-busy");
          status.textContent = err.message || "Could not fetch that listing automatically.";
        }
        return;
      }

      // Non-ATS site (LinkedIn, Indeed, Workday, etc.): manual title, company, and content required.
      const p = payload();
      if (listingUrl.value.trim() && !p.listing_url) {
        status.textContent = "Enter a valid http or https link for the listing.";
        listingUrl.focus();
        return;
      }
      if (p.position_title.length < 2) {
        status.textContent = "Enter the job title.";
        title.focus();
        return;
      }
      if (p.company_name.length < 2) {
        status.textContent = "Enter the company name.";
        company.focus();
        return;
      }
      if (p.raw_text.length < 40) {
        status.textContent = "Paste the job description and requirements (at least a few lines) — Fistbump cannot fetch this site automatically.";
        description.focus();
        return;
      }

      send.disabled = true;
      send.setAttribute("aria-busy", "true");
      status.textContent = shareCommunity.checked ? "Submitting…" : "Saving…";
      try {
        let savedJob = null;
        if (saveLocal.checked && window.FistBumpAPI?.jobs?.create) {
          const localBody = {
            company_name: p.company_name,
            position_title: p.position_title,
            source: "pasted",
            raw_text: description.value,
          };
          if (p.location) localBody.location = p.location;
          if (p.work_mode) localBody.work_mode = p.work_mode;
          if (p.listing_url) localBody.listing_url = p.listing_url;
          if (p.company_url) localBody.company_url = p.company_url;
          savedJob = await window.FistBumpAPI.jobs.create(localBody);
          window.dispatchEvent(new CustomEvent("fistbump:jobs", {detail: {}}));
        }
        let shareRes = null;
        if (shareCommunity.checked && window.Community?.shareListing) {
          shareRes = await window.Community.shareListing(p);
        }
        const known = shareRes?.status === "known";
        dialog.replaceChildren(
          make("div", {class: "report-done"},
            window.UI?.pico?.("07-results-found.png", "mascot pico") || null,
            make("h2", {id: "submit-listing-title", text: shareCommunity.checked ? (known ? "Saved — already in community" : "Listing submitted") : "Listing saved"}),
            make("p", {text: shareCommunity.checked
              ? (known
                ? "Someone already shared this listing. It shows up in Search when Community listings are enabled."
                : "Your listing is in the review queue for Search → Community listings, and you can already find it in your own searches.")
              : "Saved to your jobs with full skill and pay analysis."}),
            make("div", {class: "ask-actions"},
              savedJob ? make("button", {type: "button", onclick: () => { close(); window.JobPage?.open?.(savedJob.id); }}, "Open job") : null,
              make("button", {type: "button", class: "primary", onclick: close}, "Done"))));
      } catch (err) {
        send.disabled = false;
        send.removeAttribute("aria-busy");
        status.textContent = err.message || "Could not submit the listing right now.";
      }
    });

    dialog.append(
      make("h2", {id: "submit-listing-title", text: "Add a job listing"}),
      make("p", {class: "report-lede", text: "Paste a Greenhouse, Lever, or Ashby link to fetch it automatically — or enter a LinkedIn, Indeed, Workday, or other listing with its content to save and share in Search."}),
      make("label", {class: "report-field", for: "sl-url"}, make("span", {text: "Listing link"}), listingUrl),
      atsNote,
      manualFields,
      make("div", {class: "sl-checks"},
        make("label", {class: "sl-local-check", for: "sl-save-local"}, saveLocal, "Save to my jobs"),
        make("label", {class: "sl-local-check", for: "sl-share-community"}, shareCommunity, "Share with the Fistbump community")),
      previewWrap,
      status,
      make("div", {class: "ask-actions"}, cancel, send));

    window.UI?.wireDialogClose?.(dialog, close);
    dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
    document.body.append(dialog);
    dialog.showModal();
    syncMode();
    (listingUrl.value && !isDirectATSLink(listingUrl.value) ? title : listingUrl).focus();
  }

  return {open, isDirectATSLink};
})();

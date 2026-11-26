window.JobParser = {
    mount() {
      const $ = id => document.getElementById(id);
      const main = $("main");
      const searchForm = $("search-form");
      const searchButton = searchForm.querySelector('button[type="submit"]');

      const tabs = {find: $("tab-find"), paste: $("tab-paste"), saved: $("tab-saved"), settings: $("tab-settings")};
      const forms = {find: searchForm, paste: $("paste-title"), saved: $("saved-form"), settings: $("settings-title")};

      // Detects a direct-fetch ATS link (Greenhouse, Lever, Ashby) vs any external job link (LinkedIn, Indeed, etc.).
      const isDirectATSLink = value => {
        const t = String(value || "").trim();
        if (!t || /\s/.test(t)) return false;
        return /^(?:https?:\/\/)?(?:(?:job-)?boards\.greenhouse\.io\/|grnh\.se\/|jobs\.(?:eu\.)?lever\.co\/|jobs\.ashbyhq\.com\/)/i.test(t);
      };
      const looksLikeJobLink = value => {
        const t = String(value || "").trim();
        if (!t || /\s/.test(t)) return false;
        return /^https?:\/\//i.test(t) || isDirectATSLink(t)
          || /^(?:www\.)?(?:linkedin\.com|indeed\.com|glassdoor\.com|wellfound\.com|ziprecruiter\.com)\//i.test(t);
      };
      const normalizeJobLink = value => {
        const t = String(value || "").trim();
        return /^https?:\/\//i.test(t) ? t : `https://${t}`;
      };
      const siteNameFromURL = value => {
        const t = String(value || "").trim();
        if (!t) return "";
        try {
          const host = new URL(normalizeJobLink(t)).hostname.replace(/^www\./i, "").toLowerCase();
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
      const syncSearchButton = () => {
        const val = $("search-keywords").value;
        searchButton.textContent = looksLikeJobLink(val) ? (isDirectATSLink(val) ? "Open link" : "Add listing") : "Search";
      };

      //The results pane and the detail pane each show one state at a time.
      const RESULT_STATES = ["state-initial", "state-loading", "state-results", "state-error"];
      const DETAIL_STATES = ["detail-empty", "detail-loading", "state-success", "detail-error"];

      let posting = null;        //the listing in the detail pane
      let selectedKey = "";      //its board/id, to highlight it in the result list
      let fromResults = false;   //true while the open listing came from the result list
      let searchRetry = null;    //what Try again does in the results pane
      let detailRetry = null;    //and in the detail pane
      let detailSeq = 0;         //counts what the detail pane was given, so a slow load cannot land over a newer one
      let activeTab = "find";
      let trackedApp = null; //the open saved job's application, or null (phase 7)
      let lifecycleSeq = 0;
      let lastJobsTab = "find"; //the last of Find jobs and Open a link that was shown
      let results = null;
      let searchedQuery = {};
      const MAX_OFFSET = 5000; //the backend's offset limit
      //results a page: the setting at the time of the search, so changing it does not break paging
      const pageSize = () => searchedQuery.limit || window.Prefs.get("pageSize");
      let pages = new Map(); //page number -> the backend's response for it
      let currentPage = 1;

      const showState = state => {
        RESULT_STATES.forEach(id => { $(id).hidden = id !== state; });
        poseDetailMascot();
      };

      //The mascot in the empty detail pane: it holds out a job card beside a list of results.
      const poseDetailMascot = () => {
        const pose = !$("state-results").hidden && results?.postings.length ? "07-results-found" : "";
        $("detail-mascot").hidden = !pose;
        if (pose) $("detail-mascot").src = `views/graphics/mascots/${pose}.png`;
      };
      const showDetail = state => DETAIL_STATES.forEach(id => { $(id).hidden = id !== state; });

      //shows the results pane's loading message
      const showLoading = (message, cancelable = false) => {
        $("loading-message").textContent = message;
        $("cancel-search").hidden = !cancelable;
        showState("state-loading");
      };

      //The mascot sets out to search, or puts the magnifier down when a search was canceled.
      const showInitial = (message, pose = "05-searching-wide") => {
        $("initial-message").textContent = message;
        $("initial-mascot").src = `views/graphics/mascots/${pose}.png`;
        showState("state-initial");
      };

      //the placeholder in the detail pane, worded for the view it is in
      const showDetailEmpty = () => {
        $("detail-empty-message").textContent = "Choose a job from the list, or paste a Greenhouse, Lever, or Ashby job link into the search bar.";
        detailSeq++;
        posting = null;
        selectedKey = "";
        markSelected();
        main.classList.remove("has-detail");
        poseDetailMascot();
        showDetail("detail-empty");
      };

      const setSearchBusy = busy => { searchButton.disabled = busy; };

      //The listing each of Find jobs and Open a link had open, and how far each tab's panes were
      //scrolled, so switching back shows them as they were.
      const openListings = {};
      const scrolls = {};
      const listPane = main.querySelector(".results-pane");
      const detailPane = $("detail-pane");

      //switches between Find jobs and Open a link. Each keeps its own results and listing on screen.
      const selectTab = name => {
        if (name !== activeTab && !main.hidden) window.UI.play(main, "view-enter", 320); //same arrival as a page change
        //Find jobs and Open a link each show their own listing; Settings leaves the open one as it was.
        if (activeTab !== "settings") scrolls[activeTab] = {list: listPane.scrollTop, detail: detailPane.scrollTop};
        activeTab = name;
        if (name !== "settings" && name !== lastJobsTab) {
          openListings[lastJobsTab] = posting && !$("state-success").hidden
            ? {posting, fromResults, shown: main.classList.contains("has-detail")}
            : null;
          lastJobsTab = name;
          const saved = openListings[name];
          if (saved) {
            displayPosting(saved.posting, saved.fromResults);
            main.classList.toggle("has-detail", saved.shown);
          } else {
            showDetailEmpty();
          }
        }
        for (const key in tabs) {
          tabs[key].setAttribute("aria-selected", String(key === name));
          forms[key].hidden = key !== name;
        }

        main.classList.toggle("view-find", name === "find" || name === "saved");
        main.classList.toggle("view-saved", name === "saved"); //v3: saved jobs open on the Job page, so the list takes the width
        main.classList.toggle("view-paste", name === "paste");
        main.classList.toggle("view-settings", name === "settings");
        $("settings-pane").hidden = name !== "settings";
        $("paste-pane").hidden = name !== "paste";
        $("results-pane").hidden = name === "saved";
        $("saved-pane").hidden = name !== "saved";
        setFiltersOpen(false);
        setHistoryOpen(false);

        //The panes can only be scrolled once they are showing again.
        if (scrolls[name]) {
          listPane.scrollTop = scrolls[name].list;
          detailPane.scrollTop = scrolls[name].detail;
        }
        if (name === "settings") {
          window.Settings?.refresh();
          return;
        }

        if (name === "saved") loadSaved();

        if (name === "find" && !results && $("state-results").hidden && $("state-loading").hidden) {
          showInitial("Search by job title, location, company or industry — or paste a Greenhouse, Lever, or Ashby job link directly into the search bar.");
        }
        ({find: $("search-keywords"), paste: $("paste-text"), saved: $("saved-q")})[name]?.focus();
      };

      //Puts a search error in the results pane.
      const showError = error => {
        $("error-message").textContent = error.message || "An unknown error has occurred";
        showState("state-error");
      };

      //Puts a listing error in the detail pane.
      let failedLinkUrl = "";
      const showDetailError = (error, linkUrl = "") => {
        failedLinkUrl = linkUrl || "";
        $("detail-error-message").textContent = error.message || "An unknown error has occurred";
        const pasteBtn = $("detail-paste-instead");
        if (pasteBtn) pasteBtn.hidden = !failedLinkUrl;
        main.classList.add("has-detail");
        showDetail("detail-error");
      };

      const keyOf = result => result._community ? `community/${result.community_id}` : result.id ? `job/${result.id}` : `${result.board}/${result.external_id}`;

      //Saved jobs by their Greenhouse listing, so a listing shown again still reads as saved. Kept for every
      //saved job, archived ones included, and read when the view starts so search results know from the first
      //search which listings are already saved.
      const savedByListing = new Map();
      const loadSavedIndex = async () => {
        try {
          const jobs = await window.FistBumpAPI.jobs.list("", {archived: "include"});
          savedByListing.clear();
          for (const job of jobs) if (job.board && job.external_id) savedByListing.set(`${job.board}/${job.external_id}`, job.id);
        } catch {
          return; //the lists still work; a listing just reads as unsaved until the next try
        }
        markSavedRows();
        //the listing on screen may have been saved before the index arrived
        const open = posting;
        const id = open && !open.id && open.board && savedByListing.get(`${open.board}/${open.external_id}`);
        if (id) displayPosting({...open, id}, fromResults);
      };

      //a small "Saved" tag on the result rows that are already saved jobs
      const markSavedRows = () => {
        for (const row of document.querySelectorAll("#result-list .result")) {
          const body = row.querySelector(".result-body");
          const saved = savedByListing.has(row.dataset.key);
          const tag = body.querySelector(".result-saved");
          if (saved && !tag) {
            const small = document.createElement("small");
            small.className = "result-state result-saved";
            small.textContent = "Saved";
            body.append(small);
          } else if (!saved && tag) {
            tag.remove();
          }
        }
      };

      //highlights the open listing in the result list, if it is on this page
      const markSelected = () => {
        for (const row of document.querySelectorAll(".result")) {
          const selected = row.dataset.key === selectedKey;
          row.classList.toggle("selected", selected);
          row.querySelector(".result-open")?.setAttribute("aria-current", selected ? "true" : "false");
        }
      };

      //only allows https links
      const safeURL = value => {
        try {
          const url = new URL(value);
          return url.protocol === "https:" ? url.href : null;
        } catch {
          return null;
        }
      };
  
      //Sets a link. Without a usable address it is hidden, or with dim it stays in place, dimmed and unclickable.
      const setLink = (id, value, {dim = false} = {}) => {
        const element = document.getElementById(id);
        const url = value ? safeURL(value) : null;
        element.hidden = !url && !dim;
        element.toggleAttribute("aria-disabled", !url);
        element.tabIndex = url ? 0 : -1;

        if (url) {
          element.href = url;
        } else {
          element.removeAttribute("href");
        }
      };
  
      const SOURCE_NAMES = {greenhouse: "Greenhouse", lever: "Lever", ashby: "Ashby", community: "Community", pasted: "Pasted", manual: "Manual"};
      const sourceLabel = data => {
        const key = String(data?.source || "").toLowerCase();
        const url = String(data?.listing_url || "").toLowerCase();
        if (data?._community) {
          const site = siteNameFromURL(data?.listing_url) || (data?.source && key !== "community" ? data.source : "");
          return site ? `Community \u00b7 ${site}` : "Community";
        }
        if (url.includes("greenhouse.io") || url.includes("grnh.se")) return "Greenhouse";
        if (url.includes("lever.co")) return "Lever";
        if (url.includes("ashbyhq.com")) return "Ashby";
        if (key === "pasted" || key === "manual" || !key) {
          const site = siteNameFromURL(data?.listing_url);
          if (site) return site;
        }
        if (SOURCE_NAMES[key]) return SOURCE_NAMES[key];
        return data?.board ? "Greenhouse" : "";
      };

      //fills the detail pane with a listing
      const displayPosting = (data, opened = false) => {
        window.UI.play(detailPane, "enter", 360);
        //A listing opened from results or reloaded has no id, but it may already be saved.
        const savedId = !data.id && data.board && savedByListing.get(`${data.board}/${data.external_id}`);
        if (savedId) data = {...data, id: savedId};
        detailSeq++;
        posting = data;
        fromResults = opened;
        selectedKey = opened ? keyOf(data) : "";
        markSelected();

        $("job-title").textContent = data.position_title || "Unknown position";
        $("job-company").textContent = data.company_name || "Unknown company";
        const src = sourceLabel(data);
        if ($("job-source")) {
          $("job-source").textContent = src ? `Source: ${src}` : "";
          $("job-source").hidden = !src;
        }
        $("job-location").textContent = data.location || "Location not listed";
        $("job-updated").textContent = data.updated_at ? `Updated ${new Date(data.updated_at).toLocaleDateString()}` : "";
        $("job-description").textContent = data.raw_text || "No description was given.";

        fillLogo($("job-logo"), data.company_name, data.company_url, data.board);
        setLink("job-link", data.listing_url); //a pasted job has no listing, so the button is not shown
        setLink("company-link", data.company_url);

        main.classList.add("has-detail");
        showDetail("state-success");
        $("detail-pane").scrollTop = 0;
        rememberListing(data);

        const saved = Boolean(data.id);
        //A job that is already saved is never offered for saving again. From search, a link, or Community it offers to open the
        //saved copy; the archive, trash and delete controls belong to the Saved tab.
        const viewSaved = saved && activeTab !== "saved";
        $("save-listing").hidden = saved || (!data.board && !data._community);
        $("save-listing").textContent = "Save job";
        $("save-listing").disabled = false;
        $("view-saved").hidden = !viewSaved;
        $("delete-job").hidden = !saved || viewSaved;
        drawLifecycle(saved && !viewSaved ? data : null);
        if (viewSaved) $("tailor-job").hidden = false;
        $("tailor-job").classList.toggle("primary", !viewSaved);

        $("reload-listing").hidden = saved || !data.board;
        $("back-to-results").hidden = !(opened && (results || activeTab === "saved"));
        renderAnalysis(saved ? data.id : null, saved ? "" : (data.raw_text || ""));
        renderCost(data);

      };

      //The last listing opened, for Home's "Keep reading". Everything the detail pane shows is kept, so it
      //opens again as it was without asking Greenhouse; Reload fetches it fresh.
      const LISTING_KEY = "fistbump.last-listing";
      function rememberListing(data) {
        if (!data.board || !data.external_id || data.archived_at || data.trashed_at) return;
        try {
          const {id, source, board, external_id, position_title, company_name, location, work_mode, updated_at, raw_text, company_url, listing_url} = data;
          localStorage.setItem(LISTING_KEY, JSON.stringify({id, source, board, external_id, position_title, company_name, location, work_mode,
            updated_at, raw_text, company_url, listing_url, at: new Date().toISOString()}));
        } catch {
          //Home just won't offer it
        }
      }
      function forgetListingIfMatching(removedJobs) {
        try {
          const kept = JSON.parse(localStorage.getItem(LISTING_KEY) || "null");
          if (!kept) return;
          const list = Array.isArray(removedJobs) ? removedJobs : [removedJobs];
          const matches = list.some(j => j && (
            (j.id && kept.id === j.id) ||
            (j.board && j.external_id && kept.board === j.board && kept.external_id === j.external_id)
          ));
          if (matches) localStorage.removeItem(LISTING_KEY);
        } catch {
          // ignore storage errors
        }
      }
      const notifyJobsChanged = (detail = {}) => {
        window.dispatchEvent(new CustomEvent("fistbump:jobs", {detail}));
      };

      //Opens a remembered listing in Find jobs as it was kept, without asking Greenhouse.
      const openSaved = async saved => {
        let data = saved;
        if (saved.id) {
          try {
            data = await jobsApi.get(saved.id); //still saved: show it with its saved state
          } catch {
            data = {...saved, id: undefined}; //deleted since
          }
        }
        displayPosting(data, Boolean(results?.postings.some(p => keyOf(p) === keyOf(saved))));
      };

      const showDetailLoading = message => {
        detailSeq++;
        $("detail-loading-message").textContent = message;
        main.classList.add("has-detail");
        showDetail("detail-loading");
      };

      const DEFAULT_PASTE_HINT = "Submit a job listing from LinkedIn, Indeed, Workday, or any other site. Add the link if you have it, then paste the job content below.";
      const openPasteWithLink = (url = "", customHint = "") => {
        selectTab("paste");
        const normalized = url ? normalizeJobLink(url) : "";
        const urlInput = $("paste-url");
        const hint = $("paste-hint");
        if (urlInput) urlInput.value = normalized;
        if (hint) {
          const site = siteNameFromURL(normalized);
          const text = customHint || (normalized
            ? `${site || "This site"} doesn't allow direct fetching — paste the job listing content below and click Preview to save it.`
            : DEFAULT_PASTE_HINT);
          hint.textContent = text;
          hint.classList.toggle("site-note", Boolean(normalized));
        }
        $("paste-result").replaceChildren();
        if (normalized) $("paste-text").focus();
        else (urlInput || $("paste-text")).focus();
      };

      //opens a pasted job link directly from the search bar
      const loadPosting = async url => {
        const normalized = normalizeJobLink(url);
        if (!isDirectATSLink(normalized)) {
          openPasteWithLink(normalized);
          return;
        }
        detailRetry = () => loadPosting(normalized);
        setSearchBusy(true);
        showDetailLoading("Opening the listing\u2026");
        const mine = detailSeq;
        try {
          const data = await window.FistBumpAPI.link(normalized);
          if (mine === detailSeq) displayPosting(data);
        } catch (error) {
          if (mine === detailSeq) showDetailError(error, normalized);
        } finally {
          setSearchBusy(false);
        }
      };

      //refreshes the open listing from Greenhouse
      const reloadPosting = async () => {
        if (!posting) return;
        const current = posting;
        const opened = fromResults;
        detailRetry = reloadPosting;
        $("reload-listing").disabled = true;
        showDetailLoading("Reloading the listing\u2026");
        const mine = detailSeq;
        try {
          const data = await window.FistBumpAPI.posting(current.board, current.external_id);
          if (mine === detailSeq) displayPosting(data, opened);
        } catch (error) {
          if (mine !== detailSeq) return;
          posting = current; //keep it, so Try again can reload the same listing
          showDetailError(error);
        } finally {
          $("reload-listing").disabled = false;
        }
      };

      //to create a text element
      const el = (tag, text) => {
        const node = document.createElement(tag);
        node.textContent = text;
        return node;
      };

      //a yearly range such as "$120k-$150k", or "" when the posting lists none
      const payText = result => {
        if (!result.pay_max) return searchedQuery.pay_min || searchedQuery.pay_max ? "Pay not listed" : "";
        const k = value => `$${Math.round(value / 1000)}k`;
        return result.pay_min && result.pay_min !== result.pay_max ? `${k(result.pay_min)}-${k(result.pay_max)}` : k(result.pay_max);
      };

      //"today", "3 days ago", "2 weeks ago"; an unreadable date gives ""
      const ago = iso => {
        const then = new Date(iso);
        if (!iso || isNaN(then)) return "";
        const days = Math.max(0, Math.floor((Date.now() - then.getTime()) / 86400000));
        if (days === 0) return "Updated today";
        if (days === 1) return "Updated yesterday";
        if (days < 14) return `Updated ${days} days ago`;
        if (days < 60) return `Updated ${Math.floor(days / 7)} weeks ago`;
        return `Updated ${Math.floor(days / 30)} months ago`;
      };

      //the first couple of lines of a description, on one line
      const snippet = text => {
        const flat = (text || "").replace(/\s+/g, " ").replace(/^[-\u2022*\s]+/, "").trim();
        if (flat.length <= 200) return flat;
        return flat.slice(0, 200).replace(/\s+\S*$/, "") + "\u2026";
      };

      //Logos come from each company's own site through the backend. Each company is asked for once, six at a
      //time. A finished logo is kept decoded in memory, so a tile can be painted at once with no flicker.
      const logos = new Map(); //key -> {promise, done, image}; image is null when the company has no logo
      let logoActive = 0;
      const logoWaiting = [];
      const nextLogo = () => {
        while (logoActive < 6 && logoWaiting.length) {
          const job = logoWaiting.shift();
          logoActive++;
          job().finally(() => { logoActive--; nextLogo(); });
        }
      };

      //one entry per company site, or per board when the posting names no site
      const logoEntry = (url, board) => {
        const key = url || `board:${board}`;
        if (!logos.has(key)) {
          const entry = {done: false, image: null};
          entry.promise = new Promise(resolve => {
            logoWaiting.push(async () => {
              const dataUrl = await window.FistBumpAPI.logo(url, board);
              if (dataUrl) {
                //decode before anyone shows it, so it appears whole instead of painting in
                const image = new Image();
                image.alt = "";
                image.src = dataUrl; //a data URL from the backend
                try {
                  await image.decode();
                  entry.image = image;
                } catch {
                  //an image that will not decode counts as no logo
                }
              }
              entry.done = true;
              resolve(entry);
            });
            nextLogo();
          });
          logos.set(key, entry);
        }
        return logos.get(key);
      };

      //Asks for the logos of some results and waits for them, but never longer than budget milliseconds, so a
      //slow site cannot hold the page back. Tiles for logos that are not in time fade in when they arrive.
      const preloadLogos = (postings, budget = 700) => {
        if (!window.Prefs.get("showLogos")) return Promise.resolve();
        const waits = [...new Set(postings.map(result => {
          const url = safeURL(result.company_url);
          return url || result.board ? logoEntry(url || "", result.board) : null;
        }))].filter(entry => entry && !entry.done).map(entry => entry.promise);
        if (!waits.length) return Promise.resolve();
        return Promise.race([Promise.all(waits), new Promise(resolve => setTimeout(resolve, budget))]);
      };

      //Paints a tile: the logo when it is ready, else a plain tile that fades to the logo, or to the company's
      //first letter if there is none. A tile whose answer is already known is painted with no animation.
      const fillLogo = (tile, name, companyUrl, board) => {
        const paint = entry => {
          tile.classList.remove("pending");
          if (entry?.image) {
            tile.style.backgroundColor = "white";
            tile.replaceChildren(entry.image.cloneNode());
          } else {
            let hue = 0;
            for (const ch of name || "") hue = (hue * 31 + ch.charCodeAt(0)) % 360;
            tile.style.backgroundColor = `hsl(${hue} 45% 40%)`;
            tile.replaceChildren(document.createTextNode((name || "?").trim().charAt(0).toUpperCase() || "?"));
          }
        };
        const url = safeURL(companyUrl);
        if ((!url && !board) || !window.Prefs.get("showLogos")) return paint(null);
        const entry = logoEntry(url || "", board);
        if (entry.done) return paint(entry);
        tile.classList.add("pending"); //blank until the answer arrives
        entry.promise.then(done => {
          if (!tile.isConnected) return;
          paint(done);
          tile.classList.add("reveal");
        });
      };

      //What shows under each result is the user's choice: boxes in the filters panel, kept on this computer, and
      //applied at once to the results already on screen. The defaults are the quiet set; the rest are opt-in.
      const SHOW_KEY = "fistbump.result-show";
      const SHOW_DEFAULTS = {fit: true, missing: true, pay_range: true, ratings: true,
        benefits_list: false, left_over: false, preview: false, posted: true, locations: true, company_site: true, source: true};
      const readShow = () => {
        try {
          const kept = JSON.parse(localStorage.getItem(SHOW_KEY) || "{}");
          return {...SHOW_DEFAULTS, ...Object.fromEntries(Object.entries(kept).filter(([k, v]) => k in SHOW_DEFAULTS && typeof v === "boolean"))};
        } catch {
          return {...SHOW_DEFAULTS};
        }
      };
      let show = readShow();
      const syncShowBoxes = () => {
        for (const box of document.querySelectorAll("input[data-show]")) box.checked = show[box.dataset.show];
      };

      //One quiet line of signals under each result: how well the resume fits, then good, fair or bad for pay,
      //benefits and cost of living. The search sends the ratings with each result; a saved job, which the
      //search did not rate, asks the same analysis for them. A part that cannot be judged is left out.
      const RATING = {
        pay: {well_above: "good", above: "good", in_line: "fair", below: "bad", well_below: "bad"},
        benefits: {strong: "good", decent: "fair", basic: "bad"},
        living: {comfortable: "good", okay: "fair", tight: "bad", short: "bad"},
      };
      const RATING_WORD = {good: "Good", fair: "Fair", bad: "Bad"};
      const RATING_LABEL = {pay: "Pay", benefits: "Benefits", living: "Living"};
      const RATING_TITLE = {pay: "Pay for the role", benefits: "Benefits", living: "Cost of living"};
      let fitAvailable = false; //some result in this search has a fit, so the resume is in play
      const ratingCache = new Map();
      const ratingsFor = result => {
        const key = [result.id, result.pay_min, result.pay_max, (result.raw_text || "").length].join("|");
        if (!ratingCache.has(key)) {
          ratingCache.set(key, window.FistBumpAPI.cost.analyze({
            title: result.position_title || "", location: result.location || "", work_mode: result.work_mode || "",
            pay_min: result.pay_min || 0, pay_max: result.pay_max || 0, text: result.raw_text || "",
          }).then(c => c.supported ? {
            pay: RATING.pay[c.pay?.verdict], benefits: RATING.benefits[c.benefits?.rating], living: RATING.living[c.lifestyle?.verdict],
            tips: {pay: c.pay?.headline, benefits: c.benefits?.headline, living: c.lifestyle?.headline},
            benefits_found: c.benefits?.found, left_monthly: c.lifestyle?.left_monthly,
          } : null).catch(() => null));
        }
        return ratingCache.get(key);
      };

      const pill = (kind, level, label, value, tip) => {
        const badge = document.createElement("span");
        badge.className = "signal";
        badge.dataset.kind = kind;
        badge.dataset.level = level;
        if (tip) badge.title = tip;
        if (label) {
          const name = document.createElement("span");
          name.className = "signal-label";
          name.textContent = label;
          badge.append(name, " ");
        }
        badge.append(value);
        return badge;
      };
      const ratingPills = (ratings, tips = {}) => show.ratings ? ["pay", "benefits", "living"]
        .filter(kind => ratings[kind])
        .map(kind => pill(kind, ratings[kind], RATING_LABEL[kind], RATING_WORD[ratings[kind]], `${RATING_TITLE[kind]}: ${tips[kind] || RATING_WORD[ratings[kind]]}`)) : [];

      //Fit with the resume, as one pill. A posting that lists no required skills gets a plain "No skill list", so a
      //missing fit reads as a fact about the posting and not as something that failed to load.
      const fitPill = fit => {
        const level = fit.percent >= 70 ? "high" : fit.percent >= 40 ? "mid" : "low";
        const likely = fit.likely || [];
        const tip = `${fit.matched} of ${fit.required} required skills are on your resume` +
          (likely.length ? `; ${likely.length} more look likely from related skills (${likely.join(", ")})` : "") + ".";
        return pill("fit", level, "", `${fit.percent}% fit`, tip);
      };

      const signalLine = result => {
        const line = document.createElement("div");
        line.className = "signals";
        if (result.fit && show.fit) line.append(fitPill(result.fit));
        else if (!result.fit && show.fit && fitAvailable && result.raw_text) {
          line.append(pill("fit", "none", "", "No skill list", "This posting lists no required skills, so there is nothing to compare with your resume."));
        }
        if (result.ratings) line.append(...ratingPills(result.ratings));
        else if (result.id && result.raw_text) {
          ratingsFor(result).then(ratings => {
            if (ratings && line.isConnected) line.append(...ratingPills(ratings, ratings.tips));
            line.hidden = !line.children.length;
          });
        }
        line.hidden = !line.children.length && !(result.id && result.raw_text);
        return line;
      };

      //Two optional lines of detail, from the same ratings: the benefits the posting names, and the money left over
      //each month. They are written into a holder that a saved job fills when its ratings come back.
      const detailLines = result => {
        const holder = document.createElement("div");
        holder.className = "result-lines";
        const fillLines = ratings => {
          if (!ratings) return;
          const add = (label, text) => {
            const line = document.createElement("p");
            line.className = "detail-line";
            const name = document.createElement("span");
            name.className = "signal-label";
            name.textContent = label;
            line.append(name, ` ${text}`);
            holder.append(line);
          };
          if (show.benefits_list && ratings.benefits_found?.length) {
            const found = ratings.benefits_found;
            add("Benefits", found.slice(0, 4).join(", ").toLowerCase().replace(/^./, c => c.toUpperCase()) + (found.length > 4 ? ` +${found.length - 4}` : ""));
          }
          if (show.left_over && Number.isFinite(ratings.left_monthly)) {
            const amount = `$${Math.abs(Math.round(ratings.left_monthly / 10) * 10).toLocaleString()}`;
            add("Left over", ratings.left_monthly >= 0 ? `about ${amount} a month after living costs` : `short by about ${amount} a month after living costs`);
          }
        };
        if (!show.benefits_list && !show.left_over) return holder;
        if (result.ratings) fillLines(result.ratings);
        else if (result.id && result.raw_text) ratingsFor(result).then(ratings => { if (holder.isConnected) fillLines(ratings); });
        return holder;
      };

      //What is missing, on one line: the real gaps first, and the count of the rest.
      const gapLine = fit => {
        if (!fit?.missing?.length) return null;
        const line = document.createElement("p");
        line.className = "gap-line";
        const label = document.createElement("span");
        label.className = "signal-label";
        label.textContent = "Missing";
        line.append(label, ` ${fit.missing.slice(0, 3).join(", ")}${fit.missing.length > 3 ? ` +${fit.missing.length - 3}` : ""}`);
        line.title = "Required skills that are not on your resume, and nothing related is";
        return line;
      };

      //get one row of the search results and define it's html
      const resultItem = (result, onOpen = r => displayPosting(r, true)) => {
        const item = document.createElement("li");
        item.className = "result";
        item.dataset.key = keyOf(result);
        const tile = document.createElement("div");
        tile.className = "logo-tile";
        tile.setAttribute("aria-hidden", "true");
        fillLogo(tile, result.company_name, result.company_url, result.board);
        const body = document.createElement("div");
        body.className = "result-body";
        item.append(tile, body);
        const open = document.createElement("button");
          open.type = "button";
          open.className = "result-open";
          open.append(
            el("strong", result.position_title || "Unknown position"),
            el ("span", [
              result.company_name,
              window.Places.summary(result.location).short,
              //the work mode, unless the location already says it ("Remote", "Hybrid - Boston")
              !String(result.location || "").toLowerCase().includes(String(result.work_mode || "").toLowerCase()) && result.work_mode,
              show.pay_range && payText(result),
              show.posted && ago(result.updated_at)
            ].filter(Boolean).join(" \u00b7 "))
          );

          open.addEventListener("click", () => onOpen(result));
          body.append(open);

          // Subtle ATS / Community source badge and "New" / "Pending review" pills.
          const srcLabel = show.source ? sourceLabel(result) : "";
          if (srcLabel) {
            const badge = el("small", srcLabel);
            badge.className = "result-state result-source";
            body.append(badge);
          }
          if (result._pending) {
            const pend = el("small", "Pending review");
            pend.className = "result-state result-pending";
            body.append(pend);
          }
          if (activePinnedSeen && !activePinnedSeen.has(keyOf(result))) {
            const fresh = el("small", "New");
            fresh.className = "result-state result-new";
            body.append(fresh);
          }

          //says what state the job is in, so an archived job and a trashed job do not look alike
          if (result.trashed_at || result.archived_at) {
            const state = el("small", result.trashed_at ? "In the Trash" : "Archived");
            state.className = "result-state";
            body.append(state);
          }

          //the logo, preview and padding open the listing too; the button stays for the keyboard
          item.addEventListener("click", event => {
            if (event.target.closest("button, a")) return; //they handle their own clicks
            const selection = window.getSelection();
            if (!selection.isCollapsed && item.contains(selection.anchorNode)) return; //a drag to select text is not a click to open
            onOpen(result);
          });

          body.append(signalLine(result));
          const gaps = show.missing ? gapLine(result.fit) : null;
          if (gaps) body.append(gaps);
          body.append(detailLines(result));
          if (show.preview) {
            const preview = snippet(result.raw_text);
            if (preview) {
              const p = el("p", preview);
              p.className = "result-snippet";
              body.append(p);
            }
          }

          //quiet extras share one line: other places it is posted, and the company's site
          const extras = document.createElement("div");
          extras.className = "result-extras";
          if (result.duplicates && show.locations) {
            extras.append(el("small", `Also in ${result.duplicates} other ${result.duplicates === 1 ? "location" : "locations"}`));
          }
          const url = show.company_site && safeURL(result.company_url);
          if (url) {
            const link = el("a", "Company site");
            link.className = "result-link";
            link.href = url;
            link.target = "_blank";
            link.rel = "noopener noreferrer";
            extras.append(link);
          }
          if (extras.children.length) body.append(extras);
          return item;
      };

      //explains what the search could and could not find, so a filter that did not apply is never silent.
      //Postings outside the requested place only get through as remote, so matched - location_matched is the remote count.
      const describeSearch = (data, query) => {
        const place = query.location;
        const mode = query.work_mode;
        //an older backend build does not send location_matched; skip the location notes rather than guess
        const located = Number.isInteger(data.location_matched);
        const noun = count => count === 1 ? "posting" : "postings";
        const notes = [];
        let empty = "No postings matched. Check the keywords and try again.";

        if ((query.source === "lever" || query.source === "ashby") && data.boards_total === 0) {
          const label = SOURCE_NAMES[query.source] || query.source;
          return {
            note: "",
            empty: `No ${label} boards on your watchlist yet. Add a ${label} board (for example ${query.source}:company) in Settings, or enter a company name to search it directly.`
          };
        }
        const company = query.company;
        if (company && !(data.companies || []).length) {
          //nothing to search: say so plainly instead of showing an empty list
          return {
            note: "",
            empty: `No listings for "${company}". Fistbump searches Greenhouse, Lever, and Ashby boards (plus any boards on your watchlist in Settings), and this company was not found. Check the spelling, add its board in Settings, or search by keywords or industry instead.`
          };
        }
        if (company) {
          const names = data.companies.slice(0, 3).join(", ") + (data.companies.length > 3 ? ` and ${data.companies.length - 3} more` : "");
          notes.push(`Searched ${names}.`);
          if (data.matched === 0) {
            empty = `${names} ${data.companies.length === 1 ? "has" : "have"} no postings that match your search. Try removing a keyword or filter.`;
          }
        }
        //tell the user when the titles are not what they typed, e.g. senior roles for "junior software engineer"
        if (query.keywords.length && data.matched > 0 && Number.isInteger(data.exact_matched)) {
          const typed = query.keywords.join(", ");
          const level = {Intern: "internship", Entry: "entry-level", Senior: "senior", Lead: "lead or management"}[data.level_wanted];
          if (data.exact_matched === 0) {
            notes.push(level
              ? `No exact match for "${typed}": there are no ${level} postings, so these are other levels of similar roles.`
              : `No exact match for "${typed}". These are the closest titles.`);
          } else if (data.exact_matched < data.matched) {
            const exact = data.exact_matched.toLocaleString();
            notes.push(`${exact} exact ${data.exact_matched === 1 ? "match" : "matches"} for "${typed}" ${data.exact_matched === 1 ? "comes" : "come"} first, followed by close matches${level ? " at other levels" : ""}.`);
          }
        }
        if (query.pay_min || query.pay_max) {
          const unlisted = data.postings.filter(posting => !posting.pay_max).length;
          if (unlisted > 0 && !query.require_pay) {
            notes.push(`Pay is not listed for ${unlisted} of the ${data.postings.length} shown on this page, so the pay range could not be checked for them. Tick "Hide postings that do not list pay" to remove them.`);
          }
        }

        if (place && mode === "Remote") {
          notes.push(`Showing remote postings only. Remote jobs are not tied to ${place}, so the location was not used.`);
        } else if (place && !located) {
          //nothing to say about the location
        } else if (place && data.matched === 0) {
          empty = mode === "Any"
            ? `No postings matched in ${place}, and no remote postings matched either. Try different keywords, a nearby city or a state.`
            : `No ${mode.toLowerCase()} postings matched in ${place}. Try Any or Remote, a nearby city or a state.`;
        } else if (place && data.location_matched === 0) {
          const remote = data.matched.toLocaleString();
          notes.push(mode === "Any"
            ? `No postings were found in ${place}. These ${remote} ${noun(data.matched)} are remote, so they may be open to people there, but none are listed for ${place}.`
            : `No ${mode.toLowerCase()} postings were found in ${place}. Only remote postings were found, and they are shown below.`);
        } else if (place) {
          const local = data.location_matched.toLocaleString();
          const remote = data.matched - data.location_matched;
          notes.push(`${local} ${noun(data.location_matched)} in ${place}` +
            (remote > 0 ? `, plus ${remote.toLocaleString()} remote.` : "."));
        }

        if (data.partial) {
          notes.push("Filtering or sorting by pay, fit or ratings needs each description, so only the top matches by relevance were read. Narrow the search to see more.");
        }
        if (data.rate_limited && data.postings.length > 0) {
          notes.push("Job boards are limiting how many requests they answer right now, so some boards or descriptions were skipped. Try again in a minute or two for the full list.");
        }
        if (data.boards_searched < data.boards_total) {
          const missed = data.boards_total - data.boards_searched;
          notes.push(`${missed.toLocaleString()} of ${data.boards_total.toLocaleString()} boards could not be searched, so some postings may be missing.`);
        }
        if (data.postings.length === 0 && data.matched > 0) {
          empty = data.rate_limited || data.errors.some(failed => /limiting requests/i.test(failed.message || ""))
            ? "Matches were found, but the job boards are limiting how many requests they answer right now, so their descriptions could not be loaded. Wait a minute, then try again."
            : "Matches were found, but their descriptions could not be loaded. Try again.";
        }
        return {note: notes.join(" "), empty};
      };

      //The filters a search ran with, as a row of chips under its count. Each says what it did, and its x takes that
      //one filter off and searches again, so nothing a search hides is a mystery.
      const RATING_WORDS = {fair: "fair or better", good: "good only"};
      const activeFilters = q => {
        const out = [];
        const add = (text, ids) => out.push({text, ids});
        const optionName = (id, value) => ($(id).querySelector(`option[value="${CSS.escape(String(value))}"]`)?.textContent || "").replace(/\s*\(\d+\)$/, "").trim();
        if (q.location) add(`In ${q.location}`, ["search-location"]);
        if (q.company) add(`At ${q.company}`, ["search-company"]);
        if (q.source) add(`Source: ${SOURCE_NAMES[q.source] || q.source}`, ["search-source"]);
        if (q.include_community && !q.source) add("Including community listings", ["search-include-community"]);
        if (q.categories?.length) add(optionName("search-industry", q.categories[0]) || q.categories[0], ["search-industry"]);
        if (q.work_mode && q.work_mode !== "Any") add(`${q.work_mode} only`, ["search-work-mode"]);
        if (q.posted_within) add(optionName("search-posted", q.posted_within).replace(/^Last/, "Posted in the last") || `Posted within ${q.posted_within} days`, ["search-posted"]);
        if (q.pay_min && q.pay_max) add(`Pay ${payLabel(q.pay_min)} to ${payLabel(q.pay_max)}`, ["search-pay-min", "search-pay-max"]);
        else if (q.pay_min) add(`Pay at least ${payLabel(q.pay_min)}`, ["search-pay-min"]);
        else if (q.pay_max) add(`Pay up to ${payLabel(q.pay_max)}`, ["search-pay-max"]);
        if (q.require_pay) add("Pay must be listed", ["search-require-pay"]);
        if (q.exclude?.length) add(`Without ${q.exclude.join(", ")}`, ["search-exclude"]);
        if (q.use_resume === false) add("Resume not used", ["search-use-resume"]);
        else {
          if (q.min_fit) add(`Fit ${q.min_fit}%+`, ["search-min-fit"]);
          if (q.max_missing !== null && q.max_missing !== undefined) add(q.max_missing === 0 ? "No missing skills" : `At most ${q.max_missing} missing`, ["search-max-missing"]);
        }
        if (q.use_ratings === false) add("Ratings off", ["search-use-ratings"]);
        else {
          if (q.min_pay_rating) add(`Pay: ${RATING_WORDS[q.min_pay_rating]}`, ["search-min-pay-rating"]);
          if (q.min_benefits) add(`Benefits: ${RATING_WORDS[q.min_benefits]}`, ["search-min-benefits"]);
          if (q.min_living) add(`Cost of living: ${RATING_WORDS[q.min_living]}`, ["search-min-living"]);
        }
        return out;
      };
      const FILTER_DEFAULTS = {"search-use-resume": true, "search-use-ratings": true};
      const renderActiveFilters = q => {
        const host = $("result-filters");
        const filters = activeFilters(q);
        //no filters, no row: the space goes to the results
        host.hidden = !filters.length;
        const label = document.createElement("span");
        label.className = "filter-chips-label";
        label.textContent = "Filters";
        host.replaceChildren(label, ...filters.map(filter => {
          const chip = document.createElement("span");
          chip.className = "filter-chip";
          chip.append(filter.text);
          const remove = document.createElement("button");
          remove.type = "button";
          remove.className = "filter-chip-x";
          remove.textContent = "×";
          remove.setAttribute("aria-label", `Remove filter: ${filter.text}`);
          remove.addEventListener("click", () => {
            for (const id of filter.ids) {
              const field = $(id);
              if (field.type === "checkbox") field.checked = id in FILTER_DEFAULTS ? FILTER_DEFAULTS[id] : false;
              else if (field.tagName === "SELECT") field.value = field.options[0].value;
              else field.value = "";
            }
            applyResumeSwitch();
            applyRatingsSwitch();
            saveForm();
            searchForm.requestSubmit();
          });
          chip.append(remove);
          return chip;
        }));
      };

      //fill all results
      const renderResults = data => {
        const count = (n, one, many) => `${n.toLocaleString()} ${n === 1 ? one : many}`;
        //one short line; the postings scanned are in the tooltip, so the count never wraps beside the sort
        $("result-counts").textContent = `${data.matched.toLocaleString()}${data.partial ? "+" : ""} ${data.matched === 1 && !data.partial ? "match" : "matches"}` +
          ` \u00b7 ${count(data.boards_searched, "board", "boards")}`;
        $("result-counts").title = `${count(data.postings_scanned, "posting", "postings")} read on ${count(data.boards_searched, "board", "boards")}`;
        const {note, empty} = describeSearch(data, searchedQuery);
        $("result-note").hidden = !note;
        $("result-note").textContent = note;
        $("result-empty-message").textContent = empty;
        $("result-empty").hidden = data.postings.length > 0;
        //not map(resultItem): map's index argument would land in onOpen
        renderActiveFilters(searchedQuery);
        fitAvailable = searchedQuery.use_resume !== false && data.postings.some(result => result.fit);
        $("result-list").replaceChildren(...data.postings.map(result => resultItem(result)));
        window.UI.stagger($("result-list"), 22, 10);
        markSelected();
        markSavedRows();

        renderPager();

        $("result-errors").hidden = data.errors.length === 0;
        $("result-errors-list").replaceChildren(
          ...data.errors.map(failed => el("li", `${failed.board}: ${failed.message}`))
        );
      };

      let searchSeq = 0; //counts searches, so an older one that finishes late is ignored
      let pageSeq = 0; //counts page changes and searches, so a slow page cannot land after either

      // Filters crowdsourced community listings (published + local pending) using the active search criteria.
      const searchCommunityListings = async query => {
        if (!window.Community?.loadResources) return {postings: [], scanned: 0};
        const res = await window.Community.loadResources(false).catch(() => ({listings: [], pending: []}));
        const all = [
          ...(res.pending || []).map(x => ({...x, _community: true, community_id: x.id, id: undefined, _pending: true, updated_at: x.added})),
          ...(res.listings || []).map(x => ({...x, _community: true, community_id: x.id, id: undefined, _pending: false, updated_at: x.added})),
        ];
        const kws = (query.keywords || []).map(k => k.toLowerCase()).filter(Boolean);
        const exs = (query.exclude || []).map(e => e.toLowerCase()).filter(Boolean);
        const co = String(query.company || "").trim().toLowerCase();
        const loc = String(query.location || "").trim().toLowerCase();
        const mode = query.work_mode && query.work_mode !== "Any" ? query.work_mode.toLowerCase() : "";
        const cutoff = query.posted_within > 0 ? Date.now() - query.posted_within * 86400000 : 0;
        const matched = all.filter(item => {
          const title = String(item.position_title || "").toLowerCase();
          const company = String(item.company_name || "").toLowerCase();
          const text = String(item.raw_text || "").toLowerCase();
          const place = String(item.location || "").toLowerCase();
          const itemMode = String(item.work_mode || "").toLowerCase();
          if (co && !company.includes(co)) return false;
          if (mode && itemMode !== mode) return false;
          if (loc && !place.includes(loc) && itemMode !== "remote") return false;
          if (cutoff && item.updated_at) {
            const ts = Date.parse(item.updated_at);
            if (!Number.isNaN(ts) && ts < cutoff) return false;
          }
          if (exs.some(e => title.includes(e))) return false;
          if (kws.length && !kws.some(k => title.includes(k) || company.includes(k) || text.includes(k) || String(item.source || "").toLowerCase().includes(k))) return false;
          return true;
        });
        return {postings: matched, scanned: all.length};
      };

      //main function to run the search
      const runSearch = async query => {
        searchRetry = () => runSearch(query);
        main.classList.remove("has-detail"); //a narrow window goes back to the list
        const previous = {pages, currentPage, results, searchedQuery};
        const mine = ++searchSeq;
        pageSeq++; //a page of the old search must not land on this one
        searchedQuery = query;
        results = null;
        setSearchBusy(true);
        const {include_community, ...backendQuery} = query;
        const wide = !query.company && !query.categories.length && boardTotal;
        showLoading(query.source === "community"
          ? "Searching community listings\u2026"
          : wide
            ? `Searching ${boardTotal.toLocaleString()} boards\u2026 Boards read in the last 6 hours load from this computer; the rest are fetched from Greenhouse, Lever, and Ashby, which takes longer.`
            : "Searching Greenhouse, Lever, and Ashby boards\u2026 Boards read in the last 6 hours load from this computer.", true);

        try {
          let data;
          if (query.source === "community") {
            const c = await searchCommunityListings(query);
            data = {
              postings: c.postings,
              matched: c.postings.length,
              boards_searched: 1,
              boards_total: 1,
              postings_scanned: c.scanned,
              errors: [],
            };
          } else {
            const [atsData, commData] = await Promise.all([
              window.FistBumpAPI.search({...backendQuery, offset: 0}),
              !query.source && include_community ? searchCommunityListings(query) : Promise.resolve(null),
            ]);
            data = atsData;
            if (commData?.postings?.length) {
              data = {
                ...atsData,
                postings: [...commData.postings.slice(0, 10), ...(atsData.postings || [])],
                matched: (atsData.matched || 0) + commData.postings.length,
                postings_scanned: (atsData.postings_scanned || 0) + commData.scanned,
              };
            }
          }
          if (mine !== searchSeq) return; //a newer search owns the page now
          await preloadLogos(data.postings);
          if (mine !== searchSeq) return;
          pages = new Map([[1, data]]);
          currentPage = 1;
          results = data;
          $("result-sort").value = query.sort || "relevance";
          renderResults(data);
          syncPinnedSeen(data);
          showState("state-results");
          $("results-pane").scrollTop = 0;
          return true;
        } catch (error) {
          if (mine !== searchSeq) return;
          if (error.message === "Search canceled") {
            //put back what was on screen before this search
            ({pages, currentPage, results, searchedQuery} = previous);
            $("result-sort").value = searchedQuery.sort || "relevance";
            if (results) {
              renderResults(results);
              showState("state-results");
            } else {
              showInitial("Search canceled.", "09-search-stopped");
            }
            return;
          }
          showError(error);
        } finally {
          if (mine === searchSeq) setSearchBusy(false);
        }
      };

      //Pages are numbered from 1. A page is fetched once, then kept until the next search.
      const pageCount = () => Math.min(Math.ceil((results?.matched || 0) / pageSize()), Math.floor(MAX_OFFSET / pageSize()) + 1);

      //page numbers to show: the first, the last and the ones around the current page, with gaps as null
      const pageWindow = (current, total) => {
        const wanted = new Set([1, total]);
        for (let n = current - 2; n <= current + 2; n++) {
          if (n >= 1 && n <= total) wanted.add(n);
        }
        const sorted = [...wanted].sort((a, b) => a - b);
        return sorted.flatMap((n, i) => i > 0 && n - sorted[i - 1] > 1 ? [null, n] : [n]);
      };

      const renderPager = () => {
        const total = pageCount();
        const pager = $("pager");
        pager.hidden = total < 2;
        if (total < 2) return;

        const button = (label, page, {current = false, disabled = false} = {}) => {
          const node = el("button", label);
          node.type = "button";
          node.disabled = disabled;
          if (current) node.setAttribute("aria-current", "page");
          node.addEventListener("click", () => goToPage(page));
          return node;
        };
        pager.replaceChildren(
          button("Previous", currentPage - 1, {disabled: currentPage === 1}),
          ...pageWindow(currentPage, total).map(n =>
            n === null ? el("span", "\u2026") : button(String(n), n, {current: n === currentPage})),
          button("Next", currentPage + 1, {disabled: currentPage === total})
        );
      };

      //shows a page of results, asking the backend only if the page is not kept yet
      const goToPage = async page => {
        if (page < 1 || page > pageCount()) return;
        const mine = ++pageSeq;
        searchRetry = () => goToPage(page);
        let data = pages.get(page);

        if (!data) {
          setSearchBusy(true);
          $("pager").querySelectorAll("button").forEach(node => { node.disabled = true; });
          try {
            const {include_community: _ic, ...backendQuery} = searchedQuery;
            data = await window.FistBumpAPI.search({...backendQuery, offset: (page - 1) * pageSize()});
          } catch (error) {
            if (mine !== pageSeq) return;
            if (error.message === "Search canceled") renderPager(); //re-enable the page buttons
            else showError(error);
            return;
          } finally {
            if (mine === pageSeq) setSearchBusy(false); //a newer search keeps the button locked
          }
          if (mine !== pageSeq) return;
          pages.set(page, data);
        }
        await preloadLogos(data.postings);
        if (mine !== pageSeq) return;

        //a posting is on one page only, since the backend's order is fixed; drop a repeat if the data moved between requests
        const shown = new Set();
        for (const [number, other] of pages) {
          if (number < page) other.postings.forEach(p => shown.add(`${p.board}/${p.external_id}`));
        }
        currentPage = page;
        results = {...data, postings: data.postings.filter(p => !shown.has(`${p.board}/${p.external_id}`))};
        renderResults(results);
        showState("state-results");
        $("results-pane").scrollTop = 0;
      };

      //rest of script is state handling and button events
      tabs.find.addEventListener("click", () => selectTab("find"));
      tabs.paste.addEventListener("click", () => selectTab("paste"));
      tabs.saved.addEventListener("click", () => selectTab("saved"));
      tabs.settings.addEventListener("click", () => selectTab("settings"));

      $("search-keywords").addEventListener("input", syncSearchButton);

      searchForm.addEventListener("submit", event => {
        event.preventDefault();

        const rawKeywords = $("search-keywords").value.trim();
        if (looksLikeJobLink(rawKeywords)) {
          loadPosting(rawKeywords);
          return;
        }

        const words = id => $(id).value.split(",").map(word => word.trim()).filter(Boolean);
        const number = id => Math.max(0, parseInt($(id).value, 10) || 0);
        const pay = id => parsePay($(id).value);
        //"data analyst, " is "data analyst": stray commas and spaces are tidied in the box as well
        $("search-keywords").value = words("search-keywords").join(", ");
        const query = {
          source: $("search-source")?.value || "",
          include_community: Boolean($("search-include-community")?.checked),
          keywords: words("search-keywords"),
          location: $("search-location").value.trim(),
          work_mode: $("search-work-mode").value,
          company: $("search-company").value.trim(),
          categories: $("search-industry").value ? [$("search-industry").value] : [],
          posted_within: number("search-posted"),
          exclude: words("search-exclude"),
          pay_min: pay("search-pay-min"),
          pay_max: pay("search-pay-max"),
          require_pay: $("search-require-pay").checked,
          use_resume: $("search-use-resume").checked,
          min_fit: $("search-use-resume").checked ? number("search-min-fit") : 0,
          max_missing: !$("search-use-resume").checked || $("search-max-missing").value === "" ? null : number("search-max-missing"),
          use_ratings: $("search-use-ratings").checked,
          min_pay_rating: $("search-use-ratings").checked ? $("search-min-pay-rating").value : "",
          min_benefits: $("search-use-ratings").checked ? $("search-min-benefits").value : "",
          min_living: $("search-use-ratings").checked ? $("search-min-living").value : "",
          sort: $("result-sort").value,
          limit: window.Prefs.get("pageSize")
        };
        for (const id of ["search-pay-min", "search-pay-max"]) {
          if ($(id).value.trim() && !pay(id)) {
            showInitial("Enter pay as a yearly amount, such as 90000 or 90k.");
            setFiltersOpen(true);
            $(id).focus();
            return;
          }
        }
        if (!query.keywords.length && !query.location && !query.categories.length && !query.company && !query.source) {
          showInitial("Enter keywords, a location, a company, an industry, or paste a Greenhouse, Lever, or Ashby job link.");
          $("search-keywords").focus();
          return;
        }
        saveForm();
        //only a search that worked goes into the history and the suggestions
        runSearch(query).then(ok => {
          if (!ok) return;
          recordSearch();
          window.Suggest.remember("keywords", query.keywords);
          window.Suggest.remember("location", [query.location]);
          window.Suggest.remember("company", [query.company]);
        });
      });

      $("back-to-results").addEventListener("click", () => main.classList.remove("has-detail"));
      $("reload-listing").addEventListener("click", reloadPosting);
      $("try-again").addEventListener("click", () => searchRetry ? searchRetry() : selectTab("find"));
      $("detail-try-again").addEventListener("click", () => detailRetry ? detailRetry() : showDetailEmpty());
      $("detail-paste-instead")?.addEventListener("click", () => openPasteWithLink(failedLinkUrl));
      $("open-paste")?.addEventListener("click", () => openPasteWithLink(""));
      $("submit-listing")?.addEventListener("click", () => window.SubmitListing?.open());

      const jobsApi = window.FistBumpAPI.jobs;

      // ---- Save from ATS search or Community resources (both go through displayPosting) ----
      $("save-listing").addEventListener("click", async () => {
        const btn = $("save-listing");
        const current = posting;
        btn.disabled = true;
        try {
          if (current._community) {
            const payload = {
              company_name: current.company_name,
              position_title: current.position_title,
              source: "pasted",
              raw_text: current.raw_text,
            };
            if (current.location) payload.location = current.location;
            if (current.work_mode) payload.work_mode = current.work_mode;
            if (current.listing_url) payload.listing_url = current.listing_url;
            if (current.company_url) payload.company_url = current.company_url;
            const job = await jobsApi.create(payload);
            if (job) {
              savedByListing.set(`community/${current.community_id}`, job.id);
              if (posting === current) displayPosting({...current, ...job}, fromResults);
              loadSaved();
              notifyJobsChanged();
            }
            return;
          }
          const {imported, refreshed} = await window.FistBumpAPI.importJobs([{source: current.source || "greenhouse", board: current.board, external_id: current.external_id}]);
          const job = imported[0] || refreshed[0];
          if (job) {
            savedByListing.set(`${job.board}/${job.external_id}`, job.id);
            markSavedRows();
            if (posting === current) displayPosting({...current, ...job}, fromResults); //now shown as saved
            loadSaved();
            notifyJobsChanged();
          }
        } catch (error) {
          btn.disabled = false;
          window.UI.alert(error.message); // for example, the posting has closed (404)
        }
      });



      //A job already saved opens in Saved jobs, where it can be tailored, tracked or archived.
      $("view-saved").addEventListener("click", () => {
        const id = posting?.id;
        if (id) window.JobPage.open(id);
      });

      // ---- Saved tab ----
      //Active, Archived or Trash
      const savedScope = () => ({archived: {archived: "only"}, trash: {trashed: "only"}})[$("saved-scope").value] || {};

      //How long a job stays in the Trash before the app deletes it for good. The wording names the
      //real number, so the Trash and the archive never sound like the same thing.
      let trashDays = 7;
      const applyTrashDays = () => {
        const option = [...$("saved-scope").options].find(o => o.value === "trash");
        if (option) option.textContent = `Trash — deleted in ${trashDays} days`;
        const trashScopeBtn = document.querySelector('#saved-scope-toggle button[data-scope="trash"]');
        if (trashScopeBtn) trashScopeBtn.title = `Trash — deleted in ${trashDays} days`;
        $("trash-job").title = `Take this job out of Saved jobs. It is deleted for good after ${trashDays} days unless you restore it first.`;
      };
      const loadTrashDays = async () => {
        try {
          const values = await window.api.settings.get();
          const days = Number(values?.["jobs.trash_days"]);
          if (Number.isFinite(days) && days > 0) trashDays = days;
        } catch {
          //the settings are not there, so the default of a week stands
        }
        applyTrashDays();
      };
      loadTrashDays();
      let savedRows = [];
      const selectedSaved = new Set();

      const syncSavedBulk = () => {
        const bar = $("saved-bulk");
        const selectAll = $("saved-select-all");
        const countEl = $("saved-selected-count");
        const actions = $("saved-bulk-actions");
        const trashBtn = $("saved-bulk-trash");
        if (!bar) return;
        bar.hidden = savedRows.length === 0;
        const shownIds = savedRows.map(j => j.id);
        const picked = shownIds.filter(id => selectedSaved.has(id)).length;
        selectAll.checked = savedRows.length > 0 && picked === savedRows.length;
        selectAll.indeterminate = picked > 0 && picked < savedRows.length;
        countEl.textContent = picked ? `${picked} selected` : "";
        actions.hidden = picked === 0;
        if (trashBtn) trashBtn.hidden = $("saved-scope").value === "trash";
      };

      const savedRowItem = job => {
        const item = resultItem(job, row => window.JobPage.open(row.id)); //v3: a saved job opens on its own page
        const check = document.createElement("input");
        check.type = "checkbox";
        check.className = "saved-check";
        check.checked = selectedSaved.has(job.id);
        check.setAttribute("aria-label", `Select ${job.position_title || "job"} at ${job.company_name || "company"}`);
        if (check.checked) item.classList.add("selected-bulk");
        check.addEventListener("click", event => event.stopPropagation());
        check.addEventListener("change", () => {
          if (check.checked) selectedSaved.add(job.id);
          else selectedSaved.delete(job.id);
          item.classList.toggle("selected-bulk", check.checked);
          syncSavedBulk();
        });
        item.prepend(check);
        return item;
      };

      const loadSaved = async () => {
        try {
          const jobs = await jobsApi.list($("saved-q").value.trim(), savedScope());
          savedRows = jobs;
          const validIds = new Set(jobs.map(j => j.id));
          for (const id of [...selectedSaved]) if (!validIds.has(id)) selectedSaved.delete(id);
          loadSavedIndex(); //deleted, trashed and newly saved jobs change which listings read as saved
          $("saved-list").replaceChildren(...jobs.map(savedRowItem));
          $("saved-empty-message").textContent = ({
            archived: "No archived jobs. Archived jobs are hidden from Saved jobs, but nothing is deleted — you can bring them back any time.",
            trash: `The Trash is empty. A job you move to the Trash is deleted for good after ${trashDays} days, unless you restore it before then.`,
          })[$("saved-scope").value] || "No saved jobs yet. Save one from Find jobs or Paste a job.";
          $("saved-empty").hidden = jobs.length > 0;
          syncSavedBulk();
          markSelected();
        } catch (error) {
          $("saved-empty-message").textContent = error.message;
          $("saved-empty").hidden = false;
        }
      };

      $("saved-select-all")?.addEventListener("change", event => {
        const checked = event.target.checked;
        if (checked) savedRows.forEach(j => selectedSaved.add(j.id));
        else selectedSaved.clear();
        for (const row of $("saved-list").querySelectorAll(".result")) {
          const box = row.querySelector(".saved-check");
          if (box) box.checked = checked;
          row.classList.toggle("selected-bulk", checked);
        }
        syncSavedBulk();
      });

      $("saved-bulk-clear")?.addEventListener("click", () => {
        selectedSaved.clear();
        for (const row of $("saved-list").querySelectorAll(".result")) {
          const box = row.querySelector(".saved-check");
          if (box) box.checked = false;
          row.classList.remove("selected-bulk");
        }
        syncSavedBulk();
      });

      const runSavedBulk = async mode => {
        const ids = savedRows.filter(j => selectedSaved.has(j.id)).map(j => j.id);
        if (!ids.length) return;
        const noun = ids.length === 1 ? "saved job" : "saved jobs";
        const question = mode === "trash"
          ? `Move ${ids.length} ${noun} to the Trash?\n\nJobs with an active application in the Tracker will be skipped.`
          : `Delete ${ids.length} ${noun} permanently?\n\nThis cannot be undone. Jobs with an application in the Tracker will be skipped until their application is deleted in Tracker.`;
        const ok = await window.UI.ask(question, {
          confirm: mode === "trash" ? "Move to Trash" : "Delete permanently",
          danger: true,
          title: mode === "trash" ? "Move saved jobs to Trash" : "Delete saved jobs",
        });
        if (!ok) return;
        const settled = await Promise.allSettled(ids.map(id => mode === "trash" ? jobsApi.trash(id) : jobsApi.delete(id)));
        const succeeded = [];
        let skipped = 0;
        settled.forEach((r, idx) => {
          if (r.status === "fulfilled") succeeded.push(ids[idx]);
          else skipped++;
        });
        if (succeeded.length) {
          const removedObjs = savedRows.filter(j => succeeded.includes(j.id));
          forgetListingIfMatching(removedObjs);
          notifyJobsChanged();
        }
        if (posting?.id && succeeded.includes(posting.id)) showDetailEmpty();
        selectedSaved.clear();
        await loadSaved();
        if (skipped > 0) {
          const actionWord = mode === "trash" ? "Moved" : "Deleted";
          window.UI.alert(
            `${actionWord} ${succeeded.length} ${succeeded.length === 1 ? "job" : "jobs"}. Skipped ${skipped} ${skipped === 1 ? "job" : "jobs"} because ${skipped === 1 ? "it has" : "they have"} an application in the Tracker — delete ${skipped === 1 ? "that application" : "those applications"} in Tracker first.`,
            "Some jobs were skipped"
          );
        }
      };

      $("saved-bulk-trash")?.addEventListener("click", () => runSavedBulk("trash"));
      $("saved-bulk-delete")?.addEventListener("click", () => runSavedBulk("delete"));

      $("saved-q").addEventListener("input", loadSaved);
      const syncSavedScopeToggle = () => {
        const cur = $("saved-scope").value;
        document.querySelectorAll("#saved-scope-toggle button[data-scope]").forEach(btn => {
          btn.setAttribute("aria-pressed", String(btn.dataset.scope === cur));
        });
      };
      document.querySelectorAll("#saved-scope-toggle button[data-scope]").forEach(btn => {
        btn.addEventListener("click", () => {
          if ($("saved-scope").value === btn.dataset.scope) return;
          $("saved-scope").value = btn.dataset.scope;
          syncSavedScopeToggle();
          selectedSaved.clear();
          loadSaved();
        });
      });
      $("saved-scope").addEventListener("change", () => { syncSavedScopeToggle(); selectedSaved.clear(); loadSaved(); });
      $("saved-form").addEventListener("submit", event => event.preventDefault());

      // Same shape as loadPosting
      const openSavedJob = async row => {
        detailRetry = () => openSavedJob(row);
        showDetailLoading("Opening the job\u2026");
        const mine = detailSeq;
        try {
          const job = await jobsApi.get(row.id);
          if (mine === detailSeq) displayPosting(job, true);
        } catch (error) {
          if (mine === detailSeq) showDetailError(error);
        }
      };

      // ---- Track, archive and trash (phase 7) ----
      const appsApi = window.FistBumpAPI.applications;

      //the application for a job, archived ones included
      async function applicationFor(jobId) {
        return (await window.FistBumpAPI.applications.list({archived: "include"})).find(a => a.job_id === jobId) || null;
      }

      //says what the open job's own state means: archived is kept, trashed is on its way out
      const stateNote = job => {
        const note = $("job-state");
        note.dataset.state = "";
        if (job?.trashed_at) {
          note.dataset.state = "trash";
          note.textContent = `This job is in the Trash. It is deleted for good after ${trashDays} days, unless you restore it before then.`;
        } else if (job?.archived_at) {
          note.dataset.state = "archive";
          note.textContent = "This job is archived. It is hidden from Saved jobs, but nothing is deleted — unarchive it to bring it back.";
        } else {
          note.textContent = "";
          note.hidden = true;
          return;
        }
        note.hidden = false;
      };

      //shows the buttons that fit the open saved job: trashed jobs only offer Restore
      async function drawLifecycle(job) {
        const mine = ++lifecycleSeq;
        trackedApp = null;
        const trashed = Boolean(job?.trashed_at);
        const archived = Boolean(job?.archived_at);
        stateNote(job);
        $("track-job").hidden = !job || trashed;
        $("track-job").textContent = "Track";
        $("archive-job").hidden = !job || trashed || archived;
        $("archive-app-option").hidden = true;
        $("unarchive-job").hidden = !job || trashed || !archived;
        $("trash-job").hidden = !job || trashed;
        $("restore-job").hidden = !trashed;
        $("tailor-job").hidden = !job || trashed;
        if (!job || trashed) return;
        try {
          const app = await applicationFor(job.id);
          if (mine !== lifecycleSeq) return;
          trackedApp = app;
          $("track-job").textContent = app ? "Open in tracker" : "Track";
          $("archive-app-option").hidden = !app || archived;
          $("archive-app").checked = true;
        } catch {
          //Track still works; it reports any error itself
        }
      }

      //after a change, the open job and the Saved list are read again
      const refreshJob = async () => {
        const job = await jobsApi.get(posting.id);
        displayPosting(job, fromResults);
        loadSaved();
      };

      $("track-job").addEventListener("click", async () => {
        if (!posting?.id) return;
        try {
          const existing = trackedApp || await applicationFor(posting.id);
          if (existing) return window.Tracker.open(existing.id);
          const created = await appsApi.create({job_id: posting.id});
          trackedApp = created;
          $("track-job").textContent = "Open in tracker";
          $("archive-app-option").hidden = Boolean(posting.archived_at);
        } catch (error) {
          //409: tracked already, for example from another window; open that row
          const id = error.details?.application_id;
          if (id) return window.Tracker.open(id);
          window.UI.alert(error.message);
        }
      });

      const archiveJob = async () => {
        const target = posting;
        await jobsApi.archive(target.id, {archive_application: Boolean(trackedApp) && $("archive-app").checked});
        forgetListingIfMatching(target);
        notifyJobsChanged();
        await refreshJob();
      };

      $("archive-job").addEventListener("click", async () => {
        if (!posting?.id) return;
        try {
          await archiveJob();
        } catch (error) {
          window.UI.alert(error.message);
        }
      });

      $("unarchive-job").addEventListener("click", async () => {
        if (!posting?.id) return;
        try {
          await jobsApi.unarchive(posting.id);
          notifyJobsChanged();
          await refreshJob();
        } catch (error) {
          window.UI.alert(error.message);
        }
      });

      //A job with an application cannot go to the Trash, so the archive is offered instead.
      const archiveInstead = async () => {
        if (!await window.UI.ask("This job has an application in the Tracker, so it cannot go to the Trash.\n\nArchive keeps the job hidden without deleting it, and can be undone. Archive it instead?",
          {confirm: "Archive it", title: "Cannot trash — job has an application"})) return;
        try {
          await archiveJob();
        } catch (archiveError) {
          window.UI.alert(archiveError.message);
        }
      };

      $("trash-job").addEventListener("click", async () => {
        if (!posting?.id) return;
        if (trackedApp) return archiveInstead();
        const question = `Move "${posting.position_title}" at ${posting.company_name} to the Trash?\n\nIt leaves Saved jobs now, and it is deleted for good after ${trashDays} days, unless you restore it before then.`;
        if (!await window.UI.ask(question, {confirm: "Move to Trash", danger: true, title: "Move to Trash"})) return;
        try {
          const target = posting;
          await jobsApi.trash(target.id);
          forgetListingIfMatching(target);
          notifyJobsChanged();
          showDetailEmpty();
          loadSaved();
        } catch (error) {
          //409: the job has an application, so it can be archived instead
          if (error.code !== "conflict") return window.UI.alert(error.message);
          await archiveInstead();
        }
      });

      $("restore-job").addEventListener("click", async () => {
        if (!posting?.id) return;
        try {
          await jobsApi.restore(posting.id);
          notifyJobsChanged();
          await refreshJob();
        } catch (error) {
          window.UI.alert(error.message);
        }
      });

      //the tracker opens a job here: show it in the Saved tab
      window.JobParser.openJob = id => window.JobPage.open(id);

      // ---- Tailor: open the saved job in the Tailor view ----
      $("tailor-job").addEventListener("click", () => {
        if (posting?.id) window.Tailor.open(posting.id);
      });

      // ---- Delete ----
      $("delete-job").addEventListener("click", async () => {
        if (!posting?.id) return;
        const question = `Delete "${posting.position_title}" at ${posting.company_name} now?\n\nThis cannot be undone. To keep the job for a while and delete it later, move it to the Trash instead.`;
        if (!await window.UI.ask(question, {confirm: "Delete now", danger: true, title: "Delete job"})) return;
        try {
          const target = posting;
          await jobsApi.delete(target.id);
          forgetListingIfMatching(target);
          notifyJobsChanged();
          showDetailEmpty();
          loadSaved();
        } catch (error) {
          window.UI.alert(error.message); // the 409 message when an application uses this job
        }
      });

      // ---- Analysis panel: runs whenever a saved job opens ----
      let analysisSeq = 0;
      const renderAnalysis = async (id, text = "") => {
        const {el, fill, chips} = window.ResumeBuilder;
        const box = $("analysis");
        const mine = ++analysisSeq;
        box.hidden = !id && !text;
        box.replaceChildren();
        if (!id && !text) return;
        try {
          const [a, master] = await Promise.all([(id ? jobsApi.analyze(id) : jobsApi.analyzeText(text)), window.FistBumpAPI.resume.get()]);
          if (mine !== analysisSeq) return; // a newer job opened meanwhile
          const titles = Object.fromEntries(master.experiences.map(x => [x.id, `${x.job_title}, ${x.company_name}`]));
          const {heading, groups} = window.ResumeBuilder.skillMatch(a, titles);
          fill(box, el("h3", {text: heading}), ...groups);
        } catch (error) {
          // includes the 409 "create a profile first"
          if (mine === analysisSeq) fill(box, el("p", {class: "note", text: error.message}));
        }
      };      

      // ---- Pay, benefits and cost of living: runs for any posting, saved or not ----
      let costSeq = 0;
      const money = n => `${n < 0 ? "−" : ""}$${Math.abs(Math.round(n)).toLocaleString()}`;
      const PILL = {
        pay: {well_above: "Well above market", above: "Above market", in_line: "At market", below: "Below market", well_below: "Well below market", unknown: "Not compared"},
        benefits: {strong: "Strong", decent: "Decent", basic: "Limited", unclear: "Unclear", unlisted: "Not listed"},
        life: {comfortable: "Comfortable", okay: "Workable", tight: "Tight", short: "Short"},
      };
      const TONE = {well_above: "good", above: "good", in_line: "warn", below: "bad", well_below: "bad", unknown: "ok",
        comfortable: "good", okay: "warn", tight: "bad", short: "bad", strong: "good", decent: "warn", basic: "bad", unclear: "ok", unlisted: "ok"};
      const renderCost = async data => {
        const {el, fill, chips} = window.ResumeBuilder;
        const box = $("cost");
        const mine = ++costSeq;
        box.hidden = true;
        box.replaceChildren();
        if (!data?.raw_text && !data?.pay_min && !data?.position_title) return;
        try {
          const c = await window.FistBumpAPI.cost.analyze({
            title: data.position_title || "", location: data.location || "", work_mode: data.work_mode || "",
            pay_min: data.pay_min || 0, pay_max: data.pay_max || 0, text: data.raw_text || "",
          });
          if (mine !== costSeq) return; //a newer listing opened meanwhile
          const card = (title, kind, verdict, headline, ...more) => el("div", {class: "cost-card"},
            el("div", {class: "cost-card-head"}, el("h4", {text: title}),
              kind ? el("span", {class: `cost-pill ${TONE[verdict] || "ok"}`, text: PILL[kind][verdict] || verdict}) : null),
            el("p", {text: headline}), ...more);
          const parts = [el("h3", {text: "Pay and cost of living"}), el("p", {class: "cost-summary", text: c.summary})];
          if (c.pay) {
            const range = c.pay.market_mid ? el("p", {class: "cost-meta", text: `Typical for this role here: ${money(c.pay.market_low)} to ${money(c.pay.market_high)}`
              + (c.pay.listed ? `. Posted: ${money(c.pay.min)}${c.pay.max !== c.pay.min ? ` to ${money(c.pay.max)}` : ""}.` : ".")}) : null;
            const desired = c.pay.meets_desired === null || c.pay.meets_desired === undefined ? null
              : el("p", {class: "cost-meta", text: c.pay.meets_desired ? "This meets your minimum desired pay." : "This tops out below your minimum desired pay."});
            parts.push(card("Is the pay competitive?", "pay", c.pay.verdict, c.pay.headline, range, desired));
          }
          const b = c.benefits;
          parts.push(card("Are the benefits good?", "benefits", b.rating, b.headline, b.found.length ? chips(b.found) : null));
          if (c.lifestyle) {
            const l = c.lifestyle;
            //the pay over each stretch of time, before and after tax
            const cell = (text, tag = "td") => el(tag, {text});
            const breakdown = el("table", {class: "cost-table cost-periods"},
              el("caption", {class: "visually-hidden", text: "Pay by period, before and after tax"}),
              el("thead", {}, el("tr", {}, cell("", "th"), cell("Before tax", "th"), cell("After tax", "th"))),
              el("tbody", {}, ...l.periods.map(x => el("tr", {}, cell(x.label), cell(money(x.gross)), cell(money(x.net))))));
            const basis = c.pay?.source === "posting" ? "the posted pay (midpoint of the range)" : "an estimated salary";
            parts.push(card("What does the pay come to?", null, null, `Based on ${basis}: ${money(l.gross)} a year. About ${l.tax_rate}% goes to tax (federal, state and local, Social Security and Medicare), leaving ${money(l.periods[0].net)}.`, breakdown));
            const row = (name, amount, cls = "") => el("tr", {class: cls}, el("td", {text: name}), el("td", {text: money(amount)}));
            const table = el("table", {class: "cost-table"},
              el("caption", {class: "visually-hidden", text: `Monthly budget in ${c.location.label}`}),
              el("tbody", {}, row("Pay per month, before tax", l.gross_monthly, "cost-gross"),
                row(`Expected taxes (${l.tax_rate}%)`, -l.tax_monthly),
                row("Take-home pay per month", l.net_monthly, "cost-net"),
                ...l.costs.map(x => row(x.label, -x.amount)),
                row(l.left_monthly >= 0 ? "Left over" : "Short by", Math.abs(l.left_monthly), "cost-left")));
            const taxes = el("p", {class: "cost-meta", text: `Yearly taxes: about ${money(l.federal)} federal, ${money(l.state)} state and local, and ${money(l.payroll)} Social Security and Medicare. Rent is about ${l.rent_share}% of gross pay.`});
            parts.push(card(`Can you live well in ${c.location.label}?`, "life", l.verdict, l.headline, table, taxes));
          }
          for (const note of c.notes) parts.push(el("p", {class: "note", text: note}));
          parts.push(el("p", {class: "cost-meta", text: "Estimates from average figures for one person with a modest budget, not live prices. Taxes are simplified. Use them to compare, then check the real numbers."}));
          if (Array.isArray(c.sources) && c.sources.length) {
            const freshness = [c.data_updated_at ? `Updated ${c.data_updated_at}` : "",
              c.refresh_days ? `Refreshes automatically every ${c.refresh_days} days` : "",
              c.next_update_at ? `next check ${c.next_update_at}` : ""].filter(Boolean).join(" · ");
            const items = c.sources.map(src => {
              const href = safeURL(src.url);
              const link = href
                ? el("a", {href, target: "_blank", rel: "noopener noreferrer", text: src.name})
                : el("strong", {text: src.name});
              return el("li", {}, link, src.description ? ` — ${src.description}` : "");
            });
            parts.push(el("details", {class: "cost-sources"},
              el("summary", {text: freshness ? `Data sources (${freshness})` : "Data sources"}),
              el("ul", {class: "cost-source-list"}, items)));
          }
          fill(box, ...parts);
          box.hidden = false;
        } catch {
          //the rest of the listing is unaffected; the analysis is simply not shown
        }
      };

      const PASTE_SPEC = [
        {name: "company_name", label: "Company", required: true},
        {name: "position_title", label: "Title", required: true},
        {name: "location", label: "Location"},
        {name: "work_mode", label: "Work mode", type: "select",
         options: [["", "Not stated"], ["Remote", "Remote"], ["Hybrid", "Hybrid"], ["On-site", "On-site"]]},
        {name: "listing_url", label: "Link to the listing", type: "url"},
        {name: "company_url", label: "Company website", type: "url"},
      ];

      $("paste-preview").addEventListener("click", async () => {
        const {el, fill, chips, makeForm} = window.ResumeBuilder;
        const text = $("paste-text").value;
        const rawLink = ($("paste-url")?.value || "").trim();
        const listingUrl = rawLink ? normalizeJobLink(rawLink) : "";
        try {
          const parsed = await jobsApi.parse(text);
          const initial = {...parsed.inferred, ...(listingUrl ? {listing_url: listingUrl} : {})};
          const form = makeForm(PASTE_SPEC, initial, {idPrefix: "paste"});
          const skills = (label, list) => list.length ? [el("p", {text: label}), chips(list)] : null;

          const shareBox = el("input", {type: "checkbox", checked: Boolean(window.Prefs?.get("communityShare"))});
          const shareRow = el("div", {class: "paste-share-row"},
            el("label", {}, shareBox, "Share with the Fistbump community"),
            el("p", {class: "hint", text: "Contributes this listing (link, company, title, location, and scrubbed text) to Community resources once reviewed. Nothing personal is sent."}));

          const save = el("button", {class: "primary", type: "button", text: "Save job", onclick: async () => {
            // drop blank fields (read() turns them into null)
            const body = Object.fromEntries(Object.entries(form.read()).filter(([, value]) => value !== null));
            try {
              const job = await jobsApi.create({...body, source: "pasted", raw_text: text});
              if (shareBox.checked && window.Community?.shareListing) {
                window.Community.shareListing({...body, raw_text: text}).catch(() => {});
              }
              //saved once: the button says so instead of offering to save the same posting again
              save.disabled = true;
              save.textContent = "Saved";
              if ($("paste-url")) $("paste-url").value = "";
              $("paste-text").value = "";
              notifyJobsChanged();
              window.JobPage.open(job.id);
            } catch (error) {
              form.showError(error);
            }
          }});

          fill($("paste-result"),
            parsed.hidden_text && el("p", {class: "note", text: "Hidden text was removed from this posting."}),
            form.el,
            skills("Required skills", [...parsed.req_tech_skills, ...parsed.req_soft_skills]),
            skills("Preferred skills", [...parsed.pref_tech_skills, ...parsed.pref_soft_skills]),
            shareRow,
            el("div", {class: "job-actions paste-save-actions"}, save));
        } catch (error) {
          const {el, fill} = window.ResumeBuilder;
          fill($("paste-result"), el("p", {class: "note", text: error.message}));
        }
      });

      //The filters row opens from the toolbar. Its button counts the filters in use, so they are not forgotten.
      const FILTER_IDS = ["search-company", "search-source", "search-include-community", "search-industry", "search-work-mode", "search-posted",
        "search-pay-min", "search-pay-max", "search-exclude", "search-require-pay", "search-min-fit", "search-max-missing",
        "search-min-pay-rating", "search-min-benefits", "search-min-living"];
      const filterInUse = id => {
        const field = $(id);
        //With the resume off, the fit filters do not apply, so they do not count as in use.
        if ((id === "search-min-fit" || id === "search-max-missing") && !$("search-use-resume").checked) return false;
        if (/^search-min-(pay-rating|benefits|living)$/.test(id) && !$("search-use-ratings").checked) return false;
        if (field.type === "checkbox") return field.checked;
        const value = field.value.trim();
        return value !== "" && value !== "Any" && value !== "0";
      };
      let shownCount = 0;
      const updateFilterCount = () => {
        const count = FILTER_IDS.filter(filterInUse).length;
        if (count !== shownCount && count > 0) window.UI.play($("filters-count"), "pop", 420); //the badge answers each change
        shownCount = count;
        $("filters-count").hidden = count === 0;
        $("filters-count").textContent = String(count);
        $("filters-toggle").title = count ? (count === 1 ? "1 filter in use" : `${count} filters in use`) : "Show or hide search filters";
      };
      //The filters open as a drawer under the search bar; motion.css grows and shrinks it (see #filters there).
      const setFiltersOpen = open => {
        $("filters").hidden = !open;
        $("filters-toggle").setAttribute("aria-expanded", String(open));
      };
      $("filters-toggle").addEventListener("click", () => setFiltersOpen($("filters").hidden));
      document.addEventListener("pointerdown", event => {
        if ($("filters").hidden) return;
        const t = event.target;
        if ($("filters").contains(t) || $("filters-toggle").contains(t)) return;
        setFiltersOpen(false);
      });
      //an industry on its own is a complete search, so the empty page points straight at it
      $("browse-industry").addEventListener("click", () => {
        setFiltersOpen(true);
        $("search-industry").focus();
      });
      searchForm.addEventListener("input", updateFilterCount);
      searchForm.addEventListener("change", updateFilterCount);

      //industries come from the backend's curated lists; the form still works without them
      let boardTotal = 0; //how many boards an unfiltered search reads, from the categories route
      const companyNames = []; //filled with the curated company names for autocomplete
      window.FistBumpAPI.categories().then(data => {
        companyNames.push(...(data.companies || []));
        boardTotal = data.board_total || 0;
        for (const category of data.categories) {
          const option = el("option", `${category.parent ? "\u00a0\u00a0" : ""}${category.name} (${category.board_count})`);
          option.value = category.id;
          $("search-industry").append(option);
        }
        try {
          //the saved industry could not be chosen until its option existed
          const saved = JSON.parse(localStorage.getItem(FORM_KEY) || "{}")["search-industry"];
          if (saved) $("search-industry").value = saved;
          updateFilterCount();
        } catch {
          //nothing saved
        }
      }).catch(() => {});

      //the search form is kept between launches; pay and word boxes are text, the rest are selects and a checkbox
      const FORM_IDS = ["search-keywords", "search-location", "search-company", "search-source", "search-include-community", "search-work-mode", "search-industry",
        "search-posted", "search-pay-min", "search-pay-max", "search-exclude", "search-require-pay", "search-use-resume",
        "search-min-fit", "search-max-missing", "search-use-ratings", "search-min-pay-rating", "search-min-benefits", "search-min-living"];
      const FORM_KEY = "fistbump.search-form";

      //the form as an object, and back
      const readFields = () => {
        const fields = {};
        for (const id of FORM_IDS) fields[id] = $(id).type === "checkbox" ? $(id).checked : $(id).value;
        return fields;
      };
      const applyFields = fields => {
        for (const id of FORM_IDS) {
          if (!(id in fields)) continue;
          if ($(id).type === "checkbox") $(id).checked = fields[id] === true;
          else $(id).value = String(fields[id]);
        }
        applyResumeSwitch();
        applyRatingsSwitch();
        updateFilterCount();
      };

      //The resume switch turns the fit filters and the skill ranking off. The two boxes it holds
      //are greyed out then, and their values are not sent.
      const applyResumeSwitch = () => {
        const on = $("search-use-resume").checked;
        $("search-min-fit").disabled = !on;
        $("search-max-missing").disabled = !on;
        updateFilterCount();
      };
      $("search-use-resume").addEventListener("change", applyResumeSwitch);

      //The ratings switch does the same for the three rating filters. Sorting by a part that is switched off is
      //not possible, so those sort choices are greyed out and the sort goes back to Best match.
      const applyRatingsSwitch = () => {
        const on = $("search-use-ratings").checked;
        for (const id of ["search-min-pay-rating", "search-min-benefits", "search-min-living"]) $(id).disabled = !on;
        applySortChoices();
        updateFilterCount();
      };
      const applySortChoices = () => {
        const resume = $("search-use-resume").checked;
        const ratings = $("search-use-ratings").checked;
        for (const option of $("result-sort").options) {
          option.disabled = (option.value === "fit" && !resume) || (["pay", "benefits", "living", "overall"].includes(option.value) && !ratings);
        }
        if ($("result-sort").selectedOptions[0]?.disabled) $("result-sort").value = "relevance";
      };
      $("search-use-ratings").addEventListener("change", applyRatingsSwitch);
      $("search-use-resume").addEventListener("change", applySortChoices);
      applyResumeSwitch();
      applyRatingsSwitch();

      const saveForm = () => {
        try {
          localStorage.setItem(FORM_KEY, JSON.stringify(readFields()));
        } catch {
          //storage can be unavailable; the form just starts empty next time
        }
      };
      const restoreForm = () => {
        try {
          applyFields(JSON.parse(localStorage.getItem(FORM_KEY) || "{}"));
        } catch {
          //a damaged saved form is ignored
        }
      };

      //pay is typed as 90000, 90,000, $90k or 90K; a blank or unreadable box means no limit
      const parsePay = text => {
        const match = String(text).replace(/[$,\s]/g, "").match(/^(\d+(?:\.\d+)?)(k?)$/i);
        return match ? Math.round(parseFloat(match[1]) * (match[2] ? 1000 : 1)) : 0;
      };
      const payLabel = amount => amount >= 1000 ? `$${Math.round(amount / 1000)}k` : `$${amount}`;

      //Recent searches: the form fields of each search that ran, newest first, kept on this computer only.
      const HISTORY_KEY = "fistbump.search-history";
      const HISTORY_MAX = 15;
      const loadHistory = () => {
        try {
          const list = JSON.parse(localStorage.getItem(HISTORY_KEY) || "[]");
          return Array.isArray(list) ? list.filter(entry => entry && typeof entry.fields === "object" && entry.fields) : [];
        } catch {
          return [];
        }
      };
      const storeHistory = list => {
        try {
          if (list.length) localStorage.setItem(HISTORY_KEY, JSON.stringify(list));
          else localStorage.removeItem(HISTORY_KEY);
        } catch {
          //history is a convenience; losing it is fine
        }
      };

      //Describes a search in full: a title (what was searched for) and every option that narrowed it.
      //The industry's name is kept with the entry, since the select may not be filled when the list is drawn.
      //keywords as a clean, comma-separated line
      const tidy = value => String(value ?? "").split(",").map(w => w.trim()).filter(Boolean).join(", ");
      const describeFields = (fields, industryName = "") => {
        const text = id => id === "search-keywords" ? tidy(fields[id]) : String(fields[id] ?? "").trim();
        const optionText = (id, value) => ($(id).querySelector(`option[value="${CSS.escape(value)}"]`)?.textContent || "").trim();
        const industry = text("search-industry")
          ? (industryName || optionText("search-industry", text("search-industry")).replace(/\s*\(\d+\)$/, "")).trim()
          : "";
        const place = text("search-location");
        const company = text("search-company");

        const src = text("search-source");
        const srcName = src ? (SOURCE_NAMES[src] || src) : "";
        const title = text("search-keywords") || (company && `Jobs at ${company}`) || industry || (place && `Jobs in ${place}`) || (srcName && `${srcName} jobs`) || "All jobs";
        const details = [];
        if (text("search-keywords") && place) details.push(`in ${place}`);
        else if (!text("search-keywords") && (company || industry) && place) details.push(`in ${place}`);
        if (text("search-keywords") && company) details.push(`at ${company}`);
        if (srcName && title !== `${srcName} jobs`) details.push(`on ${srcName}`);
        if (industry && title !== industry) details.push(industry);
        if (text("search-work-mode") && text("search-work-mode") !== "Any") details.push(`${text("search-work-mode")} only`);
        if (text("search-posted") && text("search-posted") !== "0") {
          details.push(optionText("search-posted", text("search-posted")).toLowerCase());
        }
        const min = parsePay(text("search-pay-min"));
        const max = parsePay(text("search-pay-max"));
        if (min && max) details.push(`pay ${payLabel(min)} to ${payLabel(max)}`);
        else if (min) details.push(`pay at least ${payLabel(min)}`);
        else if (max) details.push(`pay up to ${payLabel(max)}`);
        if (fields["search-require-pay"] === true) details.push("only postings that list pay");
        const exclude = text("search-exclude");
        if (exclude) details.push(`leaving out ${exclude}`);
        //With the resume off, the fit is not part of the search, so it is not part of the summary.
        if ($("search-use-resume").checked) {
          if (text("search-min-fit") && text("search-min-fit") !== "0") details.push(`fit ${optionText("search-min-fit", text("search-min-fit")).toLowerCase()}`);
          if (text("search-max-missing") !== "" && text("search-max-missing") !== undefined) details.push(text("search-max-missing") === "0" ? "no missing skills" : `${optionText("search-max-missing", text("search-max-missing")).toLowerCase()} missing skills`);
        }
        if ($("search-use-ratings").checked) {
          for (const [id, label] of [["search-min-pay-rating", "pay"], ["search-min-benefits", "benefits"], ["search-min-living", "cost of living"]]) {
            if (text(id)) details.push(`${label} ${text(id) === "good" ? "good" : "fair or better"}`);
          }
        }
        return {title, details};
      };

      const sameSearch = (a, b) => FORM_IDS.every(id => String(a[id] ?? "") === String(b[id] ?? ""));

      //the history menu opens over the page and closes when the user picks something or clicks elsewhere
      const setHistoryOpen = open => {
        $("history").hidden = !open;
        $("history-toggle").setAttribute("aria-expanded", String(open));
      };
      $("history-close").addEventListener("click", () => {
        setHistoryOpen(false);
        $("history-toggle").focus();
      });
      $("history-toggle").addEventListener("click", () => setHistoryOpen($("history").hidden));
      document.addEventListener("mousedown", event => {
        if (!$("history").hidden && !$("history").parentElement.contains(event.target)) setHistoryOpen(false);
      });
      document.addEventListener("keydown", event => {
        if (event.key === "Escape" && !$("history").hidden && !event.defaultPrevented) {
          event.preventDefault();
          setHistoryOpen(false);
          $("history-toggle").focus();
        }
      });

      const renderHistory = () => {
        const list = loadHistory();
        $("history-toggle").disabled = list.length === 0;
        $("history-toggle").title = list.length ? "Recent searches" : "No recent searches yet";
        if (list.length === 0) setHistoryOpen(false);
        $("history-list").replaceChildren(...list.map((entry, index) => {
          const item = document.createElement("li");
          const run = document.createElement("button");
          run.type = "button";
          run.className = "history-run";
          const {title, details} = describeFields(entry.fields, entry.industry);
          const label = document.createElement("span");
          label.className = "history-label";
          label.append(el("strong", title));
          if (details.length) label.append(el("small", details.join(" \u00b7 ")));
          run.append(label, el("span", ago(entry.at).replace("Updated ", "")));
          run.title = [title, ...details].join(", ");
          run.addEventListener("click", () => {
            if (searchButton.disabled) return;
            setHistoryOpen(false);
            applyFields(entry.fields);
            saveForm();
            searchForm.requestSubmit();
          });
          const remove = el("button", "Remove");
          remove.type = "button";
          remove.className = "history-remove";
          remove.setAttribute("aria-label", `Remove "${title}" from history`);
          remove.addEventListener("click", () => {
            storeHistory(loadHistory().filter((_, i) => i !== index));
            renderHistory();
          });
          item.append(run, remove);
          return item;
        }));
        window.dispatchEvent(new Event("fistbump:history"));
      };

      //called when a search starts; a search already in the list moves to the top
      const recordSearch = () => {
        const fields = readFields();
        const list = loadHistory().filter(entry => !sameSearch(entry.fields, fields));
        const industry = fields["search-industry"]
          ? ($("search-industry").selectedOptions[0]?.textContent || "").replace(/\s*\(\d+\)$/, "").trim()
          : "";
        list.unshift({fields, industry, at: new Date().toISOString()});
        storeHistory(list.slice(0, HISTORY_MAX));
        renderHistory();
      };

      //Clear history also forgets the recent entries each box suggests
      $("clear-history").addEventListener("click", () => window.JobParser.clearHistory());

      // ---- Pinned (saved) searches with "New since last check" badges ----
      let savedSearches = [];
      let activePinnedId = null;
      let activePinnedSeen = null;

      const renderSavedSearches = () => {
        const section = $("saved-searches-section");
        const list = $("saved-searches-list");
        if (!section || !list) return;
        section.hidden = savedSearches.length === 0;
        if (savedSearches.length > 0) $("history-toggle").disabled = false;
        list.replaceChildren(...savedSearches.map(entry => {
          const item = document.createElement("li");
          const run = document.createElement("button");
          run.type = "button";
          run.className = "history-run";
          const label = document.createElement("span");
          label.className = "history-label";
          label.append(el("strong", entry.name));
          if (entry.last_run_at) label.append(el("small", `Checked ${ago(entry.last_run_at).replace("Updated ", "") || "recently"}`));
          run.append(label);
          run.addEventListener("click", () => {
            if (searchButton.disabled) return;
            setHistoryOpen(false);
            let fields = {};
            try { fields = JSON.parse(entry.query_json || "{}"); } catch { fields = {}; }
            activePinnedId = entry.id;
            activePinnedSeen = new Set(Array.isArray(entry.seen_keys) ? entry.seen_keys : []);
            applyFields(fields);
            saveForm();
            searchForm.requestSubmit();
          });
          const remove = el("button", "Unpin");
          remove.type = "button";
          remove.className = "history-remove";
          remove.setAttribute("aria-label", `Remove pinned search "${entry.name}"`);
          remove.addEventListener("click", async () => {
            try {
              await window.FistBumpAPI.savedSearches.delete(entry.id);
              if (activePinnedId === entry.id) { activePinnedId = null; activePinnedSeen = null; }
              await loadSavedSearches();
            } catch (err) { window.UI.alert(err.message); }
          });
          item.append(run, remove);
          return item;
        }));
      };

      const loadSavedSearches = async () => {
        if (!window.api?.savedSearches?.list) return;
        try {
          savedSearches = await window.FistBumpAPI.savedSearches.list();
          renderSavedSearches();
        } catch {
          // saved searches optional if backend is not running
        }
      };

      $("pin-search")?.addEventListener("click", async () => {
        if (!window.api?.savedSearches?.create) return;
        const fields = readFields();
        const hasCriteria = Boolean(
          String(fields["search-keywords"] || "").trim() ||
          String(fields["search-location"] || "").trim() ||
          String(fields["search-company"] || "").trim() ||
          String(fields["search-industry"] || "").trim() ||
          String(fields["search-source"] || "").trim()
        );
        if (!hasCriteria) {
          window.UI.alert("Enter keywords, a location, a company, or a filter first before saving a search.", "Nothing to save");
          return;
        }
        const {title} = describeFields(fields);
        const seenKeys = (results?.postings || []).map(p => keyOf(p));
        try {
          await window.FistBumpAPI.savedSearches.create({
            name: title,
            query_json: JSON.stringify(fields),
            seen_keys: seenKeys,
          });
          await loadSavedSearches();
          setHistoryOpen(true);
        } catch (err) {
          window.UI.alert(err.message, "Could not save search");
        }
      });

      // When a pinned search finishes, persist the updated seen keys for the next run.
      const origRenderResults = renderResults;
      const syncPinnedSeen = data => {
        if (!activePinnedId || !window.api?.savedSearches?.markSeen) return;
        const merged = new Set([...(activePinnedSeen || []), ...(data.postings || []).map(p => keyOf(p))]);
        const id = activePinnedId;
        window.FistBumpAPI.savedSearches.markSeen(id, [...merged]).then(() => loadSavedSearches()).catch(() => {});
      };
      void origRenderResults;
      void syncPinnedSeen;

      restoreForm();
      renderHistory();
      loadSavedSearches();
      searchForm.addEventListener("change", saveForm);

      //The "show under each result" boxes act at once, on the results already on screen.
      syncShowBoxes();
      document.addEventListener("change", event => {
        const box = event.target.closest?.("input[data-show]");
        if (!box) return;
        show = {...show, [box.dataset.show]: box.checked};
        try { localStorage.setItem(SHOW_KEY, JSON.stringify(show)); } catch { /* the choice lasts until the app closes */ }
        if (results) renderResults(results);
        if (activeTab === "saved") loadSaved();
      });

      $("clear-search").addEventListener("click", () => {
        searchForm.reset();
        syncShowBoxes(); //reset clears every box in the form; the display choices are not part of the search
        try { localStorage.removeItem(FORM_KEY); } catch { /* nothing saved to remove */ }
        updateFilterCount();
        $("search-keywords").focus();
      });

      $("cancel-search").addEventListener("click", () => window.FistBumpAPI.cancelSearch());

      $("result-sort").addEventListener("change", () => {
        if (results) runSearch({...searchedQuery, sort: $("result-sort").value});
      });

      window.Suggest.attach($("search-company"), {key: "company", fixed: companyNames});
      window.Suggest.attach($("search-keywords"), {key: "keywords", fixed: window.Suggest.TITLES, list: true});
      window.Suggest.attach($("search-location"), {key: "location", fixed: window.Suggest.PLACES});

      //Keyboard shortcuts: Cmd/Ctrl+F to search and Cmd/Ctrl+, for settings; Esc closes the filters.
      //Cmd/Ctrl+1 to 5 belong to the whole app (app.js) and are not handled here.
      document.addEventListener("keydown", event => {
        const mod = window.api?.platform === "darwin" ? event.metaKey : event.ctrlKey;
        if (mod && !event.shiftKey && !event.altKey) {
          if (main.hidden) {
            if (event.key === ",") {
              event.preventDefault();
              location.hash = "#/jobs";
              selectTab("settings");
            }
            return;
          }
          if (event.key === "f") {
            event.preventDefault();
            const box = $("search-keywords");
            box.focus();
            box.select();
          } else if (event.key === ",") {
            event.preventDefault();
            selectTab("settings");
          }
        } else if (event.key === "Escape" && !event.defaultPrevented && !$("filters").hidden) {
          setFiltersOpen(false);
          $("filters-toggle").focus();
        }
      });

      //What the settings page needs from this view.
      window.JobParser.selectTab = selectTab;
      window.JobParser.historyCount = () => loadHistory().length;
      window.JobParser.clearHistory = () => {
        storeHistory([]);
        window.Suggest.clearRecent();
        try { localStorage.removeItem(LISTING_KEY); } catch { /* nothing saved to remove */ }
        renderHistory();
      };

      //What Home needs: run a recent search again, or reopen the last listing.
      window.JobParser.runSearch = fields => {
        selectTab("find");
        if (searchButton.disabled) return;
        applyFields(fields);
        saveForm();
        searchForm.requestSubmit();
      };
      window.JobParser.openListing = saved => {
        selectTab("find");
        openSaved(saved);
      };
      window.JobParser.resetForm = () => $("clear-search").click();

      //A new page size or sort applies from the next search; the sort select starts at the saved default.
      $("result-sort").value = window.Prefs.get("defaultSort");
      window.Prefs.onChange(key => {
        if (key === "defaultSort" && !results) $("result-sort").value = window.Prefs.get("defaultSort");
        if (key === "showLogos" && results) renderResults(results);
      });

      showDetailEmpty();
      selectTab("find");
      loadSavedIndex();
    }
  };
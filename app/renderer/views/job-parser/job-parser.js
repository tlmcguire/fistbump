window.JobParser = {
    mount() {
      const form = document.getElementById("job-form");
      const input = document.getElementById("job-url");
      const loadButton = form.querySelector('button[type="submit"]');
      const states = ["initial", "loading", "success", "error", "results"];
      let posting = null;

      //phase 2 setup
      let retry = null;
      const $ = id => document.getElementById(id);
      const searchForm = $("search-form");
      const searchButton = searchForm.querySelector('button[type="submit"]');
      const tabs = {find: $("tab-find"), link: $("tab-link")};
      const panels = {find: $("panel-find"), link: $("panel-load-listing")};
      let activeTab = "find";
      let results = null;
      let fromResults = false; //true while a listing opened from the result list is on screen
      let searchedQuery = {};
      const PAGE_SIZE = 20; //results requested per page
      const MAX_PAGES = 250; //the backend's offset limit (5000) divided by PAGE_SIZE
      let pages = new Map(); //page number -> the backend's response for it
      let currentPage = 1;

  
      //gets each section's state and shows it accordingly.
      const showState = state => {
        states.forEach(name => {
          document.getElementById(`state-${name}`).hidden = name !== state;
        });
      };

      //shows the loading section and assigns the message
      const showLoading = (message, cancelable = false) => {
        $("loading-message").textContent = message;
        $("cancel-search").hidden = !cancelable;
        showState("loading");
      };

      //locks buttons and tabs during a request
      const setBusy = busy => {
        loadButton.disabled = busy;
        searchButton.disabled = busy;
        tabs.find.disabled = busy;
      };

      //tab switch
      const selectTab = name => {
        activeTab = name;
        for (const key in tabs){
          tabs[key].setAttribute("aria-selected", key === name);
          panels[key].hidden=key!==name;
        }
        if (name==="find" && results){
          showState("results");
        } else{
          let messageText = "";
          if (name === "find"){
            messageText = "Enter keywords and press Search to find jobs.";
          } else {
            messageText = "Enter a Greenhouse job URL to view the listing.";

          }
          $("initial-message").textContent = messageText;
          showState("initial");
        }
      };
  
      //Puts an error message on the page. 
      const showError = error => {
        document.getElementById("error-message").textContent =
          error.message || "An unknown error has occurred";
        showState("error");
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
  
      //Sets a link
      const setLink = (id, value) => {
        const element = document.getElementById(id);
        const url = value ? safeURL(value) : null;
        element.hidden = !url;
  
        if (url) {
          element.href = url;
        } else {
          element.removeAttribute("href");
        }
      };
  
      //fill page with job data
      const displayPosting = (data, opened = false) => {
        posting = data;
        fromResults = opened;
        $("back-to-results").hidden = !(opened && results);
  
        document.getElementById("job-title").textContent =
          data.position_title || "Unknown position";
        document.getElementById("job-company").textContent =
          data.company_name || "Unknown company";
        document.getElementById("job-location").textContent =
          data.location || "Unknown job location";
        document.getElementById("job-updated").textContent =
          data.updated_at
            ? `Last updated: ${new Date(data.updated_at).toLocaleString()}`
            : "";
  
        document.getElementById("job-description").textContent =
          data.raw_text || "Unknown description";
  
        fillLogo($("job-logo"), data.company_name, data.company_url, data.board);
        setLink("job-link", data.listing_url);
        setLink("company-link", data.company_url);
  
        showState("success");
      };
  
      //edited 10/22 for new state handling system
      const loadPosting = async url => {
        retry = () => loadPosting(url);
        setBusy(true);
        showLoading("Loading job listing...");
  
        try {
          displayPosting(await window.FistBumpAPI.link(url));
        } catch (error) {
          showError(error);
        } finally {
          setBusy(false);
        }
      };

      //refreshes an existing job listing
      const reloadPosting = async() => {
        if (!posting) return;
        retry = reloadPosting;
        setBusy(true);
        showLoading("Reloading job listing...")
          
        try {
          displayPosting(await window.FistBumpAPI.posting(posting.board, posting.external_id), fromResults);
        } catch (error) {
          showError(error);
        } finally {
          setBusy(false);
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
        if (!url && !board) return paint(null);
        const entry = logoEntry(url || "", board);
        if (entry.done) return paint(entry);
        tile.classList.add("pending"); //blank until the answer arrives
        entry.promise.then(done => {
          if (!tile.isConnected) return;
          paint(done);
          tile.classList.add("reveal");
        });
      };

      //get one row of the search results and define it's html
      const resultItem = result => {
        const item = document.createElement("li");
        item.className = "result";
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
              result.location,
              result.work_mode,
              payText(result),
              ago(result.updated_at)
            ].filter(Boolean).join(" . "))
          );

          open.addEventListener("click", () => displayPosting(result, true));
          body.append(open);

          const preview = snippet(result.raw_text);
          if (preview) body.append(el("p", preview));
          if (body.lastChild.tagName === "P") body.lastChild.className = "result-snippet";
          if (result.duplicates) {
            body.append(el("small", `Also posted in ${result.duplicates} other ${result.duplicates === 1 ? "location" : "locations"}`));
          }

          const url = safeURL(result.company_url);
          if (url) {
            const link = el("a", "Company site");
            link.className = "result-link";
            link.href = url;
            link.target = "_blank";
            link.rel = "noopener noreferrer";
            body.append(link);
          }
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

        const company = query.company;
        if (company && !(data.companies || []).length) {
          //nothing to search: say so plainly instead of showing an empty list
          return {
            note: "",
            empty: `We can't find listings for "${company}" yet. We search a fixed list of companies that use Greenhouse, and this one is not on it or does not post there. Check the spelling, or try searching by keywords or industry instead.`
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
          notes.push("A pay range needs each description, so only the top matches were checked. Narrow the search to see more.");
        }
        if (data.boards_searched < data.boards_total) {
          const missed = data.boards_total - data.boards_searched;
          notes.push(`${missed.toLocaleString()} of ${data.boards_total.toLocaleString()} boards could not be searched, so some postings may be missing.`);
        }
        if (data.postings.length === 0 && data.matched > 0) {
          empty = "Matches were found, but their descriptions could not be loaded. Try again.";
        }
        return {note: notes.join(" "), empty};
      };

      //fill all results
      const renderResults = data => {
        $("result-counts").textContent = 
        `${data.matched.toLocaleString()}${data.partial ? "+" : ""} matches from ${data.postings_scanned.toLocaleString()} postings on ${data.boards_searched.toLocaleString()} boards`;
        const {note, empty} = describeSearch(data, searchedQuery);
        $("result-note").hidden = !note;
        $("result-note").textContent = note;
        $("result-empty").textContent = empty;
        $("result-empty").hidden = data.postings.length > 0;
        $("result-list").replaceChildren(...data.postings.map(resultItem));

        renderPager();

        $("result-errors").hidden = data.errors.length === 0;
        $("result-errors-list").replaceChildren(
          ...data.errors.map(failed => el("li", `${failed.board}: ${failed.message}`))
        );
      };

      let searchSeq = 0; //counts searches, so an older one that finishes late is ignored

      //main function to run the search
      const runSearch = async query => {
        retry = () => runSearch(query);
        const previous = {pages, currentPage, results, searchedQuery};
        const mine = ++searchSeq;
        searchedQuery = query;
        results = null;
        setBusy(true);
        const wide = !query.company && !query.categories.length && boardTotal;
        showLoading(wide ? `Searching ${boardTotal.toLocaleString()} boards...` : "Searching Greenhouse...", true);

        try {
          const data = await window.FistBumpAPI.search({...query, offset: 0});
          if (mine !== searchSeq) return; //a newer search owns the page now
          await preloadLogos(data.postings);
          if (mine !== searchSeq) return;
          pages = new Map([[1, data]]);
          currentPage = 1;
          results = data;
          $("result-sort").value = query.sort || "relevance";
          renderResults(data);
          showState("results");
        } catch (error) {
          if (mine !== searchSeq) return;
          if (error.message === "Search canceled") {
            //put back what was on screen before this search
            ({pages, currentPage, results, searchedQuery} = previous);
            selectTab("find");
            return;
          }
          showError(error);
        } finally {
          if (mine === searchSeq) setBusy(false);
        }
      };

      //Pages are numbered from 1. A page is fetched once, then kept until the next search.
      const pageCount = () => Math.min(Math.ceil((results?.matched || 0) / PAGE_SIZE), MAX_PAGES);

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
            n === null ? el("span", "...") : button(String(n), n, {current: n === currentPage})),
          button("Next", currentPage + 1, {disabled: currentPage === total})
        );
      };

      //shows a page of results, asking the backend only if the page is not kept yet
      let pageSeq = 0; //counts page changes, so a slow one cannot land after a newer one
      const goToPage = async page => {
        if (page < 1 || page > pageCount()) return;
        const mine = ++pageSeq;
        retry = () => goToPage(page);
        let data = pages.get(page);

        if (!data) {
          setBusy(true);
          $("pager").querySelectorAll("button").forEach(node => { node.disabled = true; });
          try {
            data = await window.FistBumpAPI.search({...searchedQuery, offset: (page - 1) * PAGE_SIZE});
          } catch (error) {
            if (error.message !== "Search canceled") showError(error);
            return;
          } finally {
            setBusy(false);
          }
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
        showState("results");
        $("result-counts").scrollIntoView({block: "start"});
      };

      //rest of script is state handling and button events (updated 10/22 for tabs and new retry system replacing lastUrl)
      tabs.find.addEventListener("click", () => selectTab("find"));
      tabs.link.addEventListener("click", () => selectTab("link"));

      searchForm.addEventListener("submit", event => {
        event.preventDefault();

        const words = id => $(id).value.split(",").map(word => word.trim()).filter(Boolean);
        const number = id => Math.max(0, parseInt($(id).value, 10) || 0);
        const pay = id => parsePay($(id).value);
        const query = {
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
          sort: $("result-sort").value,
          limit: PAGE_SIZE
        };
        for (const id of ["search-pay-min", "search-pay-max"]) {
          if ($(id).value.trim() && !pay(id)) {
            $("initial-message").textContent = "Enter pay as a yearly amount, such as 90000 or 90k.";
            showState("initial");
            return;
          }
        }
        if (!query.keywords.length && !query.location && !query.categories.length && !query.company) {
          $("initial-message").textContent = "Enter keywords, a location, a company or an industry to search.";
          showState("initial");
          return;
        }
        saveForm();
        recordSearch();
        window.Suggest.remember("keywords", query.keywords);
        window.Suggest.remember("location", [query.location]);
        window.Suggest.remember("company", [query.company]);
        runSearch(query);
      });

      form.addEventListener("submit", event => {
        event.preventDefault();
        const url = input.value.trim();
  
        if (url) {
          loadPosting(url);
        }
      });

      $("back-to-results").addEventListener("click", () => showState("results"));

      document.getElementById("reload-listing").addEventListener("click", reloadPosting);
  
      document.getElementById("try-again").addEventListener("click", async () => {
        if (retry){
          retry();
        } else {
          selectTab(activeTab);
        }
      });

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
        } catch {
          //nothing saved
        }
      }).catch(() => {});

      //the search form is kept between launches; pay and word boxes are text, the rest are selects and a checkbox
      const FORM_IDS = ["search-keywords", "search-location", "search-company", "search-work-mode", "search-industry",
        "search-posted", "search-pay-min", "search-pay-max", "search-exclude", "search-require-pay"];
      const FORM_KEY = "fistbump.search-form";
      const OPTION_IDS = ["search-posted", "search-pay-min", "search-pay-max", "search-exclude", "search-require-pay"];

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
        $("more-options").open = OPTION_IDS.some(id =>
          $(id).type === "checkbox" ? $(id).checked : $(id).value && $(id).value !== "Any" && $(id).value !== "0");
      };

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
      const describeFields = (fields, industryName = "") => {
        const text = id => String(fields[id] ?? "").trim();
        const optionText = (id, value) => ($(id).querySelector(`option[value="${CSS.escape(value)}"]`)?.textContent || "").trim();
        const industry = text("search-industry")
          ? (industryName || optionText("search-industry", text("search-industry")).replace(/\s*\(\d+\)$/, "")).trim()
          : "";
        const place = text("search-location");
        const company = text("search-company");

        const title = text("search-keywords") || (company && `Jobs at ${company}`) || industry || (place && `Jobs in ${place}`) || "All jobs";
        const details = [];
        if (text("search-keywords") && place) details.push(`in ${place}`);
        else if (!text("search-keywords") && (company || industry) && place) details.push(`in ${place}`);
        if (text("search-keywords") && company) details.push(`at ${company}`);
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
        return {title, details};
      };

      const sameSearch = (a, b) => FORM_IDS.every(id => String(a[id] ?? "") === String(b[id] ?? ""));

      //the history menu opens over the page and closes when the user picks something or clicks elsewhere
      const setHistoryOpen = open => {
        $("history").hidden = !open;
        $("history-toggle").setAttribute("aria-expanded", String(open));
      };
      $("history-toggle").addEventListener("click", () => setHistoryOpen($("history").hidden));
      document.addEventListener("mousedown", event => {
        if (!$("history").hidden && !$("history").parentElement.contains(event.target)) setHistoryOpen(false);
      });
      document.addEventListener("keydown", event => {
        if (event.key === "Escape" && !$("history").hidden) {
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
          const remove = el("button", "\u00d7");
          remove.type = "button";
          remove.className = "history-remove";
          remove.setAttribute("aria-label", "Remove this search from history");
          remove.addEventListener("click", () => {
            storeHistory(loadHistory().filter((_, i) => i !== index));
            renderHistory();
          });
          item.append(run, remove);
          return item;
        }));
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
      $("clear-history").addEventListener("click", () => {
        storeHistory([]);
        window.Suggest.clearRecent();
        renderHistory();
      });

      restoreForm();
      renderHistory();
      searchForm.addEventListener("change", saveForm);

      $("clear-search").addEventListener("click", () => {
        searchForm.reset();
        try { localStorage.removeItem(FORM_KEY); } catch { /* nothing saved to remove */ }
        $("search-keywords").focus();
      });

      $("cancel-search").addEventListener("click", () => window.FistBumpAPI.cancelSearch());

      $("result-sort").addEventListener("change", () => {
        if (results) runSearch({...searchedQuery, sort: $("result-sort").value});
      });

      window.Suggest.attach($("search-company"), {key: "company", fixed: companyNames});
      window.Suggest.attach($("search-keywords"), {key: "keywords", fixed: window.Suggest.TITLES, list: true});
      window.Suggest.attach($("search-location"), {key: "location", fixed: window.Suggest.PLACES});

      selectTab("find");
    }
  };
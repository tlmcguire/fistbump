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
      const showLoading = message => {
        $("loading-message").textContent = message;
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

      //get one row of the search results and define it's html
      const resultItem = result => {
        const item = document.createElement("li");
        item.className = "result";
        const open = document.createElement("button");
          open.type = "button";
          open.className = "result-open";
          open.append(
            el("strong", result.position_title || "Unknown position"),
            el ("span", [
              result.company_name,
              result.location,
              result.work_mode,
              result.updated_at && new Date(result.updated_at).toLocaleDateString()
            ].filter(Boolean).join(" . "))
          );

          open.addEventListener("click", () => displayPosting(result, true));
          item.append(open);

          const url = safeURL(result.company_url);
          if (url) {
            const link = el("a", "Company site");
            link.className = "result-link";
            link.href = url;
            link.target = "_blank";
            link.rel = "noopener noreferrer";
            item.append(link);
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
        `${data.matched.toLocaleString()} matches from ${data.postings_scanned.toLocaleString()} postings on ${data.boards_searched.toLocaleString()} boards`;
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

      //main function to run the search
      const runSearch = async query => {
        retry = () => runSearch(query);
        searchedQuery = query;
        results = null;
        setBusy(true);
        showLoading("Searching Greenhouse...");

        try {
          const data = await window.FistBumpAPI.search({...query, offset: 0});
          pages = new Map([[1, data]]);
          currentPage = 1;
          results = data;
          renderResults(data);
          showState("results");
        } catch (error) {
          showError(error);
        } finally {
          setBusy(false);
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
      const goToPage = async page => {
        if (page < 1 || page > pageCount()) return;
        retry = () => goToPage(page);
        let data = pages.get(page);

        if (!data) {
          setBusy(true);
          $("pager").querySelectorAll("button").forEach(node => { node.disabled = true; });
          try {
            data = await window.FistBumpAPI.search({...searchedQuery, offset: (page - 1) * PAGE_SIZE});
          } catch (error) {
            showError(error);
            return;
          } finally {
            setBusy(false);
          }
          pages.set(page, data);
        }

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

        const query = {
          keywords: $("search-keywords").value.split(",").map(word => word.trim()).filter(Boolean),
          location: $("search-location").value.trim(),
          work_mode: $("search-work-mode").value,
          limit: PAGE_SIZE
        };
        window.Suggest.remember("keywords", query.keywords);
        window.Suggest.remember("location", [query.location]);
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

      window.Suggest.attach($("search-keywords"), {key: "keywords", fixed: window.Suggest.TITLES, list: true});
      window.Suggest.attach($("search-location"), {key: "location", fixed: window.Suggest.PLACES});

      selectTab("find");
    }
  };
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
      let searchedLocation = "";

  
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
            link.href = url;
            link.target = "_blank";
            link.rel = "noopener noreferrer";
            item.append(link);
          }
          return item;
      };

      //fill all results
      const renderResults = data => {
        $("result-counts").textContent = 
        `${data.matched.toLocaleString()} matches from ${data.postings_scanned.toLocaleString()} postings on ${data.boards_searched.toLocaleString()} boards`;
        //say so when nothing is in the requested location, so remote results are not mistaken for local ones
        const note = $("result-note");
        note.hidden = !(searchedLocation && data.matched > 0 && data.location_matched === 0);
        note.textContent = `No postings found in ${searchedLocation}. Showing remote postings instead.`;
        $("result-empty").hidden = data.postings.length > 0;
        $("result-list").replaceChildren(...data.postings.map(resultItem));

        $("result-errors").hidden = data.errors.length === 0;
        $("result-errors-list").replaceChildren(
          ...data.errors.map(failed => el("li", `${failed.board}: ${failed.message}`))
        );
      };

      //main function to run the search
      const runSearch = async query => {
        retry = () => runSearch(query);
        searchedLocation = query.location;
        results = null;
        setBusy(true);
        showLoading("Searching Greenhouse...");

        try {
          const data = await window.FistBumpAPI.search(query);
          renderResults(data);
          results = data;
          showState("results");
        } catch (error) {
          showError(error);
        } finally {
          setBusy(false);
        }
      };
  
      //rest of script is state handling and button events (updated 10/22 for tabs and new retry system replacing lastUrl)
      tabs.find.addEventListener("click", () => selectTab("find"));
      tabs.link.addEventListener("click", () => selectTab("link"));

      searchForm.addEventListener("submit", event => {
        event.preventDefault();

        runSearch({
          keywords: $("search-keywords").value.split(",").map(word => word.trim()).filter(Boolean),
          location: $("search-location").value.trim(),
          work_mode: $("search-work-mode").value
        });
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
      selectTab("find");
    }
  };
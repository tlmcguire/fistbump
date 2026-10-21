window.JobParser = {
    mount() {
      const form = document.getElementById("job-form");
      const input = document.getElementById("job-url");
      const loadButton = form.querySelector('button[type="submit"]');
      const states = ["initial", "loading", "success", "error"];
      let posting = null;
      let lastUrl = "";
  
      //gets each section's state and shows it accordingly.
      const showState = state => {
        states.forEach(name => {
          document.getElementById(`state-${name}`).hidden = name !== state;
        });
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
      const displayPosting = data => {
        posting = data;
  
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
  
      const loadPosting = async url => {
        lastUrl = url;
        loadButton.disabled = true;
        showState("loading");
  
        try {
          displayPosting(await window.FistBumpAPI.link(url));
        } catch (error) {
          showError(error);
        } finally {
          loadButton.disabled = false;
        }
      };
  
      //rest of script is state handling and button events
      form.addEventListener("submit", event => {
        event.preventDefault();
        const url = input.value.trim();
  
        if (url) {
          loadPosting(url);
        }
      });
  
      document.getElementById("reload-listing").addEventListener("click", async () => {
        if (!posting) return;
  
        showState("loading");
  
        try {
          displayPosting(
            await window.FistBumpAPI.posting(posting.board, posting.external_id)
          );
        } catch (error) {
          showError(error);
        }
      });
  
      document.getElementById("try-again").addEventListener("click", () => {
        if (lastUrl) {
          loadPosting(lastUrl);
        } else {
          showState("initial");
        }
      });
  
      showState("initial");
    }
  };
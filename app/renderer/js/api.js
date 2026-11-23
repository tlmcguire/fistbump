window.FistBumpAPI = {
    //runs a call and strips any error into a plain Error object
    async request(call) {
      let result;
      try {
        result = await call();
      } catch (err) {
        const msg = String(err?.message || "").replace(
          /^Error invoking remote method '[^']*': (Error: )?/, ""
        );
        throw new Error(msg || "Connection error");
      }
      //updated front end error handling for pahse 03
      //a failed request is {error: {code, message}}. A revision or a test result may carry "error" as a plain
      //string (why a round failed), which is data to show, not a failed request.
      if (result?.error && typeof result.error === "object") {
        const error = new Error(result.error.message || "An unknown error has occurred");
        error.code = result.error.code;
        error.details = result.error.details;
        // The backend is restarting after a crash. The message shows with a button that runs the
        // same call again, because the backend comes back on its own.
        if (error.code === "unreachable") window.FistBumpAPI.backendDown(call, error);
        throw error;
      }
      return result;
    },

    // The notice at the bottom of the window: what happened, and a Try again button.
    backendDown(call, error) {
      let notice = document.getElementById("backend-notice");
      if (!notice) {
        notice = document.createElement("div");
        notice.id = "backend-notice";
        notice.className = "backend-notice";
        notice.setAttribute("role", "status");
        document.body.appendChild(notice);
      }
      const text = document.createElement("span");
      text.textContent = error?.message || "The backend is not answering.";
      const retry = document.createElement("button");
      retry.type = "button";
      retry.textContent = "Try again";
      retry.addEventListener("click", async () => {
        notice.remove();
        try {
          await window.FistBumpAPI.request(call);
        } catch {
          // The view shows the error. The notice comes back if it fails again.
        }
      });
      notice.replaceChildren(text, retry);
    },
  
    async link(url) {
      if (!window.api?.connectors?.link) {
        throw new Error("Connection error");
      }
      return this.request(() => window.api.connectors.link(url));
    },
  
    async posting(board, jobId) {
      if (!window.api?.connectors?.posting) {
        throw new Error("Connection error");
      }
      return this.request(() => window.api.connectors.posting(board, jobId));
    },

    async categories() {
      if (!window.api?.connectors?.categories) {
        throw new Error("Connection error");
      }
      return this.request(() => window.api.connectors.categories());
    },

    //the company's logo as a data URL, or null when its site has none
    async logo(url, board) {
      if (!window.api?.connectors?.logo) return null;
      try {
        return (await this.request(() => window.api.connectors.logo(url, board))).data_url || null;
      } catch {
        return null;
      }
    },

    //stops the search in progress; the pending search call then fails with "Search canceled"
    cancelSearch() {
      return window.api?.connectors?.cancelSearch?.();
    },

    //Home's job matches: a search that runs apart from the Find search
    async suggest(query) {
      if (!window.api?.connectors?.suggest) {
        throw new Error("Connection error");
      }
      return this.request(() => window.api.connectors.suggest(query));
    },

    async search(query) {
      if (!window.api?.connectors?.search) {
        throw new Error("Connection error");
      }
      return this.request(() => window.api.connectors.search(query));
    }
  };

  //handles new functions called by resume-builder.js and app.js
  const wrap = (group, names) => Object.fromEntries(names.map(name => [
    name,
    (...args) => window.FistBumpAPI.request(() => window.api[group][name](...args)),
  ]));

  window.FistBumpAPI.resume = wrap("resume", [
    "get", "putProfile", "preview", "createExperience", "updateExperience", "deleteExperience", 
    "createEducation", "updateEducation", "deleteEducation",
    "importFile", "importData", "importText", "importApply", "export", "previewPdf",
  ]);

  //handles job management in phase 4
  window.FistBumpAPI.resumes = wrap("resumes", ["list", "get", "create","update", "delete"]);
  window.FistBumpAPI.importJobs = postings => window.FistBumpAPI.request(() => window.api.connectors.import(postings));
  window.FistBumpAPI.jobs = wrap("jobs", ["parse", "create", "list", "get", "update", "delete", "analyze", "analyzeText",
    "archive", "unarchive", "trash", "restore"]);

  window.FistBumpAPI.cost = wrap("cost", ["analyze"]);

  //phase 7: the application tracker
  window.FistBumpAPI.applications = wrap("applications", ["list", "create", "get", "update", "delete",
    "bulkStatus", "archive", "unarchive", "bulkArchive"]);

  //phase 5
  window.FistBumpAPI.revisions = wrap("revisions", ["create", "list", "get", "cancel", "delete"]);
  window.FistBumpAPI.suggestions = wrap("suggestions", ["accept", "reject", "edit"]);
  window.FistBumpAPI.tailored = wrap("tailored", ["create", "list", "get", "diff", "delete", "export", "previewPdf"]);
  window.FistBumpAPI.ai = wrap("ai", ["status", "setMode", "setRemote", "clearRemote", "test"]);
  //phase 8: the local model catalog
  window.FistBumpAPI.models = wrap("models", ["list", "get", "download", "downloadStatus", "cancelDownload", "delete"]);

  //phase 6: show an exported file in its folder
  window.FistBumpAPI.shell = wrap("shell", ["showItem"]);

  // The backend is back after a crash, so the notice at the bottom of the window goes away.
  window.api?.onBackendRestarted?.(() => document.getElementById("backend-notice")?.remove());
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
      if (result?.error) {
        throw new Error(result.error.message || "An unknown error has occurred");
      }
      return result;
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

    async search(query) {
      if (!window.api?.connectors?.search) {
        throw new Error("Connection error");
      }
      return this.request(() => window.api.connectors.search(query));
    }
  };
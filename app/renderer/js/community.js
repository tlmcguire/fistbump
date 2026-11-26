// Shared job boards and crowdsourced job listings (LinkedIn, Indeed, Workday, etc.),
// backed by the Fistbump community relay.
//  - Boards (opt-in in Settings > Search): fetched about once a day and handed to the backend
//    (connectors.community_boards), which searches them along with the built-in ones.
//  - Listings: users can opt in to share external listings (link + scrubbed content) when saving
//    a pasted listing, and browse/search/save published community listings in Community resources.
window.Community = (() => {
  const api = () => window.FistBumpAPI;
  const FETCHED = "fistbump.community-fetched"; //when boards were last fetched
  const PENDING_KEY = "fistbump.community-pending-listings"; //listings this user submitted, shown as pending review
  const DAY = 24 * 60 * 60 * 1000;

  const say = text => {
    const line = document.getElementById("community-status");
    if (line) line.textContent = text;
  };

  const readPending = () => {
    try {
      const list = JSON.parse(localStorage.getItem(PENDING_KEY) || "[]");
      return Array.isArray(list) ? list : [];
    } catch {
      return [];
    }
  };
  const writePending = list => {
    try { localStorage.setItem(PENDING_KEY, JSON.stringify(list.slice(0, 30))); } catch {}
  };

  let cachedListings = null;
  let cachedBoards = null;

  async function refresh(force = false) {
    if (!window.Prefs.get("communityUse")) return;
    let last = 0;
    try { last = Number(localStorage.getItem(FETCHED)) || 0; } catch {}
    if (!force && Date.now() - last < DAY) return;
    try {
      const file = await api().community.boards();
      cachedBoards = file?.boards || [];
      const list = cachedBoards
        .filter(b => ["greenhouse", "lever", "ashby"].includes(b.ats) && /^[a-z0-9][a-z0-9_.-]{0,63}$/.test(b.token || ""))
        .map(b => `${b.ats}:${b.token}`);
      await api().settings.put({"connectors.community_boards": list});
      try { localStorage.setItem(FETCHED, String(Date.now())); } catch {}
      say(list.length === 1 ? "1 shared board is in your searches." : `${list.length} shared boards are in your searches.`);
    } catch (err) {
      say(`Shared boards could not be fetched: ${err.message}`);
    }
  }

  async function stopUsing() {
    try {
      await api().settings.put({"connectors.community_boards": []});
      localStorage.removeItem(FETCHED);
    } catch {}
    say("Shared boards are no longer in your searches.");
  }

  async function share(boards) {
    if (!window.Prefs.get("communityShare") || !boards?.length) return;
    try {
      const res = await api().community.shareBoards({boards});
      const n = res?.queued?.length || 0;
      if (n) say(n === 1 ? `Shared ${res.queued[0]}. It reaches everyone once it is reviewed.` : `Shared ${n} boards. They reach everyone once they are reviewed.`);
      else if (res?.known?.length) say("Already on the shared list. Thank you all the same.");
      else if (res?.not_found?.length) say(`${res.not_found[0]} did not answer as a job board, so it was not shared.`);
    } catch (err) {
      say(`Could not share just now: ${err.message}`);
    }
  }

  // Share a single external job listing (from LinkedIn, Indeed, Workday, etc.) with its content.
  async function shareListing(listing) {
    if (!listing?.company_name || !listing?.position_title || !listing?.raw_text) {
      throw new Error("A shared listing needs a company, title, and posting content.");
    }
    const res = await api().community.shareListing(listing);
    if (res?.status === "queued" && res.listing) {
      const pending = readPending().filter(x => x.id !== res.listing.id);
      pending.unshift({...res.listing, _pending: true});
      writePending(pending);
      window.dispatchEvent(new CustomEvent("fistbump:community"));
    }
    return res;
  }

  // Fetch published community listings and boards for the Community resources view.
  async function loadResources(force = false) {
    const [listingsFile, boardsFile] = await Promise.all([
      force || !cachedListings ? api().community.listings().catch(() => null) : Promise.resolve({listings: cachedListings}),
      force || !cachedBoards ? api().community.boards().catch(() => null) : Promise.resolve({boards: cachedBoards}),
    ]);
    if (listingsFile?.listings) cachedListings = listingsFile.listings;
    if (boardsFile?.boards) cachedBoards = boardsFile.boards;

    const published = Array.isArray(cachedListings) ? cachedListings : [];
    const pubIds = new Set(published.map(x => x.id));
    // Drop local pending items once they appear in the published community list
    const pending = readPending().filter(x => !pubIds.has(x.id));
    writePending(pending);

    return {
      listings: published,
      pending,
      boards: Array.isArray(cachedBoards) ? cachedBoards : [],
      offline: !listingsFile && !boardsFile && !cachedListings && !cachedBoards,
    };
  }

  //turning sharing on offers the boards already on the watchlist once
  async function shareExisting() {
    try {
      const vals = await api().settings.get();
      const mine = Array.isArray(vals?.["connectors.custom_boards"]) ? vals["connectors.custom_boards"] : [];
      if (mine.length) await share(mine.map(b => (b.includes(":") ? b : `greenhouse:${b}`)));
    } catch {}
  }

  window.Prefs.onChange((key, value) => {
    if (key === "communityUse") value ? refresh(true) : stopUsing();
    if (key === "communityShare" && value) shareExisting();
  });
  setTimeout(() => refresh(), 5000); //after the first view has settled

  return {refresh, share, shareListing, loadResources};
})();

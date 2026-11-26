// Shared job boards, both off until turned on in Settings > Job search.
//  - Use: about once a day the shared list is fetched from the relay and handed to the backend
//    (connectors.community_boards), which searches those boards along with the built-in ones.
//  - Share: a board added to the watchlist is sent to the relay ("lever:netflix", nothing else). The relay checks
//    it is a real board and a maintainer merges it into data/community-boards.json, so everyone can find it.
window.Community = (() => {
  const api = () => window.FistBumpAPI;
  const FETCHED = "fistbump.community-fetched"; //when the list was last fetched, so it is asked for once a day
  const DAY = 24 * 60 * 60 * 1000;

  const say = text => {
    const line = document.getElementById("community-status");
    if (line) line.textContent = text;
  };

  async function refresh(force = false) {
    if (!window.Prefs.get("communityUse")) return;
    let last = 0;
    try { last = Number(localStorage.getItem(FETCHED)) || 0; } catch {}
    if (!force && Date.now() - last < DAY) return;
    try {
      const file = await api().community.boards();
      const list = (file?.boards || [])
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

  return {refresh, share};
})();

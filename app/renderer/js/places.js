// Some postings list a place for every state they hire in. Places.summary shortens such a list to a line that fits,
// and the full text stays available for a tooltip.
window.Places = (() => {
  const SEPARATOR = /\s*(?:;|\||\n)\s*/;

  // "Alabama - Remote; Arizona - Remote; ..." -> "Remote in 48 places"; "Austin; Boston; Denver; Miami" -> "Austin, Boston +2 more"
  const summary = (location, max = 60) => {
    const full = String(location || "").replace(/\s+/g, " ").trim();
    if (full.length <= max) return {short: full, full, shortened: false};
    const parts = full.split(SEPARATOR).map(part => part.trim()).filter(Boolean);
    if (parts.length >= 3 && parts.every(part => /(?:-|–|,)\s*remote$|\(remote\)$/i.test(part))) {
      return {short: `Remote in ${parts.length} places`, full, shortened: true};
    }
    if (parts.length >= 3) {
      let shown = [];
      for (const part of parts) {
        if ([...shown, part].join(", ").length > max - 12) break;
        shown.push(part);
      }
      if (!shown.length) shown = [parts[0]];
      const rest = parts.length - shown.length;
      return {short: rest > 0 ? `${shown.join(", ")} +${rest} more` : shown.join(", "), full, shortened: rest > 0};
    }
    return {short: full.slice(0, max - 1).replace(/[\s,;-]+\S*$/, "") + "…", full, shortened: true};
  };

  return {summary};
})();

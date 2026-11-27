// Calendar files both ways.
//  - write: an event (or many) as an .ics file any calendar app imports, and "add to" links for Google and Outlook.
//  - read: busy times out of the person's own calendars (an iCal address from Google, Outlook or iCloud), with
//    time zones, all-day events, and the repeating events (daily and weekly meetings) that fill a real week.
// Only times leave this file: titles of the person's own events are never kept or shown.
window.ICS = (() => {
  const pad = n => String(n).padStart(2, "0");
  const utcStamp = date => `${date.getUTCFullYear()}${pad(date.getUTCMonth() + 1)}${pad(date.getUTCDate())}T${pad(date.getUTCHours())}${pad(date.getUTCMinutes())}${pad(date.getUTCSeconds())}Z`;
  const escape = text => String(text || "").replace(/\\/g, "\\\\").replace(/;/g, "\\;").replace(/,/g, "\\,").replace(/\r\n|\r|\n/g, "\\n");
  //lines over 75 octets are folded, as the format asks
  const fold = line => {
    const out = [];
    let rest = line;
    while (rest.length > 74) { out.push(rest.slice(0, 74)); rest = ` ${rest.slice(74)}`; }
    out.push(rest);
    return out.join("\r\n");
  };

  const KIND = {interview: "Interview", call: "Call", follow_up: "Follow-up", other: "Plan"};
  const summaryOf = e => e.title || [KIND[e.kind] || "Plan", e.company_name].filter(Boolean).join(": ");
  const detailsOf = e => [e.position_title && e.company_name ? `${e.position_title} at ${e.company_name}` : "", e.notes || ""].filter(Boolean).join("\n\n");

  function vevent(e) {
    return [
      "BEGIN:VEVENT",
      `UID:fistbump-event-${e.id || Math.random().toString(36).slice(2)}@fistbump.local`,
      `DTSTAMP:${utcStamp(new Date())}`,
      `DTSTART:${utcStamp(new Date(e.starts_at))}`,
      `DTEND:${utcStamp(new Date(e.ends_at))}`,
      `SUMMARY:${escape(summaryOf(e))}`,
      e.location ? `LOCATION:${escape(e.location)}` : null,
      detailsOf(e) ? `DESCRIPTION:${escape(detailsOf(e))}` : null,
      e.kind === "interview" ? "BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Interview in an hour\r\nTRIGGER:-PT1H\r\nEND:VALARM" : null,
      "END:VEVENT",
    ].filter(Boolean).map(fold).join("\r\n");
  }

  const file = events => ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Fistbump//Calendar//EN", "CALSCALE:GREGORIAN", "METHOD:PUBLISH",
    ...events.map(vevent), "END:VCALENDAR", ""].join("\r\n");

  const googleLink = e => `https://calendar.google.com/calendar/render?${new URLSearchParams({
    action: "TEMPLATE", text: summaryOf(e), dates: `${utcStamp(new Date(e.starts_at))}/${utcStamp(new Date(e.ends_at))}`,
    details: detailsOf(e), location: e.location || "",
  })}`;
  const outlookLink = e => `https://outlook.live.com/calendar/0/deeplink/compose?${new URLSearchParams({
    path: "/calendar/action/compose", rru: "addevent", subject: summaryOf(e),
    startdt: new Date(e.starts_at).toISOString(), enddt: new Date(e.ends_at).toISOString(), body: detailsOf(e), location: e.location || "",
  })}`;

  // ---------- reading ----------

  //how far a time zone is ahead of UTC at a moment, in ms
  const zoneOffset = (utcMs, zone) => {
    try {
      const parts = Object.fromEntries(new Intl.DateTimeFormat("en-US", {timeZone: zone, hourCycle: "h23", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit"})
        .formatToParts(new Date(utcMs)).map(p => [p.type, p.value]));
      return Date.UTC(+parts.year, +parts.month - 1, +parts.day, +parts.hour % 24, +parts.minute, +parts.second) - utcMs;
    } catch {
      return -new Date(utcMs).getTimezoneOffset() * 60000; //an unknown zone reads as this computer's
    }
  };

  //a DTSTART/DTEND value with its parameters → {ms, allDay}
  function readTime(value, params) {
    const m = /^(\d{4})(\d{2})(\d{2})(?:T(\d{2})(\d{2})(\d{2})?(Z)?)?$/.exec(value.trim());
    if (!m) return null;
    const [, y, mo, d, h, mi, s, z] = m;
    if (h === undefined || /VALUE=DATE(?!-)/i.test(params)) return {ms: new Date(+y, +mo - 1, +d).getTime(), allDay: true};
    const wall = Date.UTC(+y, +mo - 1, +d, +h, +mi, +(s || 0));
    if (z) return {ms: wall, allDay: false};
    const zone = /TZID=([^;:]+)/i.exec(params)?.[1]?.replace(/^"|"$/g, "");
    if (!zone) return {ms: new Date(+y, +mo - 1, +d, +h, +mi, +(s || 0)).getTime(), allDay: false}; //floating: local time
    let utc = wall - zoneOffset(wall, zone);
    utc = wall - zoneOffset(utc, zone); //once more, for the hour around a clock change
    return {ms: utc, allDay: false};
  }

  const DAYS = {SU: 0, MO: 1, TU: 2, WE: 3, TH: 4, FR: 5, SA: 6};

  //each time a repeating event happens inside [from, to); daily and weekly rules, which is what fills a week
  function repeat(start, length, rule, exdates, from, to) {
    const r = Object.fromEntries(rule.split(";").map(p => p.split("=")));
    const freq = r.FREQ;
    if (freq !== "DAILY" && freq !== "WEEKLY") return start < to && start + length > from ? [start] : [];
    const every = Math.max(1, Number(r.INTERVAL) || 1);
    const until = r.UNTIL ? readTime(r.UNTIL, "")?.ms ?? Infinity : Infinity;
    const count = Number(r.COUNT) || Infinity;
    const byDay = freq === "WEEKLY" && r.BYDAY ? r.BYDAY.split(",").map(d => DAYS[d.slice(-2)]).filter(d => d !== undefined) : null;
    const out = [];
    const base = new Date(start);
    let n = 0;
    for (let i = 0; i < 3660 && n < count; i++) {
      const day = new Date(base);
      day.setDate(base.getDate() + i);
      const t = day.getTime();
      if (t > until || t >= to) break;
      const weeks = Math.floor(i / 7);
      const hit = freq === "DAILY" ? i % every === 0
        : weeks % every === 0 && (byDay ? byDay.includes(day.getDay()) : day.getDay() === base.getDay());
      if (!hit) continue;
      n++;
      if (exdates.has(day.toDateString())) continue;
      if (t + length > from) out.push(t);
    }
    return out;
  }

  //busy times from calendar text, inside [from, to): [{start, end, allDay}] in ms; nothing else is kept
  function busy(text, from, to) {
    const lines = String(text).replace(/\r?\n[ \t]/g, "").split(/\r?\n/);
    const out = [];
    let ev = null;
    for (const line of lines) {
      if (line === "BEGIN:VEVENT") { ev = {ex: new Set()}; continue; }
      if (line === "END:VEVENT") {
        if (ev?.start && ev.status !== "CANCELLED" && ev.transp !== "TRANSPARENT") {
          const end = ev.end ? ev.end.ms : ev.start.ms + (ev.start.allDay ? 86400000 : 3600000);
          const length = Math.max(0, end - ev.start.ms);
          const starts = ev.rule ? repeat(ev.start.ms, length, ev.rule, ev.ex, from, to) : (ev.start.ms < to && end > from ? [ev.start.ms] : []);
          for (const s of starts) out.push({start: s, end: s + length, allDay: ev.start.allDay});
        }
        ev = null;
        continue;
      }
      if (!ev) continue;
      const colon = line.indexOf(":");
      if (colon < 0) continue;
      const [name, ...params] = line.slice(0, colon).split(";");
      const value = line.slice(colon + 1);
      const p = params.join(";");
      if (name === "DTSTART") ev.start = readTime(value, p);
      else if (name === "DTEND") ev.end = readTime(value, p);
      else if (name === "RRULE") ev.rule = value;
      else if (name === "STATUS") ev.status = value.trim().toUpperCase();
      else if (name === "TRANSP") ev.transp = value.trim().toUpperCase();
      else if (name === "EXDATE") for (const v of value.split(",")) { const t = readTime(v, p); if (t) ev.ex.add(new Date(t.ms).toDateString()); } //skipped days
    }
    return out.sort((a, b) => a.start - b.start);
  }

  //what a plan runs into: other plans and busy times that overlap it (all-day busy days do not count)
  const clashes = (plan, plans, busyTimes) => {
    const s = new Date(plan.starts_at).getTime();
    const e = new Date(plan.ends_at).getTime();
    return [
      ...plans.filter(p => p.id !== plan.id && new Date(p.starts_at).getTime() < e && new Date(p.ends_at).getTime() > s).map(p => ({kind: "plan", plan: p})),
      ...busyTimes.filter(b => !b.allDay && b.start < e && b.end > s).map(b => ({kind: "busy", busy: b})),
    ];
  };

  return {file, vevent, googleLink, outlookLink, busy, clashes, summaryOf, readTime, escapeText: escape};
})();

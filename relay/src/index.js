// Fistbump community relay: a Cloudflare Worker between the app and GitHub.
//
//   GET  /v1/reports/similar?q=words   up to 3 open reports that look like the same problem
//   POST /v1/reports                   files a report as a GitHub issue, or adds it to the one it duplicates
//   POST /v1/boards                    takes job boards people chose to share; checks each is real, queues the new ones
//   GET  /v1/boards                    the shared boards, from data/community-boards.json in the repository
//   POST /v1/listings                  takes scrubbed external job listings (LinkedIn, Indeed, etc.) people chose to share
//   GET  /v1/listings                  the shared listings, from data/community-listings.json in the repository
//   (cron)                             once a day, opens or updates pull requests adding queued boards and listings
//
// People do not need a GitHub account: the Worker files under a GitHub App installed on the repository (or a token).
// Nothing personal is kept: no address, no account, only what the person read and sent. Requests are rate limited
// per IP, using a salted hash of the address that is forgotten within the hour.
//
// Configuration (wrangler.toml and `wrangler secret put`):
//   REPO                 "owner/name" of the repository
//   GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY (PKCS#8 PEM), GITHUB_INSTALLATION_ID   or   GITHUB_TOKEN
//   RATE_SALT            any random string
//   KV binding RELAY     rate limits, queued boards, recent report fingerprints

const JSON_HEADERS = { "content-type": "application/json; charset=utf-8" };
const BOARDS_FILE = "data/community-boards.json";
const LISTINGS_FILE = "data/community-listings.json";
const LABEL = "from-the-app";
const KINDS = { bug: "Something is broken", idea: "An idea", question: "A question" };
const ATS = ["greenhouse", "lever", "ashby"];
const WORK_MODES = new Set(["Remote", "Hybrid", "On-site"]);
const TOKEN = /^[a-z0-9][a-z0-9_.-]{0,63}$/;

// ---------- small helpers ----------

export const json = (body, status = 200, extra = {}) => new Response(JSON.stringify(body), { status, headers: { ...JSON_HEADERS, ...extra } });
const problem = (status, code, message) => json({ error: { code, message } }, status);

//Strips what should never leave someone's computer by accident: email addresses, phone numbers, home folders.
export function scrub(text, max) {
  return String(text ?? "")
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, "")
    .replace(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/gi, "[email removed]")
    .replace(/\+?\d[\d\s().-]{8,}\d/g, "[number removed]")
    .replace(/(\/Users\/|\/home\/|C:\\Users\\)[^\s/\\]+/gi, "$1[name]")
    .trim()
    .slice(0, max);
}

const STOP = new Set("a an and are as at be but by can did do does for from has have how i if in into is it its me my no not of on or so that the then there this to was we were what when where which while who why will with you your".split(" "));
//The words that say what a report is about, for comparing one report with another.
export function words(text) {
  return [...new Set(String(text).toLowerCase().replace(/[^a-z0-9\s]/g, " ").split(/\s+/)
    .filter(w => w.length > 2 && !STOP.has(w))
    .map(w => w.replace(/(ing|ed|es|s)$/, "")))];
}
//How alike two sets of words are, 0 to 1 (Jaccard, weighted toward the shorter text).
export function likeness(a, b) {
  if (!a.length || !b.length) return 0;
  const B = new Set(b);
  const shared = a.filter(w => B.has(w)).length;
  return shared / Math.min(a.length, b.length) * 0.6 + shared / new Set([...a, ...b]).size * 0.4;
}

async function sha256(text) {
  const bytes = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(bytes)].map(b => b.toString(16).padStart(2, "0")).join("");
}

async function limited(env, request, bucket, perHour) {
  if (!env.RELAY) return false;
  const ip = request.headers.get("cf-connecting-ip") || "unknown";
  const hour = Math.floor(Date.now() / 3600000);
  const key = `rl:${bucket}:${hour}:${(await sha256(`${env.RATE_SALT || ""}:${ip}`)).slice(0, 24)}`;
  const n = Number(await env.RELAY.get(key)) || 0;
  if (n >= perHour) return true;
  await env.RELAY.put(key, String(n + 1), { expirationTtl: 3700 });
  return false;
}

// ---------- GitHub ----------

const b64url = bytes => btoa(String.fromCharCode(...new Uint8Array(bytes))).replace(/=+$/, "").replace(/\+/g, "-").replace(/\//g, "_");
const b64urlText = text => b64url(new TextEncoder().encode(text));

async function appJwt(env) {
  const pem = env.GITHUB_APP_PRIVATE_KEY.replace(/-----[^-]+-----/g, "").replace(/\s+/g, "");
  const der = Uint8Array.from(atob(pem), c => c.charCodeAt(0));
  const key = await crypto.subtle.importKey("pkcs8", der, { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, false, ["sign"]);
  const now = Math.floor(Date.now() / 1000);
  const head = b64urlText(JSON.stringify({ alg: "RS256", typ: "JWT" }));
  const body = b64urlText(JSON.stringify({ iat: now - 60, exp: now + 540, iss: String(env.GITHUB_APP_ID) }));
  const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", key, new TextEncoder().encode(`${head}.${body}`));
  return `${head}.${body}.${b64url(sig)}`;
}

let cachedToken = null;
async function githubToken(env) {
  if (env.GITHUB_TOKEN) return env.GITHUB_TOKEN;
  if (cachedToken && cachedToken.expires > Date.now() + 60000) return cachedToken.value;
  const res = await fetch(`https://api.github.com/app/installations/${env.GITHUB_INSTALLATION_ID}/access_tokens`, {
    method: "POST",
    headers: { authorization: `Bearer ${await appJwt(env)}`, accept: "application/vnd.github+json", "user-agent": "fistbump-relay" },
  });
  if (!res.ok) throw new Error(`GitHub app token: ${res.status}`);
  const data = await res.json();
  cachedToken = { value: data.token, expires: Date.parse(data.expires_at) };
  return data.token;
}

export async function gh(env, path, init = {}) {
  const res = await fetch(`https://api.github.com${path}`, {
    ...init,
    headers: {
      authorization: `Bearer ${await githubToken(env)}`,
      accept: "application/vnd.github+json",
      "user-agent": "fistbump-relay",
      "x-github-api-version": "2022-11-28",
      ...(init.body ? { "content-type": "application/json" } : {}),
      ...init.headers,
    },
  });
  if (!res.ok && res.status !== 404 && res.status !== 422) throw new Error(`GitHub ${init.method || "GET"} ${path}: ${res.status}`);
  return { status: res.status, data: res.status === 204 ? null : await res.json().catch(() => null) };
}

// ---------- reports ----------

//Open reports from the app that look like the words given, best first.
export async function similar(env, text, limit = 3) {
  const mine = words(text);
  if (!mine.length) return [];
  const q = `repo:${env.REPO} is:issue is:open label:${LABEL} ${mine.slice(0, 6).join(" ")}`;
  const { data } = await gh(env, `/search/issues?per_page=10&q=${encodeURIComponent(q)}`);
  const loose = mine.length > 3 ? await gh(env, `/search/issues?per_page=10&q=${encodeURIComponent(`repo:${env.REPO} is:issue is:open label:${LABEL} ${mine.slice(0, 3).join(" ")}`)}`) : { data: null };
  const seen = new Map();
  for (const item of [...(data?.items || []), ...(loose.data?.items || [])]) seen.set(item.number, item);
  return [...seen.values()]
    .map(i => ({ number: i.number, title: i.title, url: i.html_url, comments: i.comments, likeness: likeness(mine, words(`${i.title} ${i.title} ${(i.body || "").slice(0, 600)}`)) }))
    .filter(i => i.likeness >= 0.25)
    .sort((a, b) => b.likeness - a.likeness)
    .slice(0, limit);
}

const SAME = 0.62; //this alike, a new report is added to the old one instead of filed again

export function issueBody(r) {
  const lines = [
    `**${KINDS[r.kind]}**, sent from the Fistbump app.`,
    "",
    "### What happened",
    r.details || "_No details given._",
  ];
  if (r.steps) lines.push("", "### How to make it happen", r.steps);
  lines.push("", "### Where", `Fistbump ${r.app.version || "?"} on ${r.app.os || "?"}${r.app.view ? `, on the ${r.app.view} page` : ""}`);
  lines.push("", `<!-- fistbump-report ${r.fingerprint} -->`);
  return lines.join("\n");
}

async function addTo(env, number, r) {
  const body = [`**Someone else ran into this too**, from the app (Fistbump ${r.app.version || "?"} on ${r.app.os || "?"}).`, "", r.details || ""].join("\n");
  await gh(env, `/repos/${env.REPO}/issues/${number}/comments`, { method: "POST", body: JSON.stringify({ body }) });
  await gh(env, `/repos/${env.REPO}/issues/${number}/reactions`, { method: "POST", body: JSON.stringify({ content: "+1" }) });
}

export function readReport(body) {
  const kind = Object.hasOwn(KINDS, body?.kind) ? body.kind : "bug";
  const title = scrub(body?.title, 120);
  const details = scrub(body?.details, 4000);
  const steps = scrub(body?.steps, 2000);
  const app = { version: scrub(body?.app?.version, 20), os: scrub(body?.app?.os, 40), view: scrub(body?.app?.view, 30) };
  const sameAs = Number.isInteger(body?.same_as) && body.same_as > 0 ? body.same_as : null;
  return { kind, title, details, steps, app, sameAs };
}

async function report(env, request) {
  if (await limited(env, request, "report", 6)) return problem(429, "rate_limited", "That is a lot of reports in one hour. Please try again later.");
  const r = readReport(await request.json().catch(() => null));
  if (r.title.length < 4 || (r.details.length < 8 && !r.sameAs)) return problem(400, "validation_failed", "Say in a few words what happened.");
  r.fingerprint = (await sha256(words(`${r.title} ${r.details}`).sort().join(" "))).slice(0, 16);

  //The person said it is one of the reports they were shown.
  if (r.sameAs) {
    const { status, data } = await gh(env, `/repos/${env.REPO}/issues/${r.sameAs}`);
    if (status === 200 && data.state === "open" && (data.labels || []).some(l => l.name === LABEL)) {
      await addTo(env, r.sameAs, r);
      return json({ status: "added", number: r.sameAs, url: data.html_url });
    }
  }
  //The very same report sent again (a double click, a retry).
  const recent = env.RELAY && await env.RELAY.get(`fp:${r.fingerprint}`, "json");
  if (recent) return json({ status: "added", number: recent.number, url: recent.url });
  //A report that reads like an open one.
  const [best] = await similar(env, `${r.title} ${r.details}`, 1);
  if (best && best.likeness >= SAME) {
    await addTo(env, best.number, r);
    return json({ status: "added", number: best.number, url: best.url });
  }
  const { data } = await gh(env, `/repos/${env.REPO}/issues`, {
    method: "POST",
    body: JSON.stringify({ title: r.title, body: issueBody(r), labels: [LABEL, r.kind] }),
  });
  if (!data?.number) throw new Error("GitHub did not create the issue");
  await env.RELAY?.put(`fp:${r.fingerprint}`, JSON.stringify({ number: data.number, url: data.html_url }), { expirationTtl: 86400 * 7 });
  return json({ status: "created", number: data.number, url: data.html_url }, 201);
}

// ---------- boards ----------

export function readBoards(body) {
  const list = Array.isArray(body?.boards) ? body.boards.slice(0, 25) : [];
  const out = [];
  for (const entry of list) {
    const [ats, token] = String(entry || "").toLowerCase().trim().split(":");
    if (ATS.includes(ats) && TOKEN.test(token || "")) out.push({ ats, token });
  }
  return [...new Map(out.map(b => [`${b.ats}:${b.token}`, b])).values()];
}

//Asks the job board itself whether it exists, and what the company calls itself.
export async function checkBoard({ ats, token }) {
  const url = ats === "greenhouse" ? `https://boards-api.greenhouse.io/v1/boards/${token}`
    : ats === "lever" ? `https://api.lever.co/v0/postings/${token}?mode=json&limit=1`
    : `https://api.ashbyhq.com/posting-api/job-board/${token}`;
  const res = await fetch(url, { headers: { "user-agent": "fistbump-relay" } });
  if (!res.ok) return null;
  const data = await res.json().catch(() => null);
  if (ats === "lever" && !Array.isArray(data)) return null;
  //Lever's public list has no company name, so the board's own name stands in
  const company = ats === "greenhouse" ? data?.name : ats === "lever" ? token : data?.organizationName || data?.name;
  return { ats, token, company: scrub(company || token, 80) };
}

async function readShared(env) {
  const { status, data } = await gh(env, `/repos/${env.REPO}/contents/${BOARDS_FILE}`);
  if (status !== 200) return { file: { version: 1, boards: [] }, sha: null };
  const text = new TextDecoder().decode(Uint8Array.from(atob(data.content.replace(/\s/g, "")), c => c.charCodeAt(0)));
  return { file: JSON.parse(text), sha: data.sha };
}

async function shareBoards(env, request) {
  if (await limited(env, request, "boards", 30)) return problem(429, "rate_limited", "Too many boards in one hour. They will keep until later.");
  const boards = readBoards(await request.json().catch(() => null));
  if (!boards.length) return problem(400, "validation_failed", "Nothing to share: boards look like greenhouse:stripe or lever:netflix.");
  const known = new Set(((await cachedShared(env)).boards || []).map(b => `${b.ats}:${b.token}`));
  const result = { queued: [], known: [], not_found: [] };
  for (const b of boards) {
    const key = `${b.ats}:${b.token}`;
    if (known.has(key) || (env.RELAY && await env.RELAY.get(`board:${key}`))) { result.known.push(key); continue; }
    const real = await checkBoard(b);
    if (!real) { result.not_found.push(key); continue; }
    await env.RELAY?.put(`board:${key}`, JSON.stringify({ ...real, added: new Date().toISOString().slice(0, 10) }));
    result.queued.push(key);
  }
  return json(result);
}

async function cachedShared(env) {
  const hit = env.RELAY && await env.RELAY.get("shared-boards", "json");
  if (hit) return hit;
  const { file } = await readShared(env);
  await env.RELAY?.put("shared-boards", JSON.stringify(file), { expirationTtl: 6 * 3600 });
  return file;
}

//Once a day: the queued boards go into one pull request, so a person reviews every addition before it ships.
export async function publishBoards(env) {
  if (!env.RELAY) return;
  const { keys } = await env.RELAY.list({ prefix: "board:" });
  if (!keys.length) return;
  const queued = (await Promise.all(keys.map(k => env.RELAY.get(k.name, "json")))).filter(Boolean);
  const branch = "community-boards";
  const { data: repo } = await gh(env, `/repos/${env.REPO}`);
  const base = repo.default_branch;
  const { data: head } = await gh(env, `/repos/${env.REPO}/git/ref/heads/${base}`);
  const existing = await gh(env, `/repos/${env.REPO}/git/ref/heads/${branch}`);
  if (existing.status === 404) {
    await gh(env, `/repos/${env.REPO}/git/refs`, { method: "POST", body: JSON.stringify({ ref: `refs/heads/${branch}`, sha: head.object.sha }) });
  }
  const { data: current } = await gh(env, `/repos/${env.REPO}/contents/${BOARDS_FILE}?ref=${branch}`);
  const file = current?.content ? JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(current.content.replace(/\s/g, "")), c => c.charCodeAt(0)))) : { version: 1, boards: [] };
  const have = new Set(file.boards.map(b => `${b.ats}:${b.token}`));
  const added = queued.filter(b => !have.has(`${b.ats}:${b.token}`));
  if (added.length) {
    file.boards = [...file.boards, ...added].sort((a, b) => `${a.ats}:${a.token}`.localeCompare(`${b.ats}:${b.token}`));
    file.updated = new Date().toISOString().slice(0, 10);
    const content = btoa(String.fromCharCode(...new TextEncoder().encode(`${JSON.stringify(file, null, 2)}\n`)));
    await gh(env, `/repos/${env.REPO}/contents/${BOARDS_FILE}`, {
      method: "PUT",
      body: JSON.stringify({ message: `data: ${added.length} shared job board${added.length === 1 ? "" : "s"}`, content, branch, ...(current?.sha ? { sha: current.sha } : {}) }),
    });
    const owner = env.REPO.split("/")[0];
    const { data: open } = await gh(env, `/repos/${env.REPO}/pulls?state=open&head=${owner}:${branch}`);
    if (!open?.length) {
      await gh(env, `/repos/${env.REPO}/pulls`, {
        method: "POST",
        body: JSON.stringify({ title: "Job boards shared from the app", head: branch, base, body: "Boards people shared from Fistbump. Each one answered as a real job board when it was shared. Merge to make them available to everyone who uses shared boards." }),
      });
    }
  }
  await Promise.all(keys.map(k => env.RELAY.delete(k.name)));
}

// ---------- community listings ----------

function safeHttpUrl(value, max, allowHttp = true) {
  const raw = String(value ?? "").trim();
  if (!raw || raw.length > max) return "";
  try {
    const u = new URL(raw);
    if (u.protocol === "https:" || (allowHttp && u.protocol === "http:")) return u.href;
  } catch {
    // invalid URL
  }
  return "";
}

function siteFromUrl(value) {
  if (!value) return "";
  try {
    const host = new URL(value).hostname.replace(/^www\./i, "").toLowerCase();
    if (host === "linkedin.com" || host.endsWith(".linkedin.com")) return "LinkedIn";
    if (host === "indeed.com" || host.endsWith(".indeed.com")) return "Indeed";
    if (host === "glassdoor.com" || host.endsWith(".glassdoor.com")) return "Glassdoor";
    if (host.includes("workday")) return "Workday";
    if (host === "wellfound.com" || host === "angel.co") return "Wellfound";
    if (host === "ziprecruiter.com" || host.endsWith(".ziprecruiter.com")) return "ZipRecruiter";
    if (host.includes("smartrecruiters")) return "SmartRecruiters";
    if (host.includes("icims")) return "iCIMS";
    if (host.includes("jobvite")) return "Jobvite";
    return host;
  } catch {
    return "";
  }
}

export function readListing(body) {
  const company_name = scrub(body?.company_name, 120);
  const position_title = scrub(body?.position_title, 160);
  const location = scrub(body?.location, 120);
  const work_mode = WORK_MODES.has(body?.work_mode) ? body.work_mode : "";
  const listing_url = safeHttpUrl(body?.listing_url, 500, true);
  const company_url = safeHttpUrl(body?.company_url, 300, false);
  const raw_text = scrub(body?.raw_text, 25000);
  const source = siteFromUrl(listing_url) || scrub(body?.source, 40) || "Community";
  return { source, company_name, position_title, location, work_mode, listing_url, company_url, raw_text };
}

async function readSharedListings(env) {
  const { status, data } = await gh(env, `/repos/${env.REPO}/contents/${LISTINGS_FILE}`);
  if (status !== 200) return { file: { version: 1, listings: [] }, sha: null };
  const text = new TextDecoder().decode(Uint8Array.from(atob(data.content.replace(/\s/g, "")), c => c.charCodeAt(0)));
  return { file: JSON.parse(text), sha: data.sha };
}

async function cachedSharedListings(env) {
  const hit = env.RELAY && await env.RELAY.get("shared-listings", "json");
  if (hit) return hit;
  const { file } = await readSharedListings(env);
  await env.RELAY?.put("shared-listings", JSON.stringify(file), { expirationTtl: 6 * 3600 });
  return file;
}

async function shareListing(env, request) {
  if (await limited(env, request, "listings", 20)) return problem(429, "rate_limited", "Too many listings shared in one hour. Please try again later.");
  const l = readListing(await request.json().catch(() => null));
  if (l.company_name.length < 2 || l.position_title.length < 2) return problem(400, "validation_failed", "A shared listing needs a company name and position title.");
  if (l.raw_text.length < 40) return problem(400, "validation_failed", "Include the job listing content so others can read and match against it.");
  const identity = l.listing_url
    ? l.listing_url.toLowerCase()
    : `${l.company_name.toLowerCase()}|${l.position_title.toLowerCase()}|${l.location.toLowerCase()}`;
  const id = (await sha256(identity)).slice(0, 16);
  const known = new Set(((await cachedSharedListings(env)).listings || []).map(x => x.id));
  if (known.has(id) || (env.RELAY && await env.RELAY.get(`listing:${id}`))) {
    return json({ status: "known", id });
  }
  const entry = { id, ...l, added: new Date().toISOString().slice(0, 10) };
  await env.RELAY?.put(`listing:${id}`, JSON.stringify(entry));
  return json({ status: "queued", id, listing: entry }, 201);
}

export async function publishListings(env) {
  if (!env.RELAY) return;
  const { keys } = await env.RELAY.list({ prefix: "listing:" });
  if (!keys.length) return;
  const queued = (await Promise.all(keys.map(k => env.RELAY.get(k.name, "json")))).filter(Boolean);
  const branch = "community-listings";
  const { data: repo } = await gh(env, `/repos/${env.REPO}`);
  const base = repo.default_branch;
  const { data: head } = await gh(env, `/repos/${env.REPO}/git/ref/heads/${base}`);
  const existing = await gh(env, `/repos/${env.REPO}/git/ref/heads/${branch}`);
  if (existing.status === 404) {
    await gh(env, `/repos/${env.REPO}/git/refs`, { method: "POST", body: JSON.stringify({ ref: `refs/heads/${branch}`, sha: head.object.sha }) });
  }
  const { data: current } = await gh(env, `/repos/${env.REPO}/contents/${LISTINGS_FILE}?ref=${branch}`);
  const file = current?.content ? JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(current.content.replace(/\s/g, "")), c => c.charCodeAt(0)))) : { version: 1, listings: [] };
  const have = new Set((file.listings || []).map(x => x.id));
  const added = queued.filter(x => !have.has(x.id));
  if (added.length) {
    file.listings = [...added, ...(file.listings || [])].sort((a, b) => String(b.added || "").localeCompare(String(a.added || "")) || a.company_name.localeCompare(b.company_name));
    file.updated = new Date().toISOString().slice(0, 10);
    const content = btoa(String.fromCharCode(...new TextEncoder().encode(`${JSON.stringify(file, null, 2)}\n`)));
    await gh(env, `/repos/${env.REPO}/contents/${LISTINGS_FILE}`, {
      method: "PUT",
      body: JSON.stringify({ message: `data: ${added.length} shared job listing${added.length === 1 ? "" : "s"}`, content, branch, ...(current?.sha ? { sha: current.sha } : {}) }),
    });
    const owner = env.REPO.split("/")[0];
    const { data: open } = await gh(env, `/repos/${env.REPO}/pulls?state=open&head=${owner}:${branch}`);
    if (!open?.length) {
      await gh(env, `/repos/${env.REPO}/pulls`, {
        method: "POST",
        body: JSON.stringify({ title: "Job listings shared from the app", head: branch, base, body: "External job listings (LinkedIn, Indeed, Workday, etc.) people shared from Fistbump with scrubbed content. Merge to make them available in Community resources." }),
      });
    }
  }
  await Promise.all(keys.map(k => env.RELAY.delete(k.name)));
}

// ---------- routing ----------

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    try {
      if (request.method === "GET" && url.pathname === "/v1/reports/similar") {
        if (await limited(env, request, "similar", 60)) return json({ reports: [] });
        return json({ reports: await similar(env, scrub(url.searchParams.get("q"), 200)) });
      }
      if (request.method === "POST" && url.pathname === "/v1/reports") return await report(env, request);
      if (request.method === "POST" && url.pathname === "/v1/boards") return await shareBoards(env, request);
      if (request.method === "GET" && url.pathname === "/v1/boards") return json(await cachedShared(env), 200, { "cache-control": "public, max-age=3600" });
      if (request.method === "POST" && url.pathname === "/v1/listings") return await shareListing(env, request);
      if (request.method === "GET" && url.pathname === "/v1/listings") return json(await cachedSharedListings(env), 200, { "cache-control": "public, max-age=3600" });
      if (url.pathname === "/") return json({ service: "fistbump-relay", ok: true });
      return problem(404, "not_found", "Nothing here.");
    } catch (err) {
      console.error(err);
      return problem(502, "unavailable", "The community service could not reach GitHub just now. Try again in a few minutes.");
    }
  },
  async scheduled(_event, env, ctx) {
    ctx.waitUntil(Promise.all([publishBoards(env), publishListings(env)]));
  },
};

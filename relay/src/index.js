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
const TOKEN = /^[a-z0-9][a-z0-9_-]{0,63}$/;
const URL_SHORTENERS = new Set([
  "bit.ly", "tinyurl.com", "t.co", "ow.ly", "goo.gl", "is.gd", "buff.ly",
  "rebrand.ly", "cutt.ly", "rb.gy", "shorturl.at", "tiny.cc", "lnkd.in",
]);

// ---------- small helpers ----------

export const json = (body, status = 200, extra = {}) => new Response(JSON.stringify(body), { status, headers: { ...JSON_HEADERS, ...extra } });
const problem = (status, code, message) => json({ error: { code, message } }, status);

// Levenshtein edit distance (bounded) to catch 1-char typosquats against known board tokens.
export function editDistance(a, b) {
  if (Math.abs(a.length - b.length) > 2) return 3;
  const prev = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 1; i <= a.length; i++) {
    let diag = prev[0];
    prev[0] = i;
    for (let j = 1; j <= b.length; j++) {
      const tmp = prev[j];
      prev[j] = Math.min(prev[j] + 1, prev[j - 1] + 1, diag + (a[i - 1] === b[j - 1] ? 0 : 1));
      diag = tmp;
    }
  }
  return prev[b.length];
}

export function isLikelyTyposquat(token, knownTokens) {
  if (token.length < 5) return false;
  for (const k of knownTokens) {
    if (k === token || k.length < 5) continue;
    if (editDistance(token, k) === 1) return true;
  }
  return false;
}

// Strips what should never leave someone's computer by accident: invisible/bidi/tag chars,
// email addresses, phone numbers, home folders.
export function scrub(text, max) {
  return (typeof text === "string" ? text : "")
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f\u200b-\u200f\u202a-\u202e\u2060-\u206f\ufeff]/g, "")
    .replace(/[\u{E0000}-\u{E007F}]/gu, "")
    .replace(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/gi, "[email removed]")
    .replace(/\+?\d[\d \t().-]{8,}\d/g, text =>
      text.replace(/\D/g, "").length >= 10 ? "[number removed]" : text)
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

const MAX_POST_BYTES = 192 * 1024;

async function readJsonBody(request) {
  if (!request.body) return null;
  const reader = request.body.getReader();
  const decoder = new TextDecoder();
  let size = 0;
  let text = "";
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > MAX_POST_BYTES) {
        await reader.cancel().catch(() => {});
        return null;
      }
      text += decoder.decode(value, { stream: true });
    }
    return JSON.parse(text + decoder.decode());
  } catch {
    return null;
  } finally {
    reader.releaseLock();
  }
}

async function limited(env, request, bucket, perHour) {
  if (!env.RELAY || !env.RATE_SALT) throw new Error("Relay rate limiting is not configured");
  const ip = request.headers.get("cf-connecting-ip") || "unknown";
  const hour = Math.floor(Date.now() / 3600000);
  const key = `rl:${bucket}:${hour}:${(await sha256(`${env.RATE_SALT || ""}:${hour}:${ip}`)).slice(0, 24)}`;
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
    redirect: "error",
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
    redirect: "error",
    headers: {
      authorization: `Bearer ${await githubToken(env)}`,
      accept: "application/vnd.github+json",
      "user-agent": "fistbump-relay",
      "x-github-api-version": "2022-11-28",
      ...(init.body ? { "content-type": "application/json" } : {}),
      ...init.headers,
    },
  });
  if (!res.ok && !(res.status === 404 && (!init.method || init.method === "GET"))) throw new Error(`GitHub ${init.method || "GET"} ${path}: ${res.status}`);
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
  const r = readReport(await readJsonBody(request));
  if (r.title.length < 4 || (r.details.length < 8 && !r.sameAs)) return problem(400, "validation_failed", "Say in a few words what happened.");
  r.fingerprint = (await sha256(words(`${r.title} ${r.details}`).sort().join(" "))).slice(0, 16);

  //The person said it is one of the reports they were shown.
  if (r.sameAs) {
    const { status, data } = await gh(env, `/repos/${env.REPO}/issues/${r.sameAs}`);
    if (status === 200 && data.state === "open" && !data.pull_request && (data.labels || []).some(l => l.name === LABEL)) {
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
    if (typeof entry !== "string") continue;
    const [ats, token, extra] = entry.toLowerCase().trim().split(":");
    if (extra === undefined && ATS.includes(ats) && TOKEN.test(token || "") && !RESERVED_SLUGS.has(token)) {
      out.push({ ats, token });
    }
  }
  return [...new Map(out.map(b => [`${b.ats}:${b.token}`, b])).values()];
}

// Asks the job board itself whether it exists, has at least one active open posting (blocking
// empty/placeholder/fake boards), and what the company calls itself.
export async function checkBoard({ ats, token }) {
  const url = ats === "greenhouse" ? `https://boards-api.greenhouse.io/v1/boards/${token}/jobs`
    : ats === "lever" ? `https://api.lever.co/v0/postings/${token}?mode=json&limit=1`
    : `https://api.ashbyhq.com/posting-api/job-board/${token}`;
  const res = await fetch(url, { headers: { "user-agent": "fistbump-relay" }, redirect: "error" });
  if (!res.ok) return null;
  const data = await res.json().catch(() => null);
  if (ats === "lever") {
    if (!Array.isArray(data) || data.length === 0) return null;
    return { ats, token, company: scrub(token, 80) };
  }
  if (ats === "greenhouse") {
    if (!Array.isArray(data?.jobs) || data.jobs.length === 0) return null;
    const meta = await fetch(`https://boards-api.greenhouse.io/v1/boards/${token}`, { headers: { "user-agent": "fistbump-relay" }, redirect: "error" }).then(r => r.ok ? r.json() : null).catch(() => null);
    return { ats, token, company: scrub(meta?.name || token, 80) };
  }
  if (!Array.isArray(data?.jobs) || data.jobs.length === 0) return null;
  return { ats, token, company: scrub(data?.organizationName || data?.name || token, 80) };
}

async function readCatalog(env, path, field, branch) {
  const suffix = branch ? `?ref=${encodeURIComponent(branch)}` : "";
  const root = `/repos/${env.REPO}`;
  const { status, data } = await gh(env, `${root}/contents/${path}${suffix}`, {
    headers: { accept: "application/vnd.github.object+json" },
  });
  if (status === 404) return { file: { version: 1, [field]: [] }, sha: null };
  if (!data?.sha || (data.type && data.type !== "file")) throw new Error("Invalid community catalog response");
  let content = data.content;
  if (data.encoding === "none") {
    const blob = await gh(env, `${root}/git/blobs/${encodeURIComponent(data.sha)}`);
    if (blob.data?.encoding !== "base64") throw new Error("Invalid community catalog blob");
    content = blob.data.content;
  }
  const bytes = Uint8Array.from(atob(content.replace(/\s/g, "")), c => c.charCodeAt(0));
  const file = JSON.parse(new TextDecoder().decode(bytes));
  if (file.version !== 1 || !Array.isArray(file[field])) throw new Error("Invalid community catalog schema");
  return { file, sha: data.sha };
}

async function shareBoards(env, request) {
  if (await limited(env, request, "boards", 30)) return problem(429, "rate_limited", "Too many boards in one hour. They will keep until later.");
  const boards = readBoards(await readJsonBody(request));
  if (!boards.length) return problem(400, "validation_failed", "Nothing to share: boards look like greenhouse:stripe or lever:netflix.");
  const sharedBoards = (await cachedShared(env)).boards || [];
  const known = new Set(sharedBoards.map(b => `${b.ats}:${b.token}`));
  const knownByAts = { greenhouse: [], lever: [], ashby: [] };
  for (const b of sharedBoards) if (Object.hasOwn(knownByAts, b.ats)) knownByAts[b.ats].push(b.token);
  const result = { queued: [], known: [], not_found: [] };
  for (const b of boards) {
    const key = `${b.ats}:${b.token}`;
    if (known.has(key) || (env.RELAY && await env.RELAY.get(`board:${key}`))) { result.known.push(key); continue; }
    if (isLikelyTyposquat(b.token, knownByAts[b.ats] || [])) { result.not_found.push(key); continue; }
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
  const { file } = await readCatalog(env, BOARDS_FILE, "boards");
  await env.RELAY?.put("shared-boards", JSON.stringify(file), { expirationTtl: 6 * 3600 });
  return file;
}

function encodeCatalog(file) {
  const bytes = new TextEncoder().encode(`${JSON.stringify(file, null, 2)}\n`);
  const chunks = [];
  for (let offset = 0; offset < bytes.length; offset += 8192) {
    chunks.push(String.fromCharCode(...bytes.subarray(offset, offset + 8192)));
  }
  return btoa(chunks.join(""));
}

export function publishBoards(env) {
  return publishQueue(env, {
    field: "boards",
    key: b => `${b.ats}:${b.token}`,
    path: BOARDS_FILE,
    prefix: "board:",
    compare: (a, b) => `${a.ats}:${a.token}`.localeCompare(`${b.ats}:${b.token}`),
    title: "Job boards shared from the app",
    description: "Review the shared ATS boards before making them available in Fistbump.",
  });
}

async function publishQueue(env, { field, key, path, prefix, compare, title, description }) {
  if (!env.RELAY) return;
  const { keys } = await env.RELAY.list({ prefix, limit: 40 });
  const pending = (await Promise.all(keys.map(async ({ name }) => ({
    name,
    item: await env.RELAY.get(name, "json"),
  })))).filter(({ item }) => item);
  if (!pending.length) return;

  const branch = `community-${field}`;
  const root = `/repos/${env.REPO}`;
  const { data: repo } = await gh(env, root);
  const base = repo.default_branch;
  const { status } = await gh(env, `${root}/git/ref/heads/${branch}`);
  if (status === 404) {
    const { data: head } = await gh(env, `${root}/git/ref/heads/${base}`);
    await gh(env, `${root}/git/refs`, {
      method: "POST",
      body: JSON.stringify({ ref: `refs/heads/${branch}`, sha: head.object.sha }),
    });
  }
  const { file, sha } = await readCatalog(env, path, field, branch);
  const have = new Set(file[field].map(key));
  const added = pending.map(({ item }) => item).filter(item => !have.has(key(item)));
  if (added.length) {
    file[field] = [...file[field], ...added].sort(compare);
    file.updated = new Date().toISOString().slice(0, 10);
    await gh(env, `${root}/contents/${path}`, {
      method: "PUT",
      body: JSON.stringify({
        message: `data: add ${added.length} community ${field}`,
        content: encodeCatalog(file), branch, ...(sha ? { sha } : {}),
      }),
    });
  }

  // If a previous run wrote the file but could not open its PR, retry that step.
  const owner = env.REPO.split("/")[0];
  const { data: open } = await gh(env, `${root}/pulls?state=open&head=${owner}:${branch}&base=${encodeURIComponent(base)}`);
  if (!Array.isArray(open)) throw new Error("Could not check for an open community pull request");
  if (!open.length) {
    const { data: comparison } = await gh(env, `${root}/compare/${encodeURIComponent(base)}...${branch}`);
    if (!comparison) throw new Error("Could not compare the community branch");
    if (comparison.ahead_by > 0) {
      await gh(env, `${root}/pulls`, {
        method: "POST",
        body: JSON.stringify({ title, head: branch, base, body: description }),
      });
    }
  }
  await Promise.all(pending.map(({ name }) => env.RELAY.delete(name)));
}

// ---------- community listings ----------

function currentListings(file) {
  const today = new Date().toISOString().slice(0, 10);
  return { ...file, listings: file.listings.filter(item =>
    parseIsoDate(item.expires_at) && item.expires_at >= today &&
    (!item.closes_at || (parseIsoDate(item.closes_at) && item.closes_at >= today))) };
}

function isHostMatch(host, domain) {
  return host === domain || host.endsWith("." + domain);
}

// Enforces HTTPS-only, rejects credentials, IP literals, localhost/local TLDs, punycode (IDN
// homographs like xn--), and URL shorteners to prevent phishing/typosquat links.
export function safeHttpsUrl(value, max) {
  const raw = typeof value === "string" ? value.trim() : "";
  if (!raw || raw.length > max) return "";
  try {
    const u = new URL(raw);
    if (u.protocol !== "https:") return "";
    if (u.username || u.password) return "";
    const host = u.hostname.toLowerCase().replace(/\.$/, "");
    if (!host || !host.includes(".") || host.includes("xn--")) return "";
    if (/^(\d+\.){3}\d+$/.test(host) || host.startsWith("[") || host === "localhost" || host.endsWith(".local") || host.endsWith(".internal")) return "";
    if (URL_SHORTENERS.has(host.replace(/^www\./, ""))) return "";
    u.hostname = host;
    return u.href;
  } catch {
    return "";
  }
}

export function siteFromUrl(value) {
  if (!value) return "";
  try {
    const host = new URL(value).hostname.replace(/^www\./i, "").toLowerCase();
    if (isHostMatch(host, "linkedin.com")) return "LinkedIn";
    if (isHostMatch(host, "indeed.com")) return "Indeed";
    if (isHostMatch(host, "glassdoor.com")) return "Glassdoor";
    if (isHostMatch(host, "myworkdayjobs.com") || isHostMatch(host, "workday.com")) return "Workday";
    if (isHostMatch(host, "wellfound.com") || isHostMatch(host, "angel.co")) return "Wellfound";
    if (isHostMatch(host, "ziprecruiter.com")) return "ZipRecruiter";
    if (isHostMatch(host, "smartrecruiters.com")) return "SmartRecruiters";
    if (isHostMatch(host, "icims.com")) return "iCIMS";
    if (isHostMatch(host, "jobvite.com")) return "Jobvite";
    if (isHostMatch(host, "greenhouse.io")) return "Greenhouse";
    if (isHostMatch(host, "lever.co")) return "Lever";
    if (isHostMatch(host, "ashbyhq.com")) return "Ashby";
    return host;
  } catch {
    return "";
  }
}

const RESERVED_SLUGS = new Set([
  "embed", "jobs", "apply", "api", "v1", "v0", "assets", "static", "favicon",
  "robots", "sitemap", "login", "signup", "auth", "cdn", "public", "app", "www", "boards", "careers",
]);
const ISO_DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const EVERGREEN_RE = /\b(evergreen|rolling\s+basis|rolling\s+applications?|accepted\s+on\s+a\s+rolling|continuous\s+recruitment|ongoing\s+recruitment|open\s+until\s+filled|always\s+hiring|general\s+application|talent\s+pool|pipeline\s+role)\b/i;
const MONTHS = {
  jan: 1, january: 1, feb: 2, february: 2, mar: 3, march: 3, apr: 4, april: 4,
  may: 5, jun: 6, june: 6, jul: 7, july: 7, aug: 8, august: 8,
  sep: 9, sept: 9, september: 9, oct: 10, october: 10, nov: 11, november: 11, dec: 12, december: 12,
};

function parseIsoDate(s) {
  if (typeof s !== "string" || !ISO_DATE_RE.test(s.trim())) return null;
  const [y, m, d] = s.trim().split("-").map(Number);
  const dt = new Date(Date.UTC(y, m - 1, d));
  if (dt.getUTCFullYear() !== y || dt.getUTCMonth() !== m - 1 || dt.getUTCDate() !== d) return null;
  return dt;
}

function addDaysIso(isoDate, days) {
  const dt = parseIsoDate(isoDate);
  if (!dt) return "";
  dt.setUTCDate(dt.getUTCDate() + days);
  return dt.toISOString().slice(0, 10);
}

// Extracts { ats, token } when a submitted listing URL belongs to a supported ATS board.
export function extractAtsBoard(rawUrl) {
  if (!rawUrl) return null;
  try {
    const u = new URL(rawUrl);
    if (u.protocol !== "https:") return null;
    const host = u.hostname.toLowerCase();
    const parts = u.pathname.split("/").filter(Boolean);
    let ats = "";
    let token = "";
    if (host === "boards.greenhouse.io" || host === "job-boards.greenhouse.io" || host === "boards.eu.greenhouse.io" || host === "job-boards.eu.greenhouse.io") {
      ats = "greenhouse";
      token = (parts[0] === "embed" ? u.searchParams.get("for") : parts[0]) || "";
    } else if (host === "jobs.lever.co" || host === "jobs.eu.lever.co") {
      ats = "lever";
      token = parts[0] || "";
    } else if (host === "jobs.ashbyhq.com") {
      ats = "ashby";
      token = parts[0] || "";
    }
    token = token.toLowerCase().trim();
    if (!ats || !TOKEN.test(token) || RESERVED_SLUGS.has(token)) return null;
    return { ats, token };
  } catch {
    return null;
  }
}

export function extractCloseDate(text) {
  const src = String(text || "");
  const cue = "(?:application\\s+deadline|apply\\s+by|closes?(?:\\s+on)?|closing\\s+date|deadline|accepting\\s+applications\\s+(?:through|until)|applications?\\s+close(?:\\s+on)?)";
  const iso = src.match(new RegExp(`${cue}[^\\n]{0,24}?(\\d{4}-\\d{2}-\\d{2})`, "i"));
  if (iso && parseIsoDate(iso[1])) return iso[1];
  const slash = src.match(new RegExp(`${cue}[^\\n]{0,24}?(\\d{1,2})\\/(\\d{1,2})\\/(\\d{4})`, "i"));
  if (slash) {
    const norm = `${slash[3]}-${String(slash[1]).padStart(2, "0")}-${String(slash[2]).padStart(2, "0")}`;
    if (parseIsoDate(norm)) return norm;
  }
  const word = src.match(new RegExp(`${cue}[^\\n]{0,24}?([A-Za-z]{3,9})\\s+(\\d{1,2})(?:st|nd|rd|th)?,?\\s+(\\d{4})`, "i"));
  if (word) {
    const m = MONTHS[word[1].toLowerCase()];
    if (m) {
      const norm = `${word[3]}-${String(m).padStart(2, "0")}-${String(word[2]).padStart(2, "0")}`;
      if (parseIsoDate(norm)) return norm;
    }
  }
  return "";
}

export function isLongRunningListing(title, text) {
  return EVERGREEN_RE.test(`${title || ""}\n${text || ""}`);
}

export function readListing(body) {
  const company_name = scrub(body?.company_name, 120);
  const position_title = scrub(body?.position_title, 160);
  const location = scrub(body?.location, 120);
  const work_mode = WORK_MODES.has(body?.work_mode) ? body.work_mode : "";
  const listing_url = safeHttpsUrl(body?.listing_url, 500);
  const company_url = safeHttpsUrl(body?.company_url, 300);
  const raw_text = scrub(body?.raw_text, 25000);
  const source = siteFromUrl(listing_url) || scrub(body?.source, 40) || "Community";
  const suppliedClose = parseIsoDate(body?.closes_at) ? body.closes_at.trim() : "";
  const extractedClose = extractCloseDate(raw_text);
  const closes_at = [suppliedClose, extractedClose].filter(Boolean).sort()[0] || "";
  const evergreen = isLongRunningListing(position_title, raw_text);
  return { source, company_name, position_title, location, work_mode, listing_url, company_url, raw_text, ...(closes_at ? { closes_at } : {}), ...(evergreen ? { evergreen: true } : {}) };
}

async function cachedSharedListings(env) {
  const hit = env.RELAY && await env.RELAY.get("shared-listings", "json");
  if (hit) return hit;
  const { file } = await readCatalog(env, LISTINGS_FILE, "listings");
  await env.RELAY?.put("shared-listings", JSON.stringify(file), { expirationTtl: 6 * 3600 });
  return file;
}

// If a submitted listing comes from an unknown supported ATS board, validate and queue that board too.
async function maybeQueueDiscoveredBoard(env, listing) {
  const candidate = extractAtsBoard(listing.listing_url) || extractAtsBoard(listing.company_url);
  if (!candidate) return "";
  const key = `${candidate.ats}:${candidate.token}`;
  const sharedBoards = (await cachedShared(env)).boards || [];
  const known = new Set(sharedBoards.map(b => `${b.ats}:${b.token}`));
  if (known.has(key) || (env.RELAY && await env.RELAY.get(`board:${key}`))) return "";
  const knownForAts = sharedBoards.filter(b => b.ats === candidate.ats).map(b => b.token);
  if (isLikelyTyposquat(candidate.token, knownForAts)) return "";
  const real = await checkBoard(candidate);
  if (!real) return "";
  await env.RELAY?.put(`board:${key}`, JSON.stringify({ ...real, added: new Date().toISOString().slice(0, 10) }));
  return key;
}

async function shareListing(env, request) {
  if (await limited(env, request, "listings", 20)) return problem(429, "rate_limited", "Too many listings shared in one hour. Please try again later.");
  const l = readListing(await readJsonBody(request));
  if (l.company_name.length < 2 || l.position_title.length < 2) return problem(400, "validation_failed", "A shared listing needs a company name and position title.");
  if (l.raw_text.length < 40) return problem(400, "validation_failed", "Include the job listing content so others can read and match against it.");
  const today = new Date().toISOString().slice(0, 10);
  if (l.closes_at && l.closes_at < today) {
    return problem(400, "listing_expired", "This job listing's closing date has already passed.");
  }
  const maxExpiry = addDaysIso(today, l.evergreen ? 120 : 45);
  const expires_at = l.closes_at && l.closes_at < maxExpiry ? l.closes_at : maxExpiry;

  const identity = l.listing_url
    ? l.listing_url
    : `${l.company_name.toLowerCase()}|${l.position_title.toLowerCase()}|${l.location.toLowerCase()}`;
  const id = (await sha256(identity)).slice(0, 16);
  const known = new Set(((await cachedSharedListings(env)).listings || []).map(x => x.id));
  const discovered_board = await maybeQueueDiscoveredBoard(env, l);
  if (known.has(id) || (env.RELAY && await env.RELAY.get(`listing:${id}`))) {
    return json({ status: "known", id, ...(discovered_board ? { discovered_board } : {}) });
  }
  const entry = { id, ...l, added: today, expires_at };
  await env.RELAY?.put(`listing:${id}`, JSON.stringify(entry));
  return json({ status: "queued", id, listing: entry, ...(discovered_board ? { discovered_board } : {}) }, 201);
}

export function publishListings(env) {
  return publishQueue(env, {
    field: "listings",
    key: listing => listing.id,
    path: LISTINGS_FILE,
    prefix: "listing:",
    compare: (a, b) => String(b.added || "").localeCompare(String(a.added || "")) || a.company_name.localeCompare(b.company_name),
    title: "Job listings shared from the app",
    description: "Review the shared external job listings before making them available in Fistbump.",
  });
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
      if (request.method === "GET" && url.pathname === "/v1/listings") return json(currentListings(await cachedSharedListings(env)), 200, { "cache-control": "public, max-age=3600" });
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

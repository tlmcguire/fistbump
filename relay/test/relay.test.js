// The relay against a pretend GitHub and pretend job boards: nothing leaves the test.
import test from "node:test";
import assert from "node:assert/strict";
import relay, { scrub, words, likeness, readReport, readBoards, issueBody } from "../src/index.js";

//a KV namespace in memory
const kv = () => {
  const m = new Map();
  return {
    get: async (k, type) => (m.has(k) ? (type === "json" ? JSON.parse(m.get(k)) : m.get(k)) : null),
    put: async (k, v) => { m.set(k, v); },
    delete: async k => { m.delete(k); },
    list: async ({ prefix }) => ({ keys: [...m.keys()].filter(k => k.startsWith(prefix)).map(name => ({ name })) }),
    m,
  };
};

const env = () => ({ REPO: "owner/app", GITHUB_TOKEN: "t", RATE_SALT: "s", RELAY: kv() });

//routes fetch to handlers by "METHOD path-prefix"
function fakeFetch(routes) {
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    const u = new URL(url);
    const key = `${init.method || "GET"} ${u.host}${u.pathname}`;
    calls.push({ key, url: String(url), body: init.body ? JSON.parse(init.body) : null });
    for (const [prefix, handler] of Object.entries(routes)) {
      if (key.startsWith(prefix)) {
        const [status, body] = handler(u, init);
        return new Response(JSON.stringify(body), { status });
      }
    }
    return new Response("{}", { status: 404 });
  };
  return calls;
}

const post = (path, body) => new Request(`https://relay.test${path}`, { method: "POST", body: JSON.stringify(body), headers: { "cf-connecting-ip": "1.2.3.4" } });

test("personal details are scrubbed before anything is sent", () => {
  const out = scrub("Mail me at ada@example.com or +1 (555) 123-4567, file in /Users/ada/Desktop", 500);
  assert.ok(!out.includes("ada@example.com"));
  assert.ok(!out.includes("555"));
  assert.ok(out.includes("/Users/[name]/Desktop"));
});

test("reports that say the same thing in other words look alike", () => {
  const a = words("Search results never load when I search for designer jobs");
  const b = words("search results won't load for designer");
  const c = words("The resume PDF export has the wrong font");
  assert.ok(likeness(a, b) > 0.62);
  assert.ok(likeness(a, c) < 0.25);
});

test("a report is read with limits and a known kind", () => {
  const r = readReport({ kind: "nope", title: "x".repeat(500), details: "d", app: { version: "0.5.0", os: "macOS" }, same_as: -3 });
  assert.equal(r.kind, "bug");
  assert.equal(r.title.length, 120);
  assert.equal(r.sameAs, null);
  assert.match(issueBody({ ...r, fingerprint: "f" }), /Fistbump 0\.5\.0 on macOS/);
});

test("a new report becomes an issue labelled as from the app", async () => {
  const calls = fakeFetch({
    "GET api.github.com/search/issues": () => [200, { items: [] }],
    "POST api.github.com/repos/owner/app/issues": () => [201, { number: 7, html_url: "https://github.com/owner/app/issues/7" }],
  });
  const e = env();
  const res = await relay.fetch(post("/v1/reports", { kind: "bug", title: "Tracker dates do not save", details: "I pick a date and it goes back", app: { version: "0.5.0", os: "macOS" } }), e);
  assert.equal(res.status, 201);
  assert.deepEqual(await res.json(), { status: "created", number: 7, url: "https://github.com/owner/app/issues/7" });
  const created = calls.find(c => c.key === "POST api.github.com/repos/owner/app/issues");
  assert.deepEqual(created.body.labels, ["from-the-app", "bug"]);

  //sent twice by mistake: the same issue, not a second one
  const again = await relay.fetch(post("/v1/reports", { kind: "bug", title: "Tracker dates do not save", details: "I pick a date and it goes back", app: {} }), e);
  assert.equal((await again.json()).status, "added");
  assert.equal(calls.filter(c => c.key === "POST api.github.com/repos/owner/app/issues").length, 1);
});

test("a report like an open issue is added to it instead of filed again", async () => {
  const calls = fakeFetch({
    "GET api.github.com/search/issues": () => [200, { items: [{ number: 3, title: "Search results never load for designer", body: "search results never load", html_url: "u3", comments: 1 }] }],
    "POST api.github.com/repos/owner/app/issues/3/comments": () => [201, {}],
    "POST api.github.com/repos/owner/app/issues/3/reactions": () => [201, {}],
  });
  const res = await relay.fetch(post("/v1/reports", { title: "Search results never load", details: "search results never load for designer jobs", app: {} }), env());
  assert.deepEqual(await res.json(), { status: "added", number: 3, url: "u3" });
  assert.ok(calls.some(c => c.key === "POST api.github.com/repos/owner/app/issues/3/comments"));
  assert.ok(!calls.some(c => c.key === "POST api.github.com/repos/owner/app/issues"));
});

test("too many reports from one place in an hour are turned away", async () => {
  fakeFetch({ "GET api.github.com/search/issues": () => [200, { items: [] }], "POST api.github.com/repos/owner/app/issues": () => [201, { number: 1, html_url: "u" }] });
  const e = env();
  let last;
  for (let i = 0; i < 7; i++) last = await relay.fetch(post("/v1/reports", { title: `Problem number ${i} here`, details: `details for problem ${i} xyz${i}`, app: {} }), e);
  assert.equal(last.status, 429);
});

test("shared boards are checked against the board itself and queued once", async () => {
  fakeFetch({
    "GET api.github.com/repos/owner/app/contents/data/community-boards.json": () => [200, { content: btoa(JSON.stringify({ version: 1, boards: [{ ats: "greenhouse", token: "known" }] })), sha: "x" }],
    "GET boards-api.greenhouse.io/v1/boards/stripe": () => [200, { name: "Stripe" }],
    "GET boards-api.greenhouse.io/v1/boards/nosuch": () => [404, {}],
  });
  assert.deepEqual(readBoards({ boards: ["greenhouse:Stripe", "bad entry", "workday:x", "greenhouse:stripe"] }), [{ ats: "greenhouse", token: "stripe" }]);
  const e = env();
  const res = await relay.fetch(post("/v1/boards", { boards: ["greenhouse:stripe", "greenhouse:known", "greenhouse:nosuch"] }), e);
  assert.deepEqual(await res.json(), { queued: ["greenhouse:stripe"], known: ["greenhouse:known"], not_found: ["greenhouse:nosuch"] });
  assert.equal(JSON.parse(e.RELAY.m.get("board:greenhouse:stripe")).company, "Stripe");
  const again = await relay.fetch(post("/v1/boards", { boards: ["greenhouse:stripe"] }), e);
  assert.deepEqual((await again.json()).known, ["greenhouse:stripe"]);
});

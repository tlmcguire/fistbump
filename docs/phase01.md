# Phase 1: Display one Greenhouse job listing

Build on `main` from the plans in [architecture.md](architecture.md), [api.md](api.md) and [schema.md](schema.md). `experiment/full-build` is a reference only; nothing is copied from it as final code.

## Goal

The user pastes a Greenhouse job link, and the app shows that job: title, company, location, link, and description. The data comes from the Go backend over the planned HTTP API.

The user never enters a board token or job id. The backend reads them from the link once and returns them with the posting; from then on the app returns to the listing by board and job id. Job search is always on; there is no setting to turn it off.

## Scope

In:

- Go backend process: flags, token, loopback listener, `PORT=` line, exit on stdin close.
- HTTP server scaffolding: router, bearer-token check, JSON and error helpers.
- `GET /v1/health` and the two routes below: open a pasted link, and fetch a listing by board and job id. Every other route in [api.md](api.md) is registered and returns `501 not_implemented`, so the frontend can wire against the full contract.
- Electron main process, preload bridge, and one renderer view.

Out (later phases): search, saving jobs to the database, parsing, skill-gap analysis, AI, resume, tracker, packaging.

## Routes

Both routes are new in this plan and are not in [api.md](api.md) yet. Both return the same posting and write nothing to the database. The backend fetches it from `https://boards-api.greenhouse.io/v1/boards/{board}/jobs/{job_id}`.

### `GET /v1/connectors/greenhouse/link?url=<encoded link>`

How a listing gets into the app. The backend reads the board and job id from the link, then fetches the posting.

```
GET /v1/connectors/greenhouse/link?url=https%3A%2F%2Fjob-boards.greenhouse.io%2Fbackblaze%2Fjobs%2F5287532008
```

Accepted links have the form `https://job-boards.greenhouse.io/{board}/jobs/{job_id}`, plus the older `boards.greenhouse.io` host. Links on a company's own site that carry only `?gh_jid=<job_id>` have no board token, so they return `422` with a message saying so.

### `GET /v1/connectors/greenhouse/boards/{board}/jobs/{job_id}`

How the app returns to a listing it already knows: reloading the view now, re-pulling a saved job from the tracker later (a `404` means the posting has closed), and opening a search result in phase 2.

### Response

```jsonc
// 200 (both routes)
{
  "external_id": "5287532008",
  "board": "backblaze",
  "company_name": "Backblaze",
  "position_title": "AI Workflow Engineer | LATAM",
  "location": "Remote - Argentina; Remote - Colombia",
  "listing_url": "https://job-boards.greenhouse.io/backblaze/jobs/5287532008",
  "updated_at": "2026-10-06T16:15:10Z",
  "raw_text": "Backblaze is the object storage leader..."
}
```

The frontend keeps `board` and `external_id` and uses the second route for every later request.

The field names match the posting in [api.md](api.md) (`POST /v1/connectors/{id}/fetch`) and, where a column exists, the `jobs` table in [schema.md](schema.md) (`external_id`, `company_name`, `position_title`, `location`, `listing_url`, `raw_text`). `board` and `updated_at` are posting-only: `jobs` has no column for either. `updated_at` is when Greenhouse last changed the posting.

| Status | `code` | When |
|--------|--------|------|
| 404 | `not_found` | Greenhouse returns 404: the job is closed or never existed |
| 422 | `validation_failed` | The link is not a Greenhouse job link, has no board token, or `board` or `job_id` is not valid |
| 502 | `upstream_error` | Greenhouse fails, times out, or rate limits |
| 501 | `not_implemented` | Any planned route not built yet |

Notes from a live check of the Greenhouse API (2026-10-10):

- `content` is HTML with entities escaped (`&lt;p&gt;...`). The backend unescapes it and converts it to plain text for `raw_text`, so the renderer never inserts HTML.
- `company_name` can be a label such as "Backblaze External Website". Use the company name from `boards.json` when the board is listed there, else Greenhouse's value.
- `updated_at` has a local offset. Convert to UTC, per the API conventions.
- A missing job returns `404`.

For later: saving a job must store its board token so the tracker can re-pull it. `jobs` has `external_id` and `listing_url` but no board column, and reading the board from `listing_url` breaks when Greenhouse changes its link format.

In phase 2, search returns results with their full data, so opening a result needs no request; this route still serves reloads and the tracker (see below).

## Files, in build order

Progress:

- [ ] 1. `connector.go`
- [ ] 2. `greenhouse.go`, `boards.go`
- [ ] 3. `respond.go`
- [ ] 4. `server.go`
- [ ] 5. `connectors.go`
- [ ] 6. `main.go`
- [ ] 7. `build-go.js`
- [ ] 8. `backend.js`
- [ ] 9. `ipc-handlers.js`
- [ ] 10. `preload.js`
- [ ] 11. `main.js`
- [ ] 12. Renderer files
- [ ] Tests and tooling

Each step is testable before the next one depends on it. Files that exist on `main` today: `backend/internal/db/` (database open and migrations), `backend/internal/connectors/greenhouse/boards.json`, `scripts/verify-greenhouse-boards.js`, and empty folders held open by `.gitkeep`. Delete a `.gitkeep` when its folder gets a real file.

### Backend

#### 1. `backend/internal/connectors/connector.go`

**Goal:** one Go type, `Posting`, that every job source returns, so the API and later the database never deal with Greenhouse's own field names.

- Fields match the response above: `ExternalID`, `Board`, `CompanyName`, `PositionTitle`, `Location`, `ListingURL`, `UpdatedAt` (`time.Time`, UTC), `RawText`. JSON tags use the API's snake_case names.
- Keep it free of HTTP and database code. Phase 2 adds the search fields api.md lists for postings (`work_mode`, `score`, `title_score`, `skill_score`), and pay once its fields are defined; api.md has no pay field for postings yet.

#### 2. `backend/internal/connectors/greenhouse/greenhouse.go` and `boards.go`

**Goal:** everything that knows about Greenhouse lives here: reading a pasted link, fetching one job, and turning the response into a `Posting`.

- **Read a link** (`ParseLink(raw string) (board, jobID string, err error)`): parse with `net/url`; accept hosts `job-boards.greenhouse.io` and `boards.greenhouse.io` and the path `/{board}/jobs/{job_id}`; the job id must be all digits. A link with only `gh_jid` (a company's own careers page) returns a specific error, because the board token is not in it.
- **Fetch one job** (`FetchJob(ctx, board, jobID) (Posting, error)`): `GET https://boards-api.greenhouse.io/v1/boards/{board}/jobs/{job_id}` with Go's `http.Client`. Set a timeout (for example 10 s), cap the response size with `io.LimitReader`, and send a descriptive `User-Agent`. Leave `Accept-Encoding` unset so Go asks for gzip and decompresses it for you. Map Greenhouse `404` to a not-found error and any other failure to an upstream error, so the API can choose the status.
- **Make the base URL a field** on a client struct (`Client{BaseURL, HTTP}`), so tests can point it at a fake server.
- **HTML to text:** `content` arrives HTML-escaped (`&lt;p&gt;`). Unescape with `html.UnescapeString`, then walk the HTML with `golang.org/x/net/html` and emit text, with line breaks for block tags (`p`, `li`, `br`, headings) and `- ` before list items. Never return HTML to the renderer. `x/net/html` is maintained by the Go team and is pure Go, so it fits the no-cgo rule.
- **Company name:** `boards.go` embeds `boards.json` with `go:embed` and looks up the board's curated `company`. Fall back to Greenhouse's `company_name`, which can be a label like "Backblaze External Website".
- **Times:** parse `updated_at` (it has a local offset) and convert to UTC.
- **Test** (`greenhouse_test.go`): an `httptest.Server` serves a saved real response from `backend/testdata/api/greenhouse_job.json`. Cover: link forms (new host, old host, `gh_jid`-only, not Greenhouse, non-numeric id), success mapping, HTML to text, company name fallback, UTC conversion, upstream `404` and `500`, an oversized body.

References: [Greenhouse Job Board API](https://developers.greenhouse.io/job-board.html), [net/url](https://pkg.go.dev/net/url), [html](https://pkg.go.dev/html), [golang.org/x/net/html](https://pkg.go.dev/golang.org/x/net/html), [embed](https://pkg.go.dev/embed), [net/http/httptest](https://pkg.go.dev/net/http/httptest).

#### 3. `backend/internal/api/respond.go`

**Goal:** one way to write responses, so every route returns the same JSON and error shape from [api.md](api.md).

- `writeJSON(w, status, v)`: sets `Content-Type: application/json` and encodes.
- `writeError(w, status, code, message, details)`: writes `{ "error": { "code", "message", "details" } }`.
- Helpers or error types for the codes phase 1 uses: `unauthorized` (401), `not_found` (404), `validation_failed` (422), `internal` (500), `not_implemented` (501), `upstream_error` (502).
- Handlers return an `error`, and one wrapper turns it into the right response. That keeps status-code decisions out of each handler.

#### 4. `backend/internal/api/server.go`

**Goal:** the HTTP server: routing, the token check, and the contract that unbuilt routes answer `501`.

- Use the standard library router (`http.ServeMux`). Since Go 1.22 it matches methods and path wildcards, for example `mux.HandleFunc("GET /v1/connectors/greenhouse/boards/{board}/jobs/{job_id}", ...)` with `r.PathValue("board")`. No router dependency is needed.
- **Token middleware:** compare `Authorization: Bearer <token>` with `crypto/subtle.ConstantTimeCompare`; return `401` otherwise. Every route goes through it.
- **Panic recovery** middleware returns `500 internal` and logs to stderr.
- **Logging** to stderr: method, path, status, duration. Never headers or bodies.
- `GET /v1/health` returns `{ "status": "ok", "version", "schema_version" }`, reading `PRAGMA user_version`.
- Register every other route in [api.md](api.md) with a handler that returns `501 not_implemented`, so the frontend gets the full contract now.

References: [net/http](https://pkg.go.dev/net/http), [Routing enhancements for Go 1.22](https://go.dev/blog/routing-enhancements), [crypto/subtle](https://pkg.go.dev/crypto/subtle).

#### 5. `backend/internal/api/connectors.go`

**Goal:** the two phase 1 routes, as thin handlers over step 2.

- `GET /v1/connectors/greenhouse/link?url=`: read the `url` query value, call `ParseLink`, then `FetchJob`. Bad or `gh_jid`-only links give `422` with a message the UI can show as written.
- `GET /v1/connectors/greenhouse/boards/{board}/jobs/{job_id}`: validate both values (board: lowercase letters, digits, `-`, `_`; job id: digits), then `FetchJob`.
- Map step 2's errors: not found to `404`, upstream to `502`, validation to `422`.
- **Test** (`api_test.go`): call the server with `httptest.NewRecorder` and a fake Greenhouse. Cover: missing or wrong token (`401`), both routes succeeding, `404`, `422` for bad and `gh_jid`-only links, `502`, and one unbuilt route returning `501`.

#### 6. `backend/cmd/fistbump-core/main.go`

**Goal:** the program Electron starts, following the process contract in [architecture.md](architecture.md).

- Read `--data-dir` (create it if missing) and `FISTBUMP_TOKEN` (exit with an error if empty).
- Open the database with `db.Open` at a file in the data directory. The docs do not name the file yet; this plan proposes `fistbump.db`. Phase 1 only reads its version for the health route.
- Listen on `127.0.0.1:0` so the OS picks a free port, then print exactly `PORT=<n>` on stdout. Everything else goes to stderr.
- Read stdin until it closes, then shut the server down gracefully (`http.Server.Shutdown` with a short timeout). This is how the backend stops when Electron quits or crashes.

References: [net/http Server.Shutdown](https://pkg.go.dev/net/http), [os/signal](https://pkg.go.dev/os/signal) (optional, for Ctrl-C when run by hand).

#### 7. `scripts/build-go.js`

**Goal:** `npm run build:go` builds the backend for this computer into `bin/<os>-<arch>/fistbump-core` (`.exe` on Windows).

- Map Node's `process.platform` and `process.arch` to Go's `GOOS` and `GOARCH` (`win32` to `windows`, `x64` to `amd64`).
- Run `go build` in `backend/` with `CGO_ENABLED=0`. `bin/` is gitignored.

**Checkpoint:** build, run `FISTBUMP_TOKEN=test bin/<os>-<arch>/fistbump-core --data-dir /tmp/fb`, then `curl -H "Authorization: Bearer test" "http://127.0.0.1:<port>/v1/connectors/greenhouse/link?url=<encoded link>"`. The backend is done before any Electron code exists.

### Electron

#### 8. `app/main/backend.js`

**Goal:** start and stop the backend, and make authenticated requests to it.

- Generate a random token per launch (`crypto.randomBytes`). Spawn the binary with `child_process.spawn`, passing `--data-dir` and `FISTBUMP_TOKEN` in its environment. Keep stdin as a pipe; closing it stops the backend.
- Read stdout line by line (`readline`) until `PORT=<n>`; fail with the stderr tail if it does not appear within 10 s.
- `request(method, path, body)`: uses Node's built-in `fetch` with the token header, and returns the JSON or the error shape unchanged.
- On quit, close stdin and wait up to 5 s before killing the process.

References: [child_process](https://nodejs.org/api/child_process.html), [readline](https://nodejs.org/api/readline.html).

#### 9. `app/main/ipc-handlers.js`

**Goal:** the only bridge from the window to the backend: each IPC channel maps to one HTTP call.

- `connectors:link` (url) to the link route, with the link URL-encoded; `connectors:posting` (board, jobId) to the listing route.
- Register with `ipcMain.handle`. Export the channel list so the test can compare it with the preload.

#### 10. `app/preload/preload.js`

**Goal:** expose a small, named API to the page and nothing else.

- `contextBridge.exposeInMainWorld('api', { connectors: { link: (url) => ipcRenderer.invoke('connectors:link', url), posting: (board, jobId) => ipcRenderer.invoke('connectors:posting', board, jobId) } })`.
- Never expose `ipcRenderer` itself, the port, or the token.

References: [contextBridge](https://www.electronjs.org/docs/latest/api/context-bridge), [Inter-process communication](https://www.electronjs.org/docs/latest/tutorial/ipc).

#### 11. `app/main/main.js`

**Goal:** the app's entry point (`"main"` in `package.json`).

- Single-instance lock. Start the backend, wait for `PORT=`, register the IPC handlers, then create the window.
- `BrowserWindow` with `contextIsolation: true`, `sandbox: true`, `nodeIntegration: false`, and the preload script. Load `app/renderer/index.html` from disk.
- Block navigation away from the app page and opening new windows; open external links in the system browser.
- Stop the backend on quit. If it fails to start, show an error page with the stderr tail.

References: [BrowserWindow](https://www.electronjs.org/docs/latest/api/browser-window), [Security checklist](https://www.electronjs.org/docs/latest/tutorial/security), [Process sandboxing](https://www.electronjs.org/docs/latest/tutorial/sandbox).

### Renderer

#### 12. `app/renderer/index.html`, `js/api.js`, `js/app.js`, `views/job-parser/job-parser.js`

**Goal:** the one screen: paste a link, see the listing.

- `index.html`: a content security policy that allows only the app's own files (`default-src 'self'`), with no remote scripts or inline scripts.
- `js/api.js`: wraps `window.api` and turns the error shape into readable messages.
- `js/app.js`: mounts the view. No router yet.
- `views/job-parser/job-parser.js`: a field and button to open a link; shows title, company, location, updated date, an "Open on Greenhouse" link, and the description. Set text with `textContent`, never `innerHTML`. Keep `board` and `external_id` and use them for a Reload button. Show errors as written by the backend (for example, the `gh_jid` message).
- Styles: optional for phase 1. [architecture.md](architecture.md) plans `styles/base.css` and `styles/components.css`.

References: [Content Security Policy (MDN)](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CSP).

### Tests and tooling

- `app/tests/ipc.test.js` (Node's built-in test runner): the preload and `ipc-handlers.js` list the same channels.
- `package.json`: `test` runs `node --test`; `dev` runs `build:go` then `electron .`. Pin the `electron` version instead of `"*"`.

References: [node:test](https://nodejs.org/api/test.html).

## Done when

- Pasting a Greenhouse job link shows the listing in the app, and reloading it uses the board and job id.
- A closed job, a link that is not a Greenhouse job link, a bad token, and an unreachable Greenhouse each show a clear message.
- `go test ./...` and `npm test` pass.

## Phase 2 preview

Search. The frontend asks for N listings; the backend finds them and returns each with its full data, so the search tab can show and open results without more requests.

```jsonc
// POST /v1/connectors/greenhouse/fetch
{ "query": { "keywords": ["data analyst"], "location": "Charleston, SC", "work_mode": "Any",
             "categories"?: [], "boards"?: [], "limit"?: 20,
             "company"?: "", "pay_min"?: null } }

// 200
{ "postings": [ /* N postings in this phase's shape plus "work_mode", "score", "title_score", "skill_score"; raw_text filled for every posting */ ],
  "matched", "boards_searched", "boards_total", "postings_scanned", "errors": [{ "board", "message" }], "keywords_used", "work_mode_used" }
```

The request and response fields are the ones [api.md](api.md) lists for this route, except two proposed additions that api.md does not have yet: `company` (search one company's boards) and `pay_min` (a minimum pay filter). Neither has a column or setting behind it; both are request-only. One change from api.md: it fills `raw_text` only for top-ranked postings, and this plan fills it for all N.

How the backend gets there:

1. **Choose boards.** All curated boards plus the user's custom boards, narrowed by `categories` or `company` when given.
2. **List each board without descriptions:** one request per board, about 2 KB gzipped, cached on disk and refreshed with `If-None-Match` (an unchanged board returns an empty `304`).
3. **Filter and rank locally** on title, location and work mode. Greenhouse has no search parameters; it always returns a board's full list.
4. **Fetch full data for the top N only**, reusing step 2's `FetchJob` with a concurrency limit and a cache. With `?pay_transparency=true`, Greenhouse adds structured `pay_input_ranges`; in a sample of 30 boards, 13% of jobs had it.
5. Return the N postings with `raw_text`. Opening one needs no further request; the phase 1 listing route still serves reloads and the tracker.

The backend caps `limit` so a request cannot fan out into hundreds of job fetches. The cap is to be set in phase 2.

The request shape above is fixed now so the frontend can build against it, and so phase 2 can parse and validate it field by field. In phase 1 the route returns `501`. Still open for phase 2: the cap on `limit`, how `pay_min` treats jobs with no structured pay, and how ranking weighs each field.

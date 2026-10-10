# Phase 1: Display one Greenhouse job listing

Build on `main` from the plans in [architecture.md](architecture.md), [api.md](api.md) and [schema.md](schema.md). `experiment/full-build` is a reference only; nothing is copied from it as final code.

## Goal

The user enters a Greenhouse board token and job id, or pastes a Greenhouse job URL, and the app shows that job: title, company, location, link, and description. The data comes from the Go backend over the planned HTTP API.

## Scope

In:

- Go backend process: flags, token, loopback listener, `PORT=` line, exit on stdin close.
- HTTP server scaffolding: router, bearer-token check, JSON and error helpers.
- `GET /v1/health` and the direct listing route below. Every other route in [api.md](api.md) is registered and returns `501 not_implemented`, so the frontend can wire against the full contract.
- Electron main process, preload bridge, and one renderer view.

Out (later phases): search, saving jobs to the database, parsing, skill-gap analysis, AI, resume, tracker, packaging.

## Route

`GET /v1/connectors/greenhouse/boards/{board}/jobs/{job_id}`

Fetches one job from `https://boards-api.greenhouse.io/v1/boards/{board}/jobs/{job_id}`. No database writes.

```jsonc
// 200
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

| Status | `code` | When |
|--------|--------|------|
| 404 | `not_found` | Greenhouse returns 404 for the board or job |
| 409 | `conflict` | `connectors.greenhouse.enabled` is `false` |
| 422 | `validation_failed` | `board` is not a valid token, or `job_id` is not a number |
| 502 | `upstream_error` | Greenhouse fails, times out, or rate limits |
| 501 | `not_implemented` | Any planned route not built yet |

Notes from a live check of the Greenhouse API (2026-10-10):

- `content` is HTML with entities escaped (`&lt;p&gt;...`). The backend unescapes it and converts it to plain text for `raw_text`, so the renderer never inserts HTML.
- `company_name` can be a label such as "Backblaze External Website". Use the company name from `boards.json` when the board is listed there, else Greenhouse's value.
- `updated_at` has a local offset. Convert to UTC, per the API conventions.
- A missing job returns `404`.

This is the same posting shape search will return in phase 2 (with scores, and `raw_text` only for top results), and search results will open with this route.

## Files, in build order

Each step is testable before the next one depends on it.

### Backend

- [ ] 1. `backend/internal/connectors/connector.go`: the normalized `Posting` type.
- [ ] 2. `backend/internal/connectors/greenhouse/greenhouse.go`: fetch one job over HTTPS with a timeout and a response size cap; HTML to text; company name lookup in `boards.json`. Test: `greenhouse_test.go` with an `httptest` server and a saved response in `backend/testdata/api/`.
- [ ] 3. `backend/internal/api/respond.go`: JSON writer, error shape and codes from api.md, `501` helper.
- [ ] 4. `backend/internal/api/server.go`: router, bearer-token middleware (constant-time compare), panic recovery, `GET /v1/health`, every planned route registered as `501`.
- [ ] 5. `backend/internal/api/connectors.go`: the listing route; reads `connectors.greenhouse.enabled`. Test: `api_test.go` (token required, 404, 409, 422, 501, success with a fake Greenhouse).
- [ ] 6. `backend/cmd/fistbump-core/main.go`: `--data-dir`, `FISTBUMP_TOKEN`, `db.Open`, listen on `127.0.0.1:0`, print `PORT=<n>`, exit when stdin closes.
- [ ] 7. `scripts/build-go.js`: build the backend into `bin/<os>-<arch>/` (`npm run build:go` already points here).

Checkpoint: run the binary and fetch a listing with `curl` and the token.

### Electron

- [ ] 8. `app/main/backend.js`: spawn the backend, read `PORT=`, send the token on every request, stop on quit.
- [ ] 9. `app/main/ipc-handlers.js`: channel `connectors:posting` to the listing route.
- [ ] 10. `app/preload/preload.js`: `window.api.connectors.posting(board, jobId)` through `contextBridge`.
- [ ] 11. `app/main/main.js`: sandboxed window, start the backend before showing it, register IPC handlers.

### Renderer

- [ ] 12. `app/renderer/index.html` (content security policy, no remote scripts), `js/api.js`, `js/app.js`, `views/job-parser/job-parser.js`: board and job id inputs, or a pasted Greenhouse job URL split into both; show the listing as text; show errors from the error shape.

### Tests and tooling

- [ ] `app/tests/ipc.test.js`: preload and main list the same channels.
- [ ] `package.json`: `test` runs the JS tests; `dev` builds the backend, then starts Electron.

## Done when

- Entering a known board and job id, or pasting its URL, shows the listing in the app.
- A missing job, a bad token, and an unreachable Greenhouse each show a clear message.
- `go test ./...` and `npm test` pass.

## Phase 2 preview

Search: `POST /v1/connectors/greenhouse/fetch` returns `{ "postings": [...] }` using the same posting shape. Its second stage (descriptions for the best matches) reuses step 2's single-job fetch, and clicking a result opens this phase's view.

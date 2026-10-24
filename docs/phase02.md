# Phase 2: Job search

Build on `main` after [phase 1](phase01.md) merges, from the plans in [architecture.md](architecture.md) ("Searching Greenhouse"), [api.md](api.md) and the phase 2 preview in [phase01.md](phase01.md). Overview and schedule: [roadmap.md](roadmap.md).

## Goal

The user enters keywords, a location and a work mode, presses Search, and gets a ranked list of Greenhouse postings. Each result arrives with its full description, so opening one shows it at once in phase 1's listing view with no further request.

Search runs only when the user presses Search. Opening the view sends nothing.

## Scope

In:

- `POST /v1/connectors/greenhouse/fetch` and `GET /v1/connectors`.
- Listing every curated board without descriptions, with a disk cache refreshed by `If-None-Match`.
- Local title, location and work mode filtering, ranking, and full data for the top N.
- A Find tab in the Jobs view, next to phase 1's Open link tab.

Out (later phases): skill fit in ranking (phase 4, needs the parser and the master resume), saving a result (phase 4), category and custom board settings (after the MVP), pay filtering (see "Decisions").

## Routes

### `POST /v1/connectors/greenhouse/fetch`

api.md writes this route as `/v1/connectors/{id}/fetch`. Greenhouse is the only connector, so the handler is registered at the literal path; `{id}` returns when a second connector exists.

```jsonc
// Request
{ "query": {
    "keywords": ["data analyst"],      // optional; defaults below
    "location": "Charleston, SC",      // optional
    "work_mode": "Any",                // Remote | Hybrid | On-site | Any (default Any)
    "categories": ["finance"],         // optional; narrows the board set
    "boards": ["acme"],                // optional; overrides the board set
    "company": "",                     // optional; proposed, see Decisions
    "limit": 20 } }                    // optional; default 20, at most 50

// 200
{ "postings": [{
    "external_id": "5287532008", "board": "backblaze", "company_name": "Backblaze",
    "position_title": "Data Analyst", "location": "Remote - US",
    "listing_url": "https://job-boards.greenhouse.io/backblaze/jobs/5287532008", "company_url": null,
    "updated_at": "2026-10-06T20:15:10Z", "raw_text": "...",
    "work_mode": "Remote", "score": 0.82, "title_score": 0.9, "skill_score": 0 }],
  "matched": 143,                // postings that passed the filters
  "boards_searched": 636,
  "boards_total": 636,
  "postings_scanned": 41210,
  "errors": [{ "board": "oldco", "message": "board not found" }],
  "keywords_used": ["data analyst"],
  "work_mode_used": "Any" }
```

- **Keywords** default to the request's `keywords`, else `profile.target_positions`, else the most recent experience title, as in architecture.md. Phase 3 builds the profile; until it merges, a request with no keywords returns `422` "Enter keywords to search".
- **Postings** use phase 1's shape plus `work_mode`, `score`, `title_score` and `skill_score`. `raw_text` is filled for every returned posting. `skill_score` is `0` in this phase.
- **Partial failure.** A board that fails or returns `404` is listed in `errors`; the search still succeeds. The search fails with `502` only when every board fails.

| Status | `code` | When |
|--------|--------|------|
| 422 | `validation_failed` | Unknown field, `limit` outside 1 to 50, unknown `work_mode` or category, a board token that is not valid, no keywords |
| 502 | `upstream_error` | Every board failed |

### `GET /v1/connectors`

```jsonc
// 200
[{ "id": "greenhouse", "name": "Greenhouse" }]
```

### api.md changes in this phase

- Write the fetch route as the literal Greenhouse path, add `company`, set `limit` to default 20 and at most 50, and say `raw_text` is filled for every returned posting.
- Remove `GET /v1/connectors/greenhouse/postings/{external_id}`. Results carry `raw_text`, and phase 1's listing route covers reloads.
- Add phase 1's two routes if phase 1 did not.

## How search works

1. **Choose boards.** `boards` if given; else every curated board in `boards.json` plus `connectors.greenhouse.boards`, narrowed to `categories` (a parent includes its subsets) when given; else narrowed to boards whose curated company name contains `company`. De-duplicate by token.
2. **List each board** with `GET /v1/boards/{board}/jobs` (no `content`): id, title, location, `updated_at`, `absolute_url`. At most 24 boards at once. `absolute_url` gives each result its `company_url` by phase 1's rules, with no extra request.
3. **Cache.** Keep each board's listing in memory and in `<data-dir>/cache/greenhouse/<board>.json` with its `ETag` and fetch time. Within 6 hours, use the cache without a request. After that, send `If-None-Match`; a `304` reuses the file. If Greenhouse sends no `ETag`, fall back to the 6 hour expiry alone. A `404` board is remembered for the cache window and reported in `errors`.
4. **Filter** on title, location and work mode (see `match.go` below).
5. **Rank** by title fit and location. No company takes more than 3 of the top N.
6. **Fetch full data for the top N** with phase 1's `FetchJob`, at most 8 at once, each with phase 1's timeout, and keep descriptions in memory for 24 hours. A posting whose detail fetch fails is dropped and its board listed in `errors`, so every returned posting has `raw_text`.

## Files, in build order

Progress:

- [x] 1. `connector.go` search fields
- [x] 2. `cache.go`
- [x] 3. `list.go`
- [x] 4. `match.go`
- [x] 5. `search.go`
- [x] 6. `connectors.go` handlers
- [x] 7. `main.go` wiring
- [x] 8. `ipc-handlers.js`, `preload.js`
- [x] 9. Find tab
- [x] 10. api.md
- [x] Tests and tooling

### Backend

#### 1. `backend/internal/connectors/connector.go`

**Goal:** add the search fields and the query type, still free of HTTP and Greenhouse names.

- Add to `Posting`: `WorkMode string`, `Score`, `TitleScore`, `SkillScore float64`, with JSON tags `work_mode`, `score`, `title_score`, `skill_score`. Phase 1's routes leave them zero; mark them `omitempty` so phase 1's response does not change.
- Add `Query` with the request fields above and `SearchResult` with `Postings` and the counters. `Validate()` on `Query` checks `limit`, `work_mode` and token format, so the API layer only maps the error.

#### 2. `backend/internal/connectors/greenhouse/cache.go`

**Goal:** one place for the listing cache (memory and disk) and the description cache (memory), so search and tests can control time.

- `type Cache struct { Dir string; Now func() time.Time; ... }`. `Now` defaults to `time.Now`; tests replace it to expire entries.
- Listing entry: `{ "board", "etag", "fetched_at", "jobs": [...] }`. Write to a temp file in the same folder, then `os.Rename`, so a crash never leaves half a file.
- Description entries: a map with a mutex, keyed by `board/job_id`, expiring after 24 hours.
- `Clear()` removes both. Phase 9's storage cleanup calls it.

References: [os.Rename](https://pkg.go.dev/os#Rename), [sync](https://pkg.go.dev/sync).

#### 3. `backend/internal/connectors/greenhouse/list.go`

**Goal:** list one board's postings without descriptions, through the cache.

- `ListBoard(ctx, board) ([]Listing, error)` on phase 1's `Client`. `Listing` holds `ID`, `Title`, `Location`, `UpdatedAt`, `AbsoluteURL`.
- Reuse phase 1's link rules (one function for `listing_url` and `company_url`), so search results and opened listings always agree.
- Use the cache as described in "How search works". Same timeout, size cap and `User-Agent` as `FetchJob`; boards are larger than one job, so cap at 8 MB.
- Map `404` to a not-found error and other failures to an upstream error, as in phase 1.

References: [Greenhouse Job Board API](https://developers.greenhouse.io/job-board.html), [HTTP conditional requests (MDN)](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Conditional_requests).

#### 4. `backend/internal/connectors/greenhouse/match.go`

**Goal:** the deterministic rules that decide whether a listing matches and how well, kept in pure functions so they are easy to test.

- **Title score** (0 to 1): lower-case and split into words; drop seniority words (`senior`, `sr`, `junior`, `jr`, `lead`, `principal`, `staff`, `i`, `ii`, `iii`); keep `intern`, which changes the job; fold synonyms (`developer` and `engineer`, `front end` and `frontend`, `back end` and `backend`). Score is the fraction of keyword words found in the title; an exact phrase match scores 1. A title must score at least 0.5 to pass.
- **Work mode** from the location and title: `Remote` only when the text says remote; `Hybrid` when it says hybrid; otherwise `On-site`. A request for `Remote` keeps only Remote; `On-site` and `Hybrid` keep their own mode plus postings that do not say.
- **Location**: case-insensitive match of the request's city, state or country terms against the posting location. Remote postings pass a location filter unless `work_mode` is `On-site`.
- **Location score**: 1 when the location matches, 0.5 for remote, 0 otherwise.
- `score = 0.7 * title_score + 0.3 * location_score` in this phase. Phase 4 adds `skill_score` and re-weights.

#### 5. `backend/internal/connectors/greenhouse/search.go`

**Goal:** run the six steps in "How search works" and return a `SearchResult`.

- `Search(ctx, q connectors.Query, boards []string) (connectors.SearchResult, error)`. The caller (API layer) builds the board list from `boards.json` and settings, so `search.go` has no settings dependency.
- Concurrency: a buffered channel of 24 slots for listing, 8 for details, with `sync.WaitGroup`. No new dependency is needed.
- Stop early when `ctx` is canceled (the user closed the view or the request timed out). Give the whole search a 20 s deadline; boards not listed in time go into `errors` as "timed out".
- Sort by `score`, then `updated_at` (newest first), then company and title, so equal inputs always give the same order.
- Company variety: walk the sorted list and skip a posting once its company has 3 in the result.
- Fill `matched`, `boards_searched`, `boards_total`, `postings_scanned`.

References: [context](https://pkg.go.dev/context), [sort.SliceStable](https://pkg.go.dev/sort#SliceStable).

#### 6. `backend/internal/api/connectors.go`

**Goal:** thin handlers for the two routes.

- Fetch: decode with unknown fields rejected (`json.Decoder.DisallowUnknownFields`), call `Query.Validate()`, choose the board list, call `Search`. Return `422` with the validation message, `502` only when every board failed.
- `GET /v1/connectors` returns the static list.
- Keyword defaults: in this phase only request keywords are used. Phase 3 adds the profile fallback here.

#### 7. `backend/cmd/fistbump-core/main.go`

**Goal:** give the Greenhouse client its cache folder.

- Create `<data-dir>/cache/greenhouse/` at startup and pass it to the `Cache`.

### Electron

#### 8. `app/main/ipc-handlers.js` and `app/preload/preload.js`

- `connectors:search` (query) calls the fetch route; `connectors:list` calls `GET /v1/connectors`.
- Preload adds `api.connectors.search(query)` and `api.connectors.list()`. Keep the channel list test passing.
- A search can take several seconds cold. Leave the request without a client timeout shorter than the backend's 20 s deadline.

### Renderer

#### 9. `app/renderer/views/job-parser/job-parser.js` (Find tab)

**Goal:** search and open results in the existing view.

- Tabs: Find and Open link (phase 1). Find is the default tab, and opening it sends nothing.
- Form: keywords, location, work mode (Any, Remote, Hybrid, On-site), Search button. Disable the button while a search runs; show "Searching N boards" until results arrive.
- Results: title, company, location, work mode, updated date, and a link to the company's site when `company_url` is set. Selecting one shows it with phase 1's listing display, from the result's own data. Reload still uses the listing route.
- Show the counts ("143 matches from 41,210 postings on 636 boards") and, if `errors` is not empty, a collapsed list of boards that failed.
- `textContent` only, as in phase 1.

### Docs

#### 10. `docs/api.md`

Apply the changes in "api.md changes in this phase" in the same pull request.

### Tests and tooling

- `backend/testdata/api/`: save two or three real board listings (`greenhouse_board_<token>.json`) and their job details. Keep them small: trim each listing to about 20 postings.
- `match_test.go`: seniority words, synonyms, exact phrase, each work mode rule, location with and without remote.
- `cache_test.go`: fresh entry used without a request; expired entry sends `If-None-Match`; `304` reuses the file; no `ETag` falls back to expiry; atomic write leaves no temp files.
- `search_test.go` with an `httptest.Server`: ranking order is stable, company variety cap, one failing board lands in `errors`, every board failing returns an error, `limit` respected, a detail failure drops that posting, context cancel stops work.
- `api_test.go`: fetch success, `422` cases, and that `GET /v1/connectors/greenhouse/postings/x` is gone (`404` or `405`).
- Manual: one live search with the cache empty, then the same search again; the second makes no listing requests (check the backend log).

## Changes made while building

The build went past the plan in these places. The decisions table below is the plan; this is what shipped.

- **Pay.** `pay_min`, `pay_max` and `require_pay` filter on a range read from description text, in US dollars only. A pay filter reads up to 300 descriptions in rank order, so `matched` can be a lower bound (`partial`). Postings that list no pay are kept unless `require_pay` is set.
- **Company, industry and location alone.** Keywords are optional when `company`, `categories`, `boards` or `location` narrows the search. A company that is not curated is tried as a Greenhouse board token. `GET /v1/connectors/greenhouse/categories` is read-only and lists industries, companies and the board total.
- **Paging.** `offset`, `has_more` and `next_offset`, with numbered pages in the app. Each company's first three postings rank ahead of its later ones, and nothing is dropped.
- **Other preferences.** `level`, `posted_within`, `exclude` and `sort`. Duplicate roles at one company fold into one result.
- **Cache.** Listings and descriptions are stored in `greenhouse_listing_cache` and `greenhouse_description_cache`, not files. Rows older than a day are deleted at launch and every hour.
- **Logos.** `GET /v1/connectors/greenhouse/logo` reads an icon from the company own site (never a third-party icon service), with address checks, size limits and a one-day cache in `logo_cache`.
- **Cancel.** The app cancels a search in progress when a new one starts or when the user presses Cancel.

## Done when

- A search over all curated boards returns ranked results with descriptions, and opening one shows it with no further request.
- A second search within 6 hours makes no listing requests.
- A failing board appears in `errors` without failing the search.
- Opening the Find tab sends no request.
- `go test ./...` and `npm test` pass.

## Decisions

Decide before step 5. The recommendation is in bold.

| Question | Options |
|----------|---------|
| `limit` cap | **Default 20, at most 50** (50 detail fetches at 8 at a time is about 2 to 4 s); or higher with paging |
| `company` field | **Keep**: filters curated boards by company name, no network cost; or drop |
| `pay_min` field | **Drop for the MVP**: only about 13% of jobs carry structured pay, so most results would be dropped or ignored. Revisit with pay parsing (build-findings 8.1) |
| Boards outside `boards.json` | **Only through `boards` in the request** for now; the custom board routes come after the MVP |

# Phase 4: Saved jobs and skill-gap analysis

Build on `main` after [phase 3](phase03.md) merges, from [api.md](api.md) ("Jobs", "Connectors") and [schema.md](schema.md). Overview and schedule: [roadmap.md](roadmap.md).

## Goal

The user saves a job, either by pasting its text or from a Greenhouse listing (phase 1's link view or phase 2's search), and sees which of the job's required and preferred skills the master resume shows and which it is missing. This meets FR-2, the job part of FR-5, and NFR-3 (analysis under 1.5 s).

Analysis is deterministic. It never calls a model.

## Scope

In:

- The `parser` package: posting text cleanup, skill and requirement extraction, inferred company, title, location and work mode.
- The `analyze` package: required and preferred skills compared with the master resume, with evidence.
- The `jobs` store and seven Jobs routes: `parse`, create, list, get, update, delete, `analyze`.
- Saving a Greenhouse posting as a job, and the `jobs.board` and `jobs.company_url` columns.
- `skill_score` in phase 2's search ranking.
- Save, a saved jobs list, and a skill-gap panel in the Jobs view.

Out: job archive and trash (phase 7), pay extraction and non-tech skill dictionaries (after the MVP, build-findings 8.1 and 8.2), suggestions (phase 5).

## Schema change

Add to `jobs` in `001_init.sql`, on a `feature/` branch with [schema.md](schema.md) and the Job shape in [api.md](api.md) updated in the same change:

```sql
board       TEXT,  -- Greenhouse board token; NULL for pasted and manual jobs
company_url TEXT,  -- direct link to the company (its own job page or site); NULL when unknown
```

No database has been released, so this edits the initial schema, as #4 did. `(source, external_id)` is already unique, so saving the same posting twice updates one row.

`company_url` keeps phase 1's direct link to the company, so a saved job is not only reachable through Greenhouse. For Greenhouse jobs it comes from the posting; for pasted and manual jobs the user may enter it. `listing_url` stays the page the job came from (the Greenhouse listing, or the link the user pasted).

## Routes

The seven Jobs routes are as written in [api.md](api.md). Notes:

```jsonc
// POST /v1/jobs/parse   { "raw_text": "Data Analyst\nAcme Corp\nCharleston, SC (Hybrid)\n\nRequirements\n- 2+ years of SQL\n- Excel..." }
// 200
{ "description": "...", "req_tech_skills": ["sql", "excel"], "pref_tech_skills": ["python"],
  "req_soft_skills": ["communication"], "pref_soft_skills": [],
  "requirements": ["2+ years of SQL", "..."],
  "inferred": { "company_name": "Acme Corp", "position_title": "Data Analyst", "location": "Charleston, SC", "work_mode": "Hybrid" } }

// POST /v1/jobs   { "source": "pasted", "raw_text": "..." }
// 201: Job, with missing company_name and position_title taken from "inferred";
//      422 if they are still empty after inference

// POST /v1/jobs/1/analyze
// 200: the analysis shape in api.md, with "duration_ms"
```

- `PUT /v1/jobs/{id}` that changes `raw_text` re-runs the parser, unless the request also sets the skill lists.
- `DELETE /v1/jobs/{id}` returns `409` with `details.application_id` when an application references the job. `applications` exists in the schema already; the foreign key's `RESTRICT` raises the conflict.
- `GET /v1/jobs` supports `archived` and `trashed` now (the columns exist); the routes that set them come in phase 7.

### Saving a Greenhouse posting

api.md describes `POST /v1/connectors/{id}/import` taking `external_ids` from recent search results. This plan changes it to take board and job id pairs, which matches phase 1's model and needs no search cache:

```jsonc
// POST /v1/connectors/greenhouse/import
{ "postings": [{ "board": "backblaze", "external_id": "5287532008" }] }   // 1 to 50 items

// 200
{ "imported": [Job], "refreshed": [Job] }
```

- Each posting is fetched with phase 1's `FetchJob` (the description cache from phase 2 makes this free for search results), parsed, and upserted on `(source, external_id)` with `source = 'greenhouse'`, `board` and `company_url` set.
- A new row goes in `imported`; an existing row gets new text, skills and `searched_at` and goes in `refreshed`.
- A posting that returns `404` fails the whole request with `404` and `details.external_id`, so the user knows which one closed. Nothing is written.
- Update api.md with this shape.

| Status | `code` | When |
|--------|--------|------|
| 404 | `not_found` | Unknown job id; a Greenhouse posting that has closed |
| 409 | `conflict` | Deleting a job that has an application |
| 422 | `validation_failed` | Missing `raw_text` for `pasted`, company or title empty after inference, bad enum, unknown field |
| 502 | `upstream_error` | Greenhouse fails during import |

## Files, in build order

Progress:

- [ ] 1. Schema: `jobs.board`
- [ ] 2. `models.go` Job
- [ ] 3. `skills.go`
- [ ] 4. `parser.go`
- [ ] 5. `analyze.go`
- [ ] 6. `job.go` (store)
- [ ] 7. `jobs.go` (api)
- [ ] 8. `connectors.go` import
- [ ] 9. Search `skill_score`
- [ ] 10. `ipc-handlers.js`, `preload.js`
- [ ] 11. Jobs view: Paste, Saved, analysis panel
- [ ] Tests and tooling

### Backend

#### 1. `backend/internal/db/migrations/001_init.sql`

Add `board` and `company_url` as above. Update `schema.md`, the Job shape in `api.md`, and `POST /v1/jobs` and `PUT /v1/jobs/{id}` to accept `company_url` (an `https` URL, else `422`). Update `db_test.go` if it checks columns.

#### 2. `backend/internal/models/models.go`

- Add `Job` with JSON tags matching api.md, including `board` and `company_url`. Skill lists and `requirements` are `[]string`, never `nil`.

#### 3. `backend/internal/parser/skills.go`

**Goal:** the skill dictionary, the one list that parsing, analysis, search ranking and phase 8's guard all share.

- Two maps, tech and soft, from a canonical name to its aliases: `"javascript": {"js", "ecmascript"}`, `"postgresql": {"postgres"}`, `"communication": {"communication skills", "written and verbal communication"}`.
- `Canon(s string) string` returns the canonical name for a skill or alias, or the trimmed lower-case input if unknown.
- Matching is by whole word (`go` must not match "good"; `c` and `r` match only as a standalone token, for example "C, C++" or "R and Python"). Build one regular expression per skill at `init`, or tokenize once and look up n-grams of 1 to 3 words; the second is faster and meets NFR-3 easily.
- Start from the full build's list (105 tech, 16 soft) and add common analyst and office skills (Excel, Tableau, Power BI, SQL Server, Salesforce, project management). The dictionary is tech-weighted; widening it to other fields is after the MVP (build-findings 8.2).

References: [regexp](https://pkg.go.dev/regexp), [strings.Fields](https://pkg.go.dev/strings#Fields).

#### 4. `backend/internal/parser/parser.go`

**Goal:** turn pasted or fetched posting text into the fields api.md's parse route returns.

- `Clean(raw string) (string, int)`: normalize line endings and whitespace, remove invisible characters with phase 1's `textclean.StripInvisible`, collapse runs of blank lines, strip bullets to a consistent `- `. Returns how many characters were hidden.
- **Hidden text in postings.** Pasted text has no styling, so invisible characters are what can hide there. Greenhouse postings arrive already cleaned by phase 1's HTML rules. `POST /v1/jobs/parse` and `POST /v1/jobs` report `hidden_text: { "characters": n }` (or `null`) so the UI can say "Hidden text was removed from this posting". Add the field to api.md.
- **Sections.** Recognize headings by line: required (`requirements`, `qualifications`, `what you bring`, `must have`, `minimum qualifications`) and preferred (`preferred`, `nice to have`, `bonus`, `plus`). Skills found under a required heading go to `req_*`; under a preferred heading to `pref_*`; elsewhere to `req_*` only when no required section exists, so a posting with no headings still yields skills.
- **Requirements.** Bullet lines under required headings, as written, in order, at most 30.
- **Description.** The text before the first required or preferred heading, trimmed to 2,000 characters.
- **Inferred fields.** Title: the first short line (under 80 characters) that is not a heading. Company: a line or phrase after "at", "About", or ending in "Inc.", "LLC", "Corp"; else empty. Location: a line with a city and state or country pattern, or "Remote". Work mode: Remote, Hybrid or On-site from the text, as phase 2's `match.go` does; share the function.
- `Parse(raw) Result` runs all of the above. Pure function, no I/O.

#### 5. `backend/internal/analyze/analyze.go`

**Goal:** compare a job's skills with the master resume, deterministically and fast.

- `Run(job models.Job, m models.Master) Result`.
- The resume's skills: `m.Skills()` plus `profile.clearance_certs`, canonicalized with `parser.Canon`.
- For each job skill, matched if it is in the resume's skills, or if the parser finds it in any experience's `description` or `impact`. Evidence: the ids of experiences whose skills or text contain it.
- `score` is matched required (tech and soft) over total required; `1` when the job lists no required skills, with the UI saying so.
- Record `duration_ms`.

#### 6. `backend/internal/store/job.go`

**Goal:** the jobs table.

- `CreateJob`, `GetJob`, `ListJobs` (filters: `q` on company or title with `LIKE`, `source`, `archived`, `trashed`, `limit` default 50 and at most 200, `offset`; without `raw_text` and `description`), `UpdateJob`, `DeleteJob`, `UpsertGreenhouseJob` (insert or update on `(source, external_id)`; returns whether it was new).

#### 7. `backend/internal/api/jobs.go`

**Goal:** the seven Jobs routes.

- `parse` calls `parser.Parse` and returns the result. No writes.
- Create: for `pasted`, parse `raw_text`, fill skills and requirements, then fill empty company and title from `inferred`. For `manual`, store what is given.
- `analyze` loads the job and `store.GetMaster`, calls `analyze.Run`. `409` "create a profile first" when no profile exists.

#### 8. `backend/internal/api/connectors.go` (import)

**Goal:** `POST /v1/connectors/greenhouse/import` as described above.

- Fetch all postings first (at most 8 at once), then write in one transaction, so a failure writes nothing.

#### 9. `backend/internal/connectors/greenhouse/search.go` (`skill_score`)

**Goal:** rank search results by fit with the resume.

- `Search` takes an optional `func(text string) float64` scorer, so the `greenhouse` package does not import `parser` or `store`. The API layer builds it: parse the posting's `raw_text`, run `analyze.Run` against the master, return `score`.
- Score the top 40 title matches (fetch their details for this; return the top N after re-ranking). `score = 0.5 * title + 0.35 * skill + 0.15 * location`. Without a profile, keep phase 2's formula.

### Electron

#### 10. `app/main/ipc-handlers.js` and `app/preload/preload.js`

- Channels: `jobs:parse`, `jobs:create`, `jobs:list`, `jobs:get`, `jobs:update`, `jobs:delete`, `jobs:analyze`, `connectors:import`.
- Preload: `api.jobs.*` and `api.connectors.import(postings)`.

### Renderer

#### 11. `app/renderer/views/job-parser/job-parser.js`

**Goal:** save jobs and see the gap.

- Tabs: Find (phase 2), Open link (phase 1), Paste, Saved.
- **Paste:** a text area; Preview calls `parse` and shows the inferred fields (editable), optional fields for the listing link and the company's site, and the skill lists; Save calls create.
- **Save from Greenhouse:** a Save button on the listing display (link view and search results) calls `connectors:import` with the posting's board and id.
- **Saved:** a list with search by company or title. Selecting a job shows its details, links to the company's site (when `company_url` is set) and to the listing, and the analysis panel. Delete asks first and shows the `409` message when an application exists.
- **Analysis panel:** score as a percentage of required skills; required and preferred groups, each split into matched (with the experience titles that show it) and missing. Run analysis when a saved job opens; it is fast enough to need no button.

### Tests and tooling

- `backend/testdata/postings/`: 6 to 10 real postings as `.txt` (tech, analyst, one with no headings, one with only a preferred section), each with a `.json` of expected skills and inferred fields.
- `parser_test.go`: table test over `testdata/postings` (include one posting with zero-width and tag characters hiding a skill list, which must not be detected as skills); whole-word matching (`go`, `c`, `r`); aliases; section assignment; posting with no headings.
- `analyze_test.go`: matched by listed skill, matched by experience text only, missing, certifications count, score with no required skills.
- `analyze_test.go` latency: a master with 30 experiences of 300 words each and a 10,000 word posting; `parser.Parse` plus `analyze.Run` must finish under 1.5 s (NFR-3). Log the duration so CI shows the margin.
- `job_test.go` (store): CRUD, filters, upsert new and existing, `ErrConflict` on delete with an application.
- `api_test.go`: create pasted with inference, `company_url` validation, `422` when inference fails, analyze without a profile (`409`), import new and refresh, import with a closed posting writes nothing.
- `search_test.go`: with a scorer, a better skill fit moves up.

## Done when

- Pasting a posting or saving a Greenhouse listing creates a saved job, and saving the same listing again updates it.
- A saved Greenhouse job keeps its direct company link when the posting had one.
- Opening a saved job shows matched and missing skills, with the experiences that show each match.
- The latency test passes.
- Search results with a profile rank by skill fit.
- `go test ./...` and `npm test` pass.

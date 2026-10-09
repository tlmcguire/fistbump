# Full-build findings

Findings from building a complete version of FistBump on branch `experiment/full-build`, compared against [architecture.md](architecture.md), [api.md](api.md), [schema.md](schema.md) and [CONTRIBUTING.md](../CONTRIBUTING.md). It lists what changed from the plans, what was not built, and decisions the team should make before any of this merges.

Nothing on the branch is committed. The docs listed above were updated as the build went, so they describe the branch. This file explains why they differ from `main`.

## Summary

| Area | Result |
|------|--------|
| Planned features | Built: master resume, paste and Greenhouse import, skill-gap analysis, suggestions with accept/reject/edit, tailored resume, PDF export, tracker, local/remote/rules AI, model downloads, storage cleanup, secrets handling |
| Added beyond the plan | Resume import from PDF or text, resume generation and export, home view, in-app posting viewer, archiving, theme switch, background notifications for suggestion rounds |
| Schema | Four migrations instead of one: `002` to `004` add `profile.summary`, `profile.skills`, `jobs.archived_at`, and turn Greenhouse on by default |
| API | 76 routes registered (plan: 67). 9 new routes and several changed behaviors, listed below |
| Not built | GitHub Actions workflow, renderer tests, `testdata/` fixtures, adjacent `_test.go` for many files |
| Plan conflicts to decide | Greenhouse on by default and auto-search (network policy), install size (NFR-5), store and api file layout |
| Plan-level risks (section 8, post-MVP) | Skill matching finds nothing in healthcare, teaching or trades postings; the 3B local model is licensed for non-commercial use only; promo claims (pay parsing, hosted AI, "every job board") are not built; no licensing, signing or update plan for a paid product |
| Tests | Go: 11 packages pass, `go vet` clean, race detector clean on all packages. JS: 6 tests on the IPC layer |

## 1. Schema changes

The plan names `001_init.sql` as the whole schema. The branch adds five migrations. They are additive and run in order through `PRAGMA user_version`, so existing databases upgrade in place.

| Migration | Change | Why |
|-----------|--------|-----|
| `002_summary_and_greenhouse.sql` | `ALTER TABLE profile ADD COLUMN summary TEXT`; sets `connectors.greenhouse.enabled` to `true` | Resumes have a summary section, imports carry one, and tailoring rewrites it. Greenhouse on by default was requested (see section 3.4) |
| `003_profile_skills.sql` | `ALTER TABLE profile ADD COLUMN skills TEXT NOT NULL DEFAULT '[]'` | Imported resumes list skills that no role mentions. Without a home for them, the import credited them to the latest job, which is false. `models.Master.Skills()` now unions profile and experience skills |
| `004_job_archive.sql` | `ALTER TABLE jobs ADD COLUMN archived_at TEXT` plus an index | Archive saved jobs without deleting their revisions, drafts or applications |
| `005_profile_links.sql` | `ALTER TABLE profile ADD COLUMN links TEXT NOT NULL DEFAULT '[]'` | Keep the LinkedIn, GitHub and site links found on an imported resume's contact line. They are kept as written and never generated |
| `006_rejected_status.sql` | Rebuilds `applications` so the status `CHECK` allows `Rejected`; rows and indexes are copied | Track companies that turned the user down, separately from Archived. SQLite cannot alter a `CHECK`, so the table is rebuilt. Verified on an existing database: rows kept, integrity and foreign key checks pass |

### Schema decisions to make

1. **One application per job is enforced in the API, not the database.** `POST /v1/applications` returns `409` when the job is tracked. A `UNIQUE` index on `applications(job_id)` would make it a real constraint, but existing duplicate rows must be merged first. Test databases created before the check already contain duplicates.
2. **Two archive mechanisms.** Applications archive with `status = 'Archived'`; jobs archive with `jobs.archived_at`. They are linked only by an option on the archive route. Options: keep both (they mean different things: "no longer pursuing" vs "hide this listing"), or add `applications.archived_at` and keep `status` for pipeline stage only.
3. **Archived jobs and GC.** `jobs.retention_days` cleanup deletes jobs by `searched_at` and does not look at `archived_at`. Decide whether archive should protect a job from cleanup, or speed its removal.
4. **Tailored resumes do not store their base.** The diff view rebuilds the base by undoing the applied suggestions (`render.Revert`). This avoids a schema change but fails if the user later edits text in a way that collides with a suggestion. A `tailored_resumes.base_content` column would make the diff exact.
5. **Suggestions do not record their source.** A round can mix model suggestions with rule-based ones that fill items the model skipped. Only `revisions.engine` is stored. A `suggestions.engine` column would let the UI label each one.
6. **`connectors.greenhouse.categories` is now unused by default.** Search covers every curated board; categories are only an optional filter in the request. The setting can be retired or turned into a saved default filter.
7. **Theme is not in `settings`.** It lives in `userData/prefs.json` in the Electron main process, because it must apply before the window opens and before the backend is up. Documented in `app/main/prefs.js`.
8. **File tree in architecture.md** still lists only `migrations/001_init.sql` under `db/`.

## 2. API changes

All are reflected in [api.md](api.md).

### New routes

| Route | Purpose |
|-------|---------|
| `POST /v1/resume/import` | Extract text from a PDF, `.txt` or `.md` (base64) or pasted text and return a draft. No writes |
| `POST /v1/resume/import/apply` | Write a reviewed draft to the master resume, optionally replacing entries and saving the original text |
| `POST /v1/resume/summary` | Draft a summary (model when available, rules otherwise). Not saved |
| `GET /v1/resume/preview` | Rendered master resume as Markdown |
| `POST /v1/resume/export` | Master resume as PDF or Markdown bytes |
| `POST /v1/jobs/{id}/archive`, `/unarchive` | Archive or restore a saved job, optionally archiving its application |
| `POST /v1/applications/bulk-status` | Set one status on many applications |
| `GET /v1/connectors/greenhouse/postings/{external_id}` | Full text of a search result, for the in-app viewer |

### Changed behavior

| Route | Plan | Branch |
|-------|------|--------|
| `POST /v1/connectors/{id}/fetch` | Boards from settings (custom plus enabled categories) | Every curated board plus custom boards; categories are an optional filter. Keywords default to target positions, then the latest job title. Response adds `matched`, `boards_total`, `postings_scanned`, `keywords_used`, `work_mode_used`, and per posting `title_score`, `skill_score`, `work_mode`, `saved_job_id`, `archived` |
| `POST /v1/connectors/{id}/import` | Re-runs the query; omitting ids imports all results | Imports by `external_ids` from recent results; `query` is accepted and ignored; ids are required |
| `GET /v1/connectors/greenhouse/categories` | Array plus a notice | Object: `categories`, `custom_boards`, `verified_at`, `notice` |
| `POST /v1/applications` | Any job | `409` with `details.application_id` if the job is already tracked |
| `GET /v1/jobs` | All jobs | Excludes archived unless `archived=include` or `only` |
| `GET /v1/revisions/{id}` | `suggestions` present | Always present, even when empty (it was omitted when empty, which broke the UI) |
| `GET /v1/health` | `schema_version: 1` | `4` |

## 3. Comparison with the architecture plan

### 3.1 Matches the plan

- Electron shell with a sandboxed renderer, `contextBridge` preload, and IPC channels mapped to HTTP in `ipc-handlers.js`. A test checks that preload and main list the same channels.
- `backend.js`: spawn with `--data-dir`, token in `FISTBUMP_TOKEN`, single `PORT=` line on stdout, health check, restart with a new port and token (3 restarts per 60 s), stdin close as the quit signal, 10 s startup and 5 s shutdown timeouts.
- Single-instance lock; macOS keeps running with no windows.
- `secrets.js` encrypts the remote key and Hugging Face token with `safeStorage`, refuses to persist on Linux `basic_text`, and re-sends them after every backend restart. Go holds them in memory only.
- Bearer token compared in constant time; loopback only; logs never include headers or bodies.
- AI layer: `local` (llama-server child, health check, idle stop), `remote` (OpenAI-compatible, HTTPS unless loopback, no cross-host redirects, key redacted from errors), `rules` fallback. Auto mode falls through on failure.
- Model downloads: HTTP range resume, SHA-256 check, atomic rename, `.part` kept on cancel.
- Deterministic skill-gap analysis with a latency test (NFR-3).
- Export returns bytes; Electron owns the save dialog, so Go never receives a path.
- Storage report and cleanup with dry run.
- Open item "Go test for `boards.json`" is done (`greenhouse_test.go` checks 32 categories, parents, tokens, and 517 unique boards).

### 3.2 Changed from the plan

| Plan | Branch | Note |
|------|--------|------|
| `pdf` uses gofpdf or maroto | `github.com/go-pdf/fpdf` (maintained gofpdf fork) | Core fonts are Latin-1 only. Names or text outside Latin-1 render as `?`. Embedding a UTF-8 TTF would fix it |
| `store/` one file per table (`experience.go`, `education.go`, `job_listing.go`, `suggestion.go`, `tailored_resume.go`) | `profile.go` holds profile, experiences and education; `revision.go` holds suggestions; `job.go`, `tailored.go` | Split to match the plan, or update the plan |
| `api/` has `middleware.go` | Middleware lives in `server.go`; `resume_import.go` added | Same choice |
| Packages listed in architecture.md | Added `render` (Markdown rendering, apply and revert of suggestions) and `resumeparse` (PDF text extraction, resume parsing) | Add both to the package table |
| `ai` files: provider, local, remote, fallback, models, download, catalog.json | Also `engine.go` (engine order and fallthrough), `prompt.go`, `guard.go` (anti-hallucination), `resume.go` (import structuring, summaries) | |
| Searching Greenhouse: fetch selected boards with content, cap per request, memory cache | Two stages: list every board without content (about 3 to 4 s cold for 517 boards and 39,000 postings, about 0.1 to 0.5 s cached), then fetch descriptions for the 40 best title matches. Listings cached on disk for 6 h | Ranking adds title synonyms, location and country awareness, and at most 3 results per company at the top |
| Connectors are opt-in and off by default | On by default (migration `002`) | See 3.4 |
| Renderer views: resume-builder, job-parser, diff-view, tracker, settings | Also `home`. The UI calls diff-view "Tailor". Settings is a sheet, not a view. Kept-alive views (Resume, Jobs, Tailor) stay mounted when you switch tabs. Shared modules `js/icons.js` and `js/bus.js` were added | |
| Tracker view (unspecified) | Spreadsheet: inline edits, sorting, filters, bulk status, CSV export | |
| Main process files: main, backend, secrets, ipc-handlers | Also `prefs.js` (theme, window state), `menu.js` (menu bar, context menu), `custom-css.js` | |
| `scripts/fetch-llama-server.js` | Picks the newest `bNNNN` build release (GitHub's "latest" release is not a build) and checks GitHub's published SHA-256 | Plus `scripts/host-arch.js` for Apple Silicon running x64 Node |
| `electron-builder.yml` uses `bin/${os}-${arch}` | Per-platform `extraResources` | `${os}` expands to `mac`, `win`, `linux`, which did not match `bin/darwin-*`. Packaging (`npm run dist`) is still untested |

### 3.3 Not built

| Plan item | State |
|-----------|-------|
| `.github/workflows/build.yml` (Go tests, JS tests, build matrix) | Not created |
| JS renderer and IPC handler tests in `app/tests` | Only IPC routing tests (6). No renderer tests |
| `backend/testdata/` (`seed.sql`, `postings/`, `api/`) | Not created. Tests use inline fixtures and `httptest` servers |
| "Every `foo.go` has an adjacent `foo_test.go`" | Not met. 37 files lack one, including most of `store/` (only `store_test.go` exists, 15% direct coverage), most of `api/` (covered by `api_test.go`), and most of `ai/` (covered by `ai_test.go`). Coverage: ai 56%, analyze 79%, api 53%, greenhouse 89%, db 72%, diff 96%, parser 85%, pdf 97%, render 61%, resumeparse 86%, models 0% direct |
| `golangci-lint` | Not installed or run |
| Model download through the UI | Code and unit tests exist. The installed model was fetched with `curl` using the same URL and checksum, so the in-app download path is not tested end to end |
| `docs/proposal.pdf` | Not touched |

### 3.4 Conflicts with stated constraints

1. **Network policy (NFR-1 and "Security model").** The plan says no network calls happen unless the user opts in, and connectors are off by default. On this branch Greenhouse is on by default, and opening Jobs > Find runs a search automatically when keywords are known. Requests contain only public board tokens; keywords, resume and filters never leave the machine because filtering and ranking are local. Still, this is a policy change that the docs and any privacy statement must reflect. Options: keep on by default but require a click to search; or ask on first run.
2. **Install size (NFR-5, under 250 MB excluding models).** The unpacked Electron runtime is 307 MB, and the bundled `bin/darwin-arm64` (backend 12 MB plus llama.cpp 67 MB) adds 79 MB. A packaged app will likely exceed 250 MB on disk before any model. Measure a real `npm run dist` build; consider not bundling llama-server and downloading it on demand like models.
3. **"User controls final text" (NFR-4).** Met: every change is a suggestion, and tailored resumes are built only after each suggestion is accepted, edited or rejected. "Accept all remaining" exists as a shortcut.

## 4. AI accuracy and hallucination controls

Model output passes these checks before it is stored. A failing proposal is dropped, never repaired, and logged to stderr.

| Check | Where | Effect |
|-------|-------|--------|
| Skill guard | `ai/guard.go` | A proposal may name only skills the resume already shows (resume text, listed skills, matched skills, certifications). Skills the job wants but the resume lacks are rejected |
| Skills line | `ai/guard.go` | The skills line may only be reordered: same items, none added or removed |
| Grounding | `ai/prompt.go` | At least 75% of a proposal's content words must appear in the resume or matched skills. Missing job skills are excluded from the allowed vocabulary |
| Summary | `ai/resume.go` | AI summaries pass the same skill and grounding checks, else the rules summary is used |
| Resume import | `ai/resume.go` | Every field a model extracts must appear in the source text; bullets must be at least 80% grounded |
| Gap fill and labels | `ai/engine.go` | Items the model skipped are filled by rules. A round where the model contributed nothing is labeled `rules`, not as AI |
| JSON mode | `ai/remote.go` | `response_format: json_object` for structured calls, with a retry without it for servers that reject it |

Known limits:

- The skill guard depends on the parser's skill dictionary (`parser/skills.go`). A tool not in the dictionary is not recognized as a skill, so the guard cannot block it; the 75% grounding check is the only backstop.
- Grounding is word overlap, not meaning. In testing, the 3B local model turned "Built" into "Led the development", which passes. Options: raise the threshold, flag leadership verbs, or highlight new words in the review UI.
- The local model is Qwen2.5 3B (Q4_K_M, about 2.1 GB, chosen for an 8 GB machine). It skips many items, which is why rules fill gaps. Larger models would rewrite more but need more RAM.

## 5. Resume import

Not in the plan. Added because users start from an existing resume.

- Text extraction is pure Go (`github.com/ledongthuc/pdf`). It removes common word-processor artifacts: overprinted invisible glyphs, a decorative letter some heading fonts append to every run, private-use icon glyphs, and superscripts split onto their own line.
- Parsing is heuristic: fuzzy section headings, title/organization/date header blocks, paragraph or bullet descriptions, season and single dates, several degrees under one school, research and activity sections parsed as entries.
- Scanned PDFs (no text layer) are rejected with a message; there is no OCR.
- Multi-column layouts and tables are not handled specially and may interleave text.
- The user reviews every field before anything is saved. "Read with AI" can redo the parse with a model, under the grounding rules above.

## 6. Other considerations

| Topic | Note |
|-------|------|
| Greenhouse load | A cold search makes 517 listing requests (24 concurrent) plus up to 40 detail requests, then reuses listings for 6 h. Worth a check of Greenhouse terms and a descriptive User-Agent before release |
| Interrupted rounds | Suggestion generation runs in memory. On startup, rounds left `queued` or `running` are marked `failed` |
| Dependency pinning | `package.json` uses `"*"` for `electron` and `electron-builder`. Pin versions before release. npm 11 needed an `allowScripts` entry for Electron's installer; `electron-winstaller` is still unapproved |
| Logo | `app/renderer/img/logo.png` is cropped from the promo video (132 by 98 px). Replace it with the source artwork |
| Font | Inter is bundled under the SIL Open Font License (`app/renderer/fonts/OFL.txt`) because the content security policy blocks remote fonts |
| Branch name | `experiment/full-build` does not use a prefix from CONTRIBUTING.md (`feature/`, `fix/`, and others). Rename before opening a pull request |
| Commit plan | The branch is one large change. Splitting it into reviewable commits by area (schema, store, api, ai, connectors, resume import, electron main, renderer, docs) would follow CONTRIBUTING.md |

## 7. Further considerations

Found in a review pass after the build. None of these is a new feature. Each was checked against the code. Numbers are kept stable so items can be referenced; "Effort" is a rough size (small: a line or a few, or a copy change; medium: a function or a test).

### 7.1 Fixed in review

| # | Finding | Fix | Covered by |
|---|---------|-----|------------|
| 1 | Home marked a next step "Overdue" by comparing it with the UTC date, so steps due today showed as overdue in the US evening | Compares with the local date (`views/home/home.js`) | Checked in the app |
| 2 | Opening an archived job from the tracker selected a different job, because Jobs > Saved lists active jobs only | Jobs opens the Archived list when the requested job is archived (`job-parser.js`) | Checked in the app |
| 3 | `PUT /v1/settings` validated each value while writing it, so one bad value could leave earlier keys saved | Validates every value first (`store.ValidateSetting`), then writes all in one transaction | `TestSettingsAreAllOrNothing` |
| 4 | Resume import with `replace` deleted entries, then inserted new ones, without a transaction | The store gained `WithTx` (methods now run on a connection or a transaction); apply runs in one transaction | `TestWithTxRollsBackEveryWrite`, `TestImportApplyValidatesBeforeWriting` |
| 5 | The rules summary counted years from any entry, so imported clubs and projects inflated "N+ years" | Years count only entries whose title reads as a job (`resumeparse.IsJobTitle`) | `TestRulesSummaryIgnoresActivitiesForYears` |
| 6 | Home's "tailored drafts" counted finished suggestion rounds | Counts tailored resumes built | Checked in the app |
| 7 | Jobs added a window listener on every mount | Removed in the view's cleanup | Code review |
| 8 | README said Node 20+, but `npm test` needs Node 21 or later | README says Node 22+ and why | n/a |
| 10 | A hosted remote provider with no key in memory (after a restart where the key could not be stored) was tried first on every round | `Remote.Ready()` requires a key unless the server is loopback; auto mode skips it, a forced remote returns `503`, `GET /v1/ai/status` reports `needs_key`, and Settings explains it | `TestRemoteWithoutKeyIsSkipped` |
| 12 | The gold focus ring measured 2.1:1 on the light background (WCAG 2.2 asks 3:1) | Light theme uses navy (13.7:1); the navy top bar keeps gold (6.5:1) | Contrast measured |
| 13 | Warning text measured 4.44:1 | Darkened to 5.9:1 | Contrast measured |
| 18 | `jobs.retention_days` accepted 0, and cleanup with 0 removed every untracked job | `jobs.retention_days` and `ai.idle_minutes` have a minimum of 1; cleanup's `older_than_days` must be at least 1 | `TestSettingMinimums`, `TestSettingsAreAllOrNothing` |
| 9 | "Read with AI" and "Write it for me" did not say a remote provider would receive the resume | Both name the provider's host when the active engine is remote; "Write it for me" asks first | Code review |
| 11 | No in-app statement of where data lives | Settings > Privacy and data shows the folder, an Open Data Folder button (also in the File and Help menus), and what leaves the computer | Checked in the app |
| 14 | Tabs had no arrow-key navigation | Arrow keys, Home and End move between tabs, with roving focus | Code review |
| 19 | CSV export did not say it exports filtered rows | The button reads "Export N rows to CSV" and explains the filter in its tooltip | Checked in the app |
| 20 | `FISTBUMP_DATA_DIR` did not isolate preferences and stored keys | When set, Electron's `userData` (preferences, keys, custom CSS, single-instance lock) moves under it too | Checked: a test run no longer touches the real app's settings |

### 7.2 Open considerations

| # | Area | Finding | Why it matters | Effort |
|---|------|---------|----------------|--------|
| 15 | Reproducibility | `fetch-llama-server.js` downloads the newest llama.cpp build on every run | Two builds a day apart can ship different binaries | Small: pin a build tag and its SHA-256, as the model catalog does |
| 16 | Matching quality | Skill matching, the anti-hallucination guard and search ranking all rely on the dictionary in `parser/skills.go` (105 tech and 16 soft skills). Common resume items such as LaTeX, MIPS and Dash are not in it | Unknown skills are not matched and not guarded | Small per entry; worth an owner and a review cadence, like `boards.json` |
| 17 | Testing | The UI was verified by driving Electron through the DevTools protocol from scripts outside the repo | The checks cannot be rerun by others | Small: keep one smoke script in `scripts/`, or decide on a test tool later |

### 7.3 Desktop polish round

Changes made to make the app feel native rather than like a web page, and their open questions.

| Change | Where | Note |
|--------|-------|------|
| Settings is a sheet over the current view (gear button, status pill, menu, ⌘,) instead of a tab | `views/settings/settings.js` `openSettings`; old `#/settings` links open the sheet | Departs from the plan's `settings` view. Sections: Appearance, AI engine, Local models, Job search, Privacy and data, About |
| Window chrome: controls inside the navy bar (`hiddenInset` on macOS, `titleBarOverlay` on Windows and Linux), themed background, show when ready, saved size and position | `app/main/main.js` | Only macOS was run. Windows and Linux overlay buttons are untested |
| Native menu bar with shortcuts, a Theme submenu and Open Data Folder; right-click menu with spelling suggestions; native About panel | `app/main/menu.js` | The About panel credits names taken from the promo video |
| Navigation locked to the app page; dropping a file can no longer replace the page | `app/main/main.js` `will-navigate` | |
| Drop a resume anywhere to import it. The renderer sends the bytes, never a path | `app.js`, `resume:importData` | Main and Go still only read files the user picked or dropped |
| Suggestion rounds post a system notification when the app is not in front | `app.js` (HTML Notification API) | Unsigned development builds may not show macOS notifications |
| Custom CSS: Settings > Appearance editor, or the user's own editor with live reload | `app/main/custom-css.js`, `app.js` (constructable stylesheet) | Applied after the app's styles so plain token overrides win. The content security policy still blocks remote fonts and images (verified: requests are blocked with reason `csp`). Only the design tokens in `styles/app.css` are a stable surface; class names may change between versions |
| Design system: color roles (`--accent` interactive, `--highlight` attention, status colors for meaning only), flat hairline surfaces, drawn line icons instead of text glyphs, terse copy | `styles/app.css`, `js/icons.js` | Superseded in 7.4: all styles now live in `app.css` |
| Features the plan supported but the UI did not expose: tailoring from a saved resume version, editing a job's pay, type, closing date and link, reordering experience and education | `diff-view.js`, `job-parser.js`, `resume-builder.js` | No API changes were needed |

### 7.4 Parsing, PDF and visual round

| Change | Where | Note |
|--------|-------|------|
| Exported PDFs embed Inter (TTF, UTF-8) instead of the built-in Latin-1 fonts. Names like Łukasiewicz and symbols no longer print as dots. Contact links are clickable | `internal/pdf/pdf.go`, `internal/pdf/fonts/` (OFL) | Adds about 1 MB to the binary |
| `ledongthuc/pdf` is vendored with a one-line fix: `bfrange` lookups dropped the carry, so text from UTF-8 PDFs (including our own exports) came back with wrong characters | `backend/third_party/ledongthuc-pdf/`, `FISTBUMP-PATCHES.md`, `replace` in `go.mod` | Should be sent upstream; the vendored copy can go once it is merged |
| Import fixes: first role lost after a blank line, titles ending in "Inc." read as sentences, ", Inc." split from the company, honors line taken whole, wrapped URLs, URLs counted as skills | `internal/resumeparse/parse.go` | Covered by an export-then-import round trip test |
| Contact links kept from the original resume | migration `005`, `render.go`, `resume_import.go` | |
| `Rejected` application status | migration `006`, `applications.go`, tracker and home views | |
| New shell: sidebar navigation that collapses to an icon rail under 1080 px, sticky toolbars, grouped settings-style forms, container queries for pane width | `index.html`, `styles/app.css` | The four old stylesheets were folded into one `app.css` |
| macOS finish: system font, half-pixel hairlines, one shadow level (nested surfaces are flat), push-button and segmented controls, a translucent sidebar (`vibrancy: 'sidebar'`) on macOS, a brighter brand blue as the dark-mode accent | `styles/app.css` (Native finish section), `app/main/main.js` | Replaces an earlier layered-shadow pass. Windows and Linux get a solid sidebar. Instrument Sans is now only the fallback font off macOS |
| UI fonts: Instrument Sans (text) and Bricolage Grotesque (titles), bundled under OFL | `app/renderer/fonts/` | Replaces Inter in the UI so the app has its own voice; Inter stays in exported PDFs |

## 8. Plan-level findings (post-MVP)

**MVP status: not required for the MVP.** The team marked every item in this section as post-MVP. They are kept here so they are not lost, and should be revisited before a paid or campus release.

A third review pass, aimed at findings that should change plans rather than code. Each was checked against the build, the promo video (`Fistbump 60s Video`), or a primary source.

### 8.1 What the product promises vs. what is built (post-MVP)

From the promo video. Each row needs either a plan change or a wording change.

| Claim in the video | Build | Decision |
|--------------------|-------|----------|
| "Every job board. One private app." | Search covers about 517 companies on Greenhouse. Everything else (LinkedIn, Indeed, Handshake, most large employers) is paste only | Narrow the claim, or plan more connectors (Lever, Ashby, USAJOBS are listed as later candidates) |
| "Parse: fixed rules pull pay, location and requirements" | Location and requirements are parsed. Pay is not: probes with "$18.50 - $22.00 per hour", "$48,000 to $61,000", "$28/hr" and "$85,000-$100,000" all left `pay_min` and `pay_max` empty | Add pay extraction to the parser (the schema already has the columns), or drop "pay" from the claim |
| "Compare: AI matches your pick to your master resume" | Matching is deterministic and never uses AI, by design (NFR-3) | Keep the design and change the wording ("matches"), or decide AI should take part |
| "Suggest: tailored edits you approve line by line" | Suggestions are per section or entry (a whole description, the summary, the skills line). The review shows word-level changes inside each | Accept the granularity, or split suggestions per bullet (affects `suggestions.target_id` and the rules for applying them) |
| "Cloud AI, yours or ours, only if you choose." | "Yours" exists (any OpenAI-compatible endpoint with your key). "Ours" does not | A hosted option implies accounts, billing, a data processing agreement and a privacy policy, which the plan's "no accounts, local first" design does not have. Decide before promising it |
| Demo: "Patient Care Tech, Harbor Medical" | Matching finds no healthcare skills (see 8.2) | See 8.2 |
| "Career centers: one-time license per campus, free for students. Job seekers buy a one-time license." | No licensing, activation, campus deployment or student tier exists, and the plan does not mention them | See 8.4 |

### 8.2 Matching only works for tech roles (post-MVP)

This is the largest gap between the product's audience (career centers, every major) and the build.

Probes of the parser on four non-tech postings found:

| Posting | Skills detected |
|---------|-----------------|
| Patient Care Technician (BLS/CPR, vital signs, Epic EHR, phlebotomy, EKG) | none |
| Middle School Math Teacher (certification, classroom management, lesson planning) | none |
| HVAC Technician (EPA 608, refrigeration, heat pumps) | none |
| Financial Analyst (financial modeling, forecasting, variance analysis, CPA/CFA) | `excel` only |

Consequences: skill-gap analysis is empty, search ranking ignores the resume, and the guard against invented skills (section 4) has nothing to check, so the anti-hallucination protection is weakest exactly where the product claims to serve people.

The cause is the hand-written dictionary in `parser/skills.go` (105 tech and 16 soft skills). Options, which can combine:

1. **Use a public occupational taxonomy.** O*NET (US Department of Labor) publishes occupations with their skills, knowledge and technology tools under CC BY 4.0, with listed exceptions that need checking for the technology data. ESCO (European Commission) is a multilingual alternative under CC BY 4.0. Either needs attribution and an import script; the dictionary would grow from about 120 entries to thousands, so matching speed (NFR-3) must be re-measured.
2. **Let the user's own words count.** Treat skills listed on the resume and phrases repeated in the posting's requirements as candidates, even when not in a dictionary.
3. **Optional model-assisted extraction,** under the same grounding rules as resume import (every extracted skill must appear in the posting text).

Whatever is chosen, build a fixture set of real postings and resumes across majors with expected results, and measure against it in CI (8.6).

### 8.3 Licensing and legal (post-MVP)

| Item | Finding | Action |
|------|---------|--------|
| Local model default | Qwen2.5 3B (the model selected on the development machine, chosen for 8 GB of RAM) is under the Qwen Research License: "FOR NON-COMMERCIAL PURPOSES ONLY". Qwen2.5 1.5B is Apache 2.0. Llama 3.2 3B is under the Llama 3.2 Community License (commercial use allowed, with attribution and an acceptable use policy) | Remove or flag non-commercial models before any paid release; add a `license` field to `ai/catalog.json` and show it in Settings. Evaluate other 3 to 4B models with permissive licenses |
| Greenhouse | Greenhouse documents the Job Board API for building a company's own career site. Bulk searching 517 companies' boards is a different use, and job descriptions belong to employers | Legal review of Greenhouse's terms before release, and consider asking Greenhouse for permission. Keep requests user-initiated |
| O*NET or ESCO (if adopted) | CC BY 4.0 requires attribution | Add an attributions screen (About) |
| Dependencies | Go: modernc.org/sqlite (BSD-3), go-pdf/fpdf (MIT), ledongthuc/pdf (BSD-3); Inter font (OFL); llama.cpp (MIT). Not yet inventoried formally | Generate a third-party notices file at build time and include it in the app |

### 8.4 Distribution, updates and the business model (post-MVP)

| Topic | Finding | Consideration |
|-------|---------|---------------|
| Code signing | `llama-server` from the llama.cpp release is ad-hoc signed (`Signature=adhoc`, no team). A notarized macOS app requires every bundled executable to be signed with the developer's ID and hardened runtime. Windows unsigned apps trigger SmartScreen warnings | Budget for an Apple Developer account and a Windows code-signing certificate; sign `fistbump-core` and `llama-server` in the build |
| Updates vs. one-time license | Electron ships security releases every few weeks, and the app embeds a browser engine that renders job text. A one-time license still needs a way to deliver updates | Plan auto-update (electron-updater needs signing and a release host) or a documented update channel for campus IT |
| Licensing | Nothing checks a license | Decide the model (honor system, offline license keys, campus site keys) before building; it affects the local-first, no-accounts design |
| Campus deployment | Lab machines are shared; installs are managed (MSI, pkg, MDM) | Per-user data is already separate. Plan silent installers and preconfigured settings (for example a campus-wide Greenhouse board list or a hosted model endpoint) |
| Device reach | Electron runs on Windows, macOS and Linux only. Many students use Chromebooks or tablets | Decide whether Chromebook users are in scope (the Linux container is the only route) |
| Hardware | Measured on the development Mac: llama-server with the 3B model uses about 2.3 GB of memory; the app (Electron and backend) about 0.65 GB. The model download is 2.1 GB (1.1 GB for 1.5B) | State 8 GB of RAM as the minimum for local AI; default smaller machines to the 1.5B model or rules; plan for slow campus Wi-Fi on first download |
| Install size | See 3.4 (NFR-5) | |

### 8.5 Data safety (post-MVP)

| Topic | Finding | Consideration |
|-------|---------|---------------|
| Backups | Local first means losing the computer loses everything. There is no export or restore | Plan an export and import of the whole data set (the database and settings), for backup and for moving to a new computer |
| Downgrades | `db.Open` applies migrations newer than `PRAGMA user_version` but does not reject a database newer than the app. An older app opening a newer database runs against columns it does not know | Refuse to open, with a clear message, when the database version is newer than the app's latest migration |
| File permissions | The backend creates folders `0755` and files `0644` (`cmd/fistbump-core/main.go`, `ai/download.go`, `greenhouse.go`). On macOS the Electron folder above is `0700`, so the data is private. Elsewhere (Linux, a custom `FISTBUMP_DATA_DIR`, shared lab machines) other users can read the resume | Create the data folder `0700` and files `0600` |
| Encryption at rest | The database is plain SQLite. The no-cgo constraint (NFR-2) rules out SQLCipher | Rely on OS disk encryption and say so in the privacy statement, or revisit the constraint |
| Stored keys and signing | `safeStorage` keys are tied to the app's identity. Changing the signing identity, or moving from a development build to a signed one, makes stored API keys unreadable | Expect users to re-enter keys once after the first signed release; say so in release notes |

### 8.6 Development process (post-MVP)

| Topic | Consideration |
|-------|---------------|
| Evaluation set | Matching, resume parsing and suggestion quality have no measured baseline. A versioned set of anonymized resumes and postings across majors, with expected skills, fields and "must not invent" checks, would turn each change into a number (parse accuracy, skills found, suggestions dropped by the guard) |
| End-to-end tests | UI checks in this build were run by hand through the DevTools protocol. Playwright supports driving Electron apps and could replace them in CI |
| CSS | Done in 7.4: one `app.css`. Publishing the token list for custom CSS is still open |
| Scope control | The build added features beyond the plan (import, home, archiving, overlays, custom CSS). Before merging, confirm with the team which of these are in scope for the course timeline |

## 9. Suggested next steps

### For the MVP

1. Decide the Greenhouse network policy (3.4).
2. Run `npm run dist` on each platform, measure install size (3.4, NFR-5), and decide whether to bundle llama-server.
3. Add `.github/workflows/build.yml` (3.3).
4. Decide the schema questions in section 1, especially one application per job and archive semantics.
5. Rename the branch and split it into reviewable commits.

### After the MVP (section 8)

1. Decide who the product serves first. If it is every major, plan the taxonomy change (8.2) before more matching or AI work.
2. Replace the non-commercial default model and add licenses to the model catalog (8.3).
3. Align the promo claims with the plan, or plan the missing pieces: pay extraction, hosted AI, more job sources (8.1).
4. Decide the licensing and update model, and budget for code signing (8.4).
5. Legal review of Greenhouse's terms (8.3).
6. Add data export and downgrade protection, and tighten file permissions (8.5).
7. Add an evaluation set and end-to-end tests (8.6).

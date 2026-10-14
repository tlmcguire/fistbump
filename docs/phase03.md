# Phase 3: Master resume and import

Build on `main` after [phase 1](phase01.md) merges, from [api.md](api.md) ("Resume", "Saved resumes", "Settings") and [schema.md](schema.md). Can run in parallel with [phase 2](phase02.md). Overview and schedule: [roadmap.md](roadmap.md).

## Goal

The user builds a master resume (contact details, summary, experience, education, skills) in the app, or imports an existing resume (PDF, `.txt` or `.md`), reviews the draft, and applies it. The master resume is saved in SQLite, survives a restart, and can be saved as a named version. This meets FR-1 and the resume-version part of FR-5.

Import never trusts the file. Formats other than PDF, `.txt` and `.md` are rejected with a message that says what to do instead, and text hidden in a PDF (invisible, same color as its background, microscopic, or off the page) is left out of the draft and reported to the user.

This phase also builds the parts every later phase uses: the `store` package, shared types in `models`, request validation, the settings store, and a view router in the renderer.

## Scope

In:

- The `store` package for `profile`, `experiences`, `education`, `resumes` and `settings`.
- The 10 Resume routes, the 5 Saved resume routes, the 2 Settings routes, and `GET /v1/resume/preview`.
- `render.Master`: the master resume as Markdown, used by `from_master`, the preview, tailoring (phase 5) and PDF export (phase 6).
- A hash router in the renderer and the `resume-builder` view.
- Resume import: `POST /v1/resume/import` and `POST /v1/resume/import/apply`, with format checks, PDF text extraction that drops hidden text, and a rules-based parser. Steps 13 to 19; they can merge as a second pull request, and phase 4 does not wait for them.

Out: AI reading of imports (`"engine": "auto"`) and summary generation (`POST /v1/resume/summary`) (after the MVP), OCR for scanned PDFs (not planned), Word documents (after the MVP), PDF export (phase 6), a settings view (phase 8).

## Routes

All as written in [api.md](api.md). The notes below fix details api.md leaves open.

| Group | Routes |
|-------|--------|
| Resume | `GET /v1/resume`, `PUT /v1/resume/profile`, `GET`/`POST /v1/resume/experiences`, `PUT`/`DELETE /v1/resume/experiences/{id}`, `GET`/`POST /v1/resume/education`, `PUT`/`DELETE /v1/resume/education/{id}` |
| Preview | `GET /v1/resume/preview` |
| Saved resumes | `GET`/`POST /v1/resumes`, `GET`/`PUT`/`DELETE /v1/resumes/{id}` |
| Settings | `GET`/`PUT /v1/settings` |

```jsonc
// PUT /v1/resume/profile
{ "full_name": "Ada Lovelace", "email": "ada@example.com", "phone": null, "home_location": "Charleston, SC",
  "target_positions": ["Data Analyst"], "preferred_mode": "Hybrid", "min_desired_pay": null,
  "clearance_certs": [], "summary": "Analyst with...", "skills": ["sql", "excel"], "links": ["github.com/ada"] }
// 200: Profile with id, created_at, updated_at

// POST /v1/resume/experiences   (no profile yet)
// 409
{ "error": { "code": "conflict", "message": "create a profile first" } }

// POST /v1/resume/education   { "institution": "College of Charleston", "gpa": 7 }
// 422
{ "error": { "code": "validation_failed", "message": "gpa must be between 0 and 5", "details": { "field": "gpa" } } }

// GET /v1/resume/preview
// 200
{ "markdown": "# Ada Lovelace\n\nCharleston, SC · ada@example.com\n\n## Summary\n\n..." }
```

- **Validation errors** carry `details.field` with the JSON field name, so the form can mark the field.
- **Dates** are `YYYY-MM-DD`. The form may send a month (`2024-06`); the API accepts `YYYY-MM` and stores `YYYY-MM-01`. Write this in api.md.
- **Reordering** uses `PUT` with a new `sort_order` on each moved row. Moving one entry up is two `PUT`s. api.md has no reorder route, and two writes are acceptable for a single user; if a write fails, the UI reloads the list.
- **One profile.** `PUT /v1/resume/profile` updates the single row or creates it. `POST` routes for experience and education attach to it.
- **Settings** known in this phase: every key in api.md's settings table. `PUT` validates every value before writing any, in one transaction.

| Status | `code` | When |
|--------|--------|------|
| 404 | `not_found` | Unknown experience, education or resume id |
| 409 | `conflict` | Experience or education created before a profile exists |
| 422 | `validation_failed` | Unknown field, missing required field, bad date, `end_date` before `start_date`, GPA outside 0 to 5, unknown or secret-looking setting key, wrong setting type, value below a minimum |

### Resume import

```jsonc
// POST /v1/resume/import
{ "filename": "ada-resume.pdf", "data_base64": "JVBERi0xLjcK..." }
// or
{ "text": "Ada Lovelace\nCharleston, SC ..." }

// 200
{ "text": "Ada Lovelace\nCharleston, SC ...",          // the visible text that was parsed
  "draft": { "profile": { "full_name": "Ada Lovelace", "email": "ada@example.com", "skills": ["sql"], ... },
             "experiences": [ { "company_name": "Acme", "job_title": "Data Analyst", "start_date": "2022-06-01", ... } ],
             "education": [ ... ],
             "skills": ["sql", "excel"],
             "warnings": ["Could not read a date for \"Research Assistant\""] },
  "hidden_text": { "words": 37, "reasons": ["same color as the background", "font smaller than 1 pt"],
                   "sample": "python kubernetes terraform aws machine learning ..." },
  "engine": "rules",
  "notice": "Review every field before applying. Nothing is saved yet." }

// POST /v1/resume/import   (a Word document renamed to .pdf)
// 422
{ "error": { "code": "validation_failed",
             "message": "This is a Word document. Save it as a PDF (File > Save As > PDF) and import that.",
             "details": { "reason": "unsupported_format", "detected": "docx" } } }
```

- **Nothing is saved by `import`.** The user reviews the draft; `import/apply` writes it, as api.md describes, in one transaction (`store.WithTx`).
- **`engine`** accepts only `rules` in the MVP (the default). `auto` returns `422` "Reading with AI is not available yet". Update api.md.
- **`hidden_text`** is `null` when nothing was hidden. Hidden text never goes into `text` or the draft. `sample` is the first 200 characters, so the user can see what was dropped.

**Accepted formats.** The bytes decide the type, not the filename. A file whose extension and content disagree is judged by its content, and the message names what it really is.

| Accepted | Check |
|----------|-------|
| PDF | Starts with `%PDF-` (allow up to 1 KB of leading bytes, which some generators write) |
| `.txt`, `.md` | Valid UTF-8, no NUL bytes; a UTF-8 byte order mark is removed; invisible characters are dropped (below) |
| Pasted `text` | 1 to 100,000 characters |

All files: 1 byte to 5 MB after base64 decoding.

**Rejected uploads.** Each is `422 validation_failed` with `details.reason`, and a message the UI shows as written.

| Upload | `details.reason` | Message (gist) |
|--------|------------------|----------------|
| Word, Pages, OpenDocument (ZIP `PK\x03\x04`, or OLE `D0 CF 11 E0`) | `unsupported_format` | Save it as a PDF and import that |
| Image (PNG, JPEG, HEIC, GIF, WebP magic bytes) | `unsupported_format` | Import a PDF with selectable text, or paste the text |
| Anything else that is not PDF or UTF-8 text | `unsupported_format` | Only PDF, `.txt` and `.md` files can be imported |
| Empty file, or `data_base64` that is not valid base64 | `empty` or `invalid_encoding` | The file is empty or could not be read |
| Over 5 MB | `too_large` | The file is larger than 5 MB |
| Password-protected PDF | `encrypted` | Remove the password (print to PDF) and import again |
| PDF with no text layer (scanned), or one where all text is hidden | `no_text` | No selectable text was found; paste the text instead |
| A PDF the library cannot parse (malformed, or it panics) | `unreadable` | The PDF could not be read; try exporting it again |

**Hidden text in every import.** Every import path (PDF, `.txt`, `.md`, pasted text) drops text the user would not see, and reports it in `hidden_text`:

- **Invisible characters**, in all paths: zero-width characters (U+200B to U+200D, U+2060, U+FEFF inside text), Unicode tag characters (U+E0000 to U+E007F), bidirectional controls (U+202A to U+202E, U+2066 to U+2069), and other format characters (Unicode category Cf) except a single leading byte order mark. A run of tag characters can spell out whole words that no viewer shows. Reason: "invisible characters".
- **Hidden PDF text**, in PDFs, by the rules below.

**Hidden text in PDFs.** A text piece is hidden, and dropped, when any of these hold:

| Rule | How it is detected |
|------|--------------------|
| Invisible render mode | Text render mode (`Tr`) 3 (invisible) or 7 (clip only) |
| Same color as its background | Fill color within 0.1 (on a 0 to 1 scale per channel) of the background under it. The background is the last filled rectangle drawn before the text that contains its position, else white. White text on a dark sidebar stays visible |
| Microscopic | Effective font size under 1 pt |
| Off the page | Position outside the page's crop box (media box when there is no crop box) |
| Overprinted | A second glyph drawn at the same position as another, which word processors leave under real text (the full build's `dropOverprints`) |

Not detected, and documented as limits: text covered by an image, text hidden by a clipping path, and text in a color close to a background image. These are rarer than white text on white.

## Files, in build order

Progress:

- [ ] 1. `models.go`
- [ ] 2. `store.go`
- [ ] 3. `profile.go`, `experience.go`, `education.go`
- [ ] 4. `resume.go` (store)
- [ ] 5. `settings.go` (store)
- [ ] 6. `render.go`
- [ ] 7. `decode.go`, `respond.go` additions
- [ ] 8. `resume.go` (api)
- [ ] 9. `settings.go` (api)
- [ ] 10. `ipc-handlers.js`, `preload.js`
- [ ] 11. `app.js` router
- [ ] 12. `resume-builder.js`
- [ ] 13. Vendored `ledongthuc/pdf` with patches
- [ ] 14. `resumeparse/detect.go`
- [ ] 15. `resumeparse/extract.go`
- [ ] 16. `resumeparse/hidden.go`
- [ ] 17. `resumeparse/parse.go`
- [ ] 18. `resume_import.go` (api)
- [ ] 19. Import in Electron and the Resume view
- [ ] Tests and tooling

### Backend

#### 1. `backend/internal/models/models.go`

**Goal:** the Go types shared by `store`, `api`, `render`, and later `analyze` and `ai`, so none of them imports another to get a type.

- `Profile`, `Experience`, `Education`, `ResumeVersion`, with JSON tags matching api.md. Nullable columns are pointers (`*string`, `*int64`, `*float64`); JSON arrays are `[]string` and never `nil` in responses (encode `[]`, not `null`).
- `Master { Profile *Profile; Experiences []Experience; Education []Education }` with `Skills() []string`: the de-duplicated union of `profile.skills` and every `experiences.skills`, in first-seen order.
- No database or HTTP code.

#### 2. `backend/internal/store/store.go`

**Goal:** the shared base for every table file: the connection, transactions, and error mapping.

- `type DBTX interface { ExecContext; QueryContext; QueryRowContext }`, satisfied by `*sql.DB` and `*sql.Tx`.
- `type Store struct { db DBTX; root *sql.DB }`, `New(*sql.DB)`, and `WithTx(ctx, func(*Store) error) error`, which runs the function on a transaction and commits or rolls back. Every method works the same inside or outside a transaction.
- Errors: `ErrNotFound` (from `sql.ErrNoRows`) and `ErrConflict` (from SQLite constraint errors: unique, foreign key, check). `mapErr` turns driver errors into these so `api` never sees a driver error.
- Helpers: `now()` in RFC 3339 UTC; JSON list encode and decode (`[]` for empty); nullable value helpers.

References: [database/sql](https://pkg.go.dev/database/sql), [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (error codes in `sqlite3` subpackage).

#### 3. `backend/internal/store/profile.go`, `experience.go`, `education.go`

**Goal:** CRUD for the master resume tables, one file per table as architecture.md plans.

- Profile: `GetProfile`, `UpsertProfile` (sets `updated_at`).
- Experience and education: `List` (by `sort_order`, then `id`), `Get`, `Create` (with `sort_order` defaulting to max plus one), `Update` (full replace), `Delete`.
- `GetMaster(ctx) (models.Master, error)` reads all three in one transaction, so the master is consistent.
- Skills are trimmed, de-duplicated case-insensitively, and stored as written; matching (phase 4) lower-cases them.

#### 4. `backend/internal/store/resume.go`

**Goal:** saved resume versions.

- `ListResumes` (without `content`, newest first), `GetResume`, `CreateResume`, `UpdateResume` (any subset; sets `updated_at`), `DeleteResume`. The schema's `ON DELETE SET NULL` handles dependents.

#### 5. `backend/internal/store/settings.go`

**Goal:** typed settings with defaults, validated before any write.

- A table of known keys with type, default and minimum: every row in api.md's settings table.
- `GetSettings(ctx) (map[string]any, error)` returns every known key, filling defaults for absent rows.
- `ValidateSettings(map[string]json.RawMessage) error` checks every key and value first; `PutSettings` then writes all in one transaction. Reject keys not in the table and any key containing `key`, `token`, `secret` or `password`, so secrets never land in SQLite.
- Store values as text (`"5"`, `"true"`, `"[]"`), as the schema expects.

#### 6. `backend/internal/render/render.go`

**Goal:** turn the master resume into Markdown, the text format later phases tailor, diff and print.

- `Master(models.Master) string`. Layout:

```markdown
# Ada Lovelace

Charleston, SC · ada@example.com · github.com/ada

## Summary

Analyst with...

## Experience

### Data Analyst, Acme

Jun 2022 to Present | Charleston, SC

<description, as written>

<impact, as written>

## Education

### BS in Computer Science, College of Charleston

May 2022 | GPA 3.80

## Skills

sql, excel, python
```

- Description, impact and the skills line appear exactly as stored, so phase 5's suggestions can find and replace them by exact text. Escape only characters that would change the Markdown structure at the start of a line (`#`, `-`, `*`).
- Empty sections are left out. The output always ends with one newline.

#### 7. `backend/internal/api/decode.go` and `respond.go`

**Goal:** one way to read request bodies, and a validation error type.

- `decodeJSON(r, &v) error`: caps the body (1 MB) and rejects trailing data. Malformed JSON returns `400 bad_request`; an unknown field or a wrong type returns `422 validation_failed` with the field name, as api.md's conventions say.
- `validationError(field, message)` builds a `422` with `details.field`.
- Map `store.ErrNotFound` to `404` and `store.ErrConflict` to `409` in the one wrapper from phase 1.

References: [encoding/json Decoder](https://pkg.go.dev/encoding/json#Decoder.DisallowUnknownFields), [http.MaxBytesReader](https://pkg.go.dev/net/http#MaxBytesReader).

#### 8. `backend/internal/api/resume.go`

**Goal:** the Resume, Preview and Saved resume routes.

- Validate in the handler before calling the store: required fields, dates (`YYYY-MM-DD` or `YYYY-MM`), date order, GPA, `preferred_mode` values.
- `POST /v1/resumes` with `"from_master": true` stores `render.Master` of the current master; `422` if neither `content` nor `from_master` is given.
- Replace the phase 1 `501` stubs for these routes.

#### 9. `backend/internal/api/settings.go`

**Goal:** `GET` and `PUT /v1/settings` over step 5.

- Also wire phase 2's keyword default: request keywords, else `profile.target_positions`, else the latest experience title.

### Electron

#### 10. `app/main/ipc-handlers.js` and `app/preload/preload.js`

- Channels: `resume:get`, `resume:putProfile`, `resume:preview`, `experiences:create`, `experiences:update`, `experiences:delete`, `education:create`, `education:update`, `education:delete`, `resumes:list`, `resumes:get`, `resumes:create`, `resumes:update`, `resumes:delete`, `settings:get`, `settings:put`.
- Preload groups them: `api.resume.*`, `api.resumes.*`, `api.settings.*`. Keep the channel list test passing.

### Renderer

#### 11. `app/renderer/js/app.js`

**Goal:** more than one view, with a navigation bar.

- Hash routes: `#/resume`, `#/jobs` (phase 1 and 2's view). Later phases add `#/tailor`, `#/tracker`, `#/settings`.
- Each view module exports `mount(container)` and returns a cleanup function, called before the next view mounts, so views do not leak listeners.
- Default route: `#/resume` when no profile exists, else `#/jobs`.

References: [hashchange event (MDN)](https://developer.mozilla.org/en-US/docs/Web/API/Window/hashchange_event).

#### 12. `app/renderer/views/resume-builder/resume-builder.js`

**Goal:** edit the master resume.

- Profile form: name, email, phone, location, links, target positions, preferred mode, summary, general skills. Save writes `PUT /v1/resume/profile`.
- Experience and education lists: add, edit in place, delete with a confirmation, move up and down. Skills are entered as comma-separated text and shown as chips.
- Show a validation error next to its field using `details.field`.
- A Preview panel shows `GET /v1/resume/preview` as plain text (rendering Markdown to HTML is not needed for the MVP).
- "Save as version" asks for a label and calls `POST /v1/resumes` with `from_master`. A list of saved versions with rename and delete.
- `textContent` only.

Styles: start `styles/base.css` (layout, typography, form controls) in this phase. Every later view uses it.

### Resume import (steps 13 to 19)

#### 13. `backend/third_party/ledongthuc-pdf/`

**Goal:** a pure-Go PDF reader that reports what hidden-text checks need.

- `github.com/ledongthuc/pdf` (BSD-3, pure Go) extracts positioned text: `Page.Content()` returns `Text{Font, FontSize, X, Y, W, S}` and rectangles. It tracks the render mode (`Tr`) and fill color (`g`, `rg`, `k`, `sc`, `scn`) internally but does not expose them.
- Vendor it under `backend/third_party/ledongthuc-pdf/` with a `replace` directive in `backend/go.mod`, and record each change in `FISTBUMP-PATCHES.md`:
  1. The `bfrange` carry fix the full build made: codes above `0xFF` in a range decoded to the wrong character (U+2022 became `"`). Proposed upstream in [ledongthuc/pdf#95](https://github.com/ledongthuc/pdf/pull/95), still open on 2026-10-10.
  2. Add `RenderMode int` and `Fill [3]float64` (RGB, with gray and CMYK converted) to `Text`.
  3. Record filled rectangles (`re` followed by `f`, `f*`, `B` or `B*`) with their fill color, in drawing order.
- Keep the library's own tests passing. Remove the vendored copy once upstream has what we need.

References: [ledongthuc/pdf](https://github.com/ledongthuc/pdf), [PDF 1.7 reference, section 9.3.6 text rendering mode and 8.6 color spaces](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/PDF32000_2008.pdf), [Go modules replace directive](https://go.dev/ref/mod#go-mod-file-replace).

#### 14. `backend/internal/resumeparse/detect.go`

**Goal:** decide the file type from its bytes before any parser sees it.

- `Detect(filename string, data []byte) (Kind, error)` returns `PDF` or `Text`, or an error carrying the `reason`, the detected type and the user message from the table above.
- Check size first (empty, over 5 MB), then magic bytes (PDF, ZIP, OLE, PNG, JPEG, GIF, WebP, HEIC `ftyp`), then UTF-8 validity for text. `net/http.DetectContentType` covers some of these but not Word or HEIC; use explicit prefixes.
- The filename is used only to name the type in messages ("ada.pdf is a Word document").

References: [List of file signatures (Wikipedia)](https://en.wikipedia.org/wiki/List_of_file_signatures), [unicode/utf8.Valid](https://pkg.go.dev/unicode/utf8#Valid).

#### 15. `backend/internal/resumeparse/extract.go`

**Goal:** visible text from a PDF, in reading order.

- `TextFromPDF(data []byte) (Extracted, error)` where `Extracted` holds `Text` and `Hidden` (word count, reasons, sample).
- Open with the vendored reader; an encrypted PDF gives `encrypted`. Recover panics from the library and return `unreadable`.
- Per page: drop hidden pieces (step 16), group the rest into lines by baseline, sort left to right, turn large vertical gaps into blank lines. Remove the word-processor artifacts the full build found: overprinted glyphs, a decorative letter some heading fonts append to every run, private-use icon glyphs (keep Symbol-font bullets as `•`), superscripts split onto their own line.
- Fewer than 5 visible words gives `no_text`.

#### 16. `backend/internal/resumeparse/hidden.go`

**Goal:** the hidden-text rules, as pure functions, so each rule has its own test.

- Every import path calls phase 1's `textclean.StripInvisible`, including `.txt`, `.md` and pasted text, and PDF text after extraction, and adds the count to `hidden_text`.

- `isHidden(t Text, page Page) (reason string, hidden bool)` applies the table above in order: render mode, off page, microscopic, same color as background. Overprints are handled while building lines (step 15).
- Background lookup walks the page's filled rectangles from the last drawn backwards and takes the first that contains the text's position and was drawn before it.
- Color distance: the largest per-channel difference in RGB.

#### 17. `backend/internal/resumeparse/parse.go`

**Goal:** turn visible text into a draft master resume, by rules.

- Name: the first line. Contact line: email, phone, location, and links found by pattern.
- Sections by fuzzy heading match (`experience`, `work history`, `employment`; `education`; `skills`, `technical skills`; `summary`, `profile`, `objective`; `projects`, `research`, `activities` parsed as experience entries).
- Entries: a title line, organization, dates (`Jun 2022 - Present`, `2019 - 2023`, `Summer 2021`), location; following bullets or paragraphs become `description`.
- Skills: split the skills section on commas, bullets, pipes and semicolons into `profile.skills`. Do not credit them to any role (the full build found that crediting the latest job was false).
- Anything that cannot be placed adds a warning; nothing is dropped silently.

#### 18. `backend/internal/api/resume_import.go`

**Goal:** the two import routes.

- `import`: decode with a body cap of 7 MB (5 MB of file grows by a third in base64); `Detect`; extract or take the text; parse; return the draft, `hidden_text`, and the notice.
- `import/apply`: validate the draft with the same rules as the Resume routes; `replace: true` deletes existing experience and education first; `save_copy` stores the imported text as a saved resume; all in one transaction. `422` names the bad entry with `details.experience_index` or `details.education_index`.

#### 19. Electron and `resume-builder.js`

**Goal:** import from a file picker or a drop, with fast feedback, while Go stays the authority.

- Main: channel `resume:importFile` shows `dialog.showOpenDialog` filtered to `pdf`, `txt`, `md`; checks the size with `fs.promises.stat` before reading (over 5 MB is refused without reading); reads the bytes and sends them base64 to `POST /v1/resume/import`. The renderer never sends a path.
- Drop: the Resume view accepts a dropped file, reads it with `File.arrayBuffer()`, and sends the bytes over `resume:importData`. Check the extension and size first for a quick message; the backend checks again.
- Paste: a "Paste text" option sends `text`.
- Review screen: the draft as editable forms (the same components as the builder), warnings at the top, and, when `hidden_text` is set, a notice such as "37 words of hidden text were found and left out (same color as the background)" with the sample under a disclosure. Apply with "Replace my current entries" unchecked by default and "Keep a copy of the original" checked.
- Errors show the backend's message as written.

### Tests and tooling

- `store` tests, one file per table file (`profile_test.go`, `experience_test.go`, `education_test.go`, `resume_test.go`, `settings_test.go`), each on a fresh temporary database from `db.Open`. Cover: create, read, update, delete, ordering, `ErrNotFound`, `ErrConflict` (experience without a profile), JSON lists round trip as `[]`, `WithTx` rollback.
- `settings_test.go`: defaults filled, one bad value in a batch writes nothing, secret-looking keys rejected, minimums enforced.
- `render_test.go`: a full master renders to the expected Markdown (golden file in `backend/testdata/render/`), empty sections left out, description text kept verbatim.
- `api_test.go`: each route's success case, `422` with `details.field`, `404`, `409`, unknown fields rejected.
- `backend/testdata/seed.sql`: a sample profile with three experiences and two education entries, for manual testing and later phases.
- `backend/testdata/resumes/`: small fixture files, generated by a test helper with `go-pdf/fpdf` where possible so they are reproducible: a plain resume; white text on white; white text on a dark sidebar (must stay); `Tr 3` text; 0.5 pt text; text placed off the page; a scanned page (image only); a password-protected PDF; a `.docx` and a `.png` renamed to `.pdf`; a UTF-16 `.txt`; an empty file.
- `detect_test.go`: every row of the rejected-uploads table, by content not extension; a 5 MB file passes and 5 MB plus one byte fails.
- `hidden_test.go`: each rule alone, the dark-sidebar case kept, and the reasons and sample reported.
- `api_test.go` (import): a `.txt` and a pasted text with tag characters report `hidden_text` with "invisible characters" and leave them out of the draft.
- `extract_test.go`: reading order, artifacts removed, `no_text`, `encrypted`, a panic in the library becomes `unreadable`.
- `parse_test.go`: sections, entries, date forms, skills to `profile.skills`, warnings for unplaced text.
- `api_test.go`: import success with `hidden_text`, each `422` reason, apply with and without `replace`, apply rollback on a bad entry.

## Done when

- A resume entered in the app is still there after quitting and relaunching.
- Invalid input shows the backend's message next to the field.
- "Save as version" creates a saved resume whose content matches the preview.
- Importing a PDF, `.txt` or `.md` resume gives a draft to review, and applying it fills the master resume.
- A Word document, an image, or a scanned PDF is refused with a message that says what to do instead, whatever its file extension.
- Hidden text in an imported PDF is left out of the draft, and the user is told how much was dropped and why.
- `go test ./...` and `npm test` pass, and each `store` table file has a test file.

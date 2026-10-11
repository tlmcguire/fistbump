# Phase 3: Master resume

Build on `main` after [phase 1](phase01.md) merges, from [api.md](api.md) ("Resume", "Saved resumes", "Settings") and [schema.md](schema.md). Can run in parallel with [phase 2](phase02.md). Overview and schedule: [roadmap.md](roadmap.md).

## Goal

The user builds a master resume (contact details, summary, experience, education, skills) in the app. It is saved in SQLite, survives a restart, and can be saved as a named version. This meets FR-1 and the resume-version part of FR-5.

This phase also builds the parts every later phase uses: the `store` package, shared types in `models`, request validation, the settings store, and a view router in the renderer.

## Scope

In:

- The `store` package for `profile`, `experiences`, `education`, `resumes` and `settings`.
- The 10 Resume routes, the 5 Saved resume routes, the 2 Settings routes, and `GET /v1/resume/preview`.
- `render.Master`: the master resume as Markdown, used by `from_master`, the preview, tailoring (phase 5) and PDF export (phase 6).
- A hash router in the renderer and the `resume-builder` view.

Out: resume import from PDF or text and summary generation (after the MVP), PDF export (phase 6), a settings view (phase 8).

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

### Tests and tooling

- `store` tests, one file per table file (`profile_test.go`, `experience_test.go`, `education_test.go`, `resume_test.go`, `settings_test.go`), each on a fresh temporary database from `db.Open`. Cover: create, read, update, delete, ordering, `ErrNotFound`, `ErrConflict` (experience without a profile), JSON lists round trip as `[]`, `WithTx` rollback.
- `settings_test.go`: defaults filled, one bad value in a batch writes nothing, secret-looking keys rejected, minimums enforced.
- `render_test.go`: a full master renders to the expected Markdown (golden file in `backend/testdata/render/`), empty sections left out, description text kept verbatim.
- `api_test.go`: each route's success case, `422` with `details.field`, `404`, `409`, unknown fields rejected.
- `backend/testdata/seed.sql`: a sample profile with three experiences and two education entries, for manual testing and later phases.

## Done when

- A resume entered in the app is still there after quitting and relaunching.
- Invalid input shows the backend's message next to the field.
- "Save as version" creates a saved resume whose content matches the preview.
- `go test ./...` and `npm test` pass, and each `store` table file has a test file.

# FistBump Backend API

Routes exposed by the Go backend (`fistbump-core`). Only the Electron main process calls it. See [architecture.md](architecture.md) for the design and [schema.md](schema.md) for tables.

JSON field names match column names in [schema.md](schema.md). The "DB" column lists the tables each route reads and writes.

## Conventions

**Transport**
- HTTP/1.1 on `127.0.0.1`, random port, JSON bodies (`Content-Type: application/json`) unless noted.
- Every request needs `Authorization: Bearer <token>`. Electron generates the token per launch and passes it to Go in the `FISTBUMP_TOKEN` environment variable. A missing or wrong token returns `401`.
- All routes live under `/v1`.

**Process contract (not HTTP)**
- Flag: `--data-dir <path>`.
- stdout: exactly one line, `PORT=<n>`, once the listener is ready. Nothing else is written to stdout.
- stderr: logs. Request logs never include `Authorization` or request bodies for secret-bearing routes.
- The process exits when stdin closes.

**Formats**
- Integer ids. Dates are `YYYY-MM-DD`. Timestamps are RFC 3339 UTC.
- SQL `NULL` is JSON `null`. JSON columns (`skills`, `target_positions`, and others) are JSON arrays.
- Unknown fields in a request body are rejected with `422`. Read-only fields (`id`, `created_at`, `updated_at`, `archived_at`, `trashed_at`, `base_content`) are ignored on write; archive, trash and restore have their own routes.
- List routes return a bare JSON array unless noted.
- Enumerated fields use the exact values allowed by the schema `CHECK` constraints. Other values return `422`.

**Error shape**

```json
{ "error": { "code": "validation_failed", "message": "preferred_mode is not valid", "details": { "field": "preferred_mode" } } }
```

| Status | `code` | Meaning |
|--------|--------|---------|
| 400 | `bad_request` | Malformed JSON or query |
| 401 | `unauthorized` | Missing or wrong bearer token |
| 404 | `not_found` | No such resource |
| 409 | `conflict` | State forbids the action (pending suggestions, job with applications) |
| 422 | `validation_failed` | Well-formed but invalid fields |
| 500 | `internal` | Unexpected error (panic recovery also lands here) |
| 501 | `not_implemented` | The route is planned and registered but not built yet |
| 502 | `upstream_error` | A remote API or connector failed |
| 503 | `ai_unavailable` | The requested engine cannot run (only when the caller forced one) |

AI routes do not fail on a missing model in `auto` mode. They fall back to the `rules` engine and report the engine that ran.

## Shared shapes

```jsonc
// Profile
{ "id": 1, "full_name": "", "email": null, "phone": null, "home_location": null,
  "target_positions": ["Backend Engineer"], "preferred_mode": "Remote|Hybrid|On-site|Any|null",
  "min_desired_pay": null, "clearance_certs": [], "summary": null, "skills": [], "links": [], "created_at": "", "updated_at": "" }

// Experience
{ "id": 1, "profile_id": 1, "company_name": "", "job_title": "", "start_date": "2022-06-01", "end_date": null,
  "location": null, "description": null, "impact": null, "skills": ["go", "sql"], "sort_order": 0 }

// Education
{ "id": 1, "profile_id": 1, "institution": "", "degree_level": null, "discipline": null,
  "graduation_date": null, "gpa": null, "honors": null, "details": null, "sort_order": 0 }

// ResumeVersion (resumes table: a saved resume document)
{ "id": 1, "profile_id": 1, "version_label": "", "target_position": null, "content": "", "created_at": "", "updated_at": "" }

// Job
{ "id": 1, "company_name": "", "position_title": "", "listing_url": null, "source": "pasted|manual|greenhouse",
  "external_id": null, "work_mode": null, "employment_type": null, "location": null, "pay_min": null, "pay_max": null,
  "description": null, "raw_text": null,
  "req_tech_skills": [], "pref_tech_skills": [], "req_soft_skills": [], "pref_soft_skills": [], "requirements": [],
  "close_date": null, "created_at": "", "searched_at": "", "archived_at": null, "trashed_at": null }

// Revision
{ "id": 1, "job_id": 1, "base_resume_id": null, "engine": "local|remote|rules",
  "status": "queued|running|done|failed|canceled", "error": null, "created_at": "", "completed_at": null }

// Suggestion
{ "id": 1, "revision_id": 1, "section": "summary|experience|education|skills", "target_id": null,
  "original_text": "", "proposed_text": "", "state": "pending|accepted|rejected|edited", "edited_text": null }

// TailoredResume
{ "id": 1, "job_id": 1, "revision_id": null, "base_resume_id": null, "content": "", "base_content": "", "created_at": "" }

// Application
{ "id": 1, "job_id": 1, "profile_id": 1, "base_resume_id": null, "tailored_resume_id": null,
  "status": "Saved|Applied|Interviewing|Offered|Rejected", "date_applied": null, "next_step_date": null,
  "exported_pdf_path": null, "notes": null, "archived_at": null, "created_at": "", "updated_at": "" }
```

`archived_at` and `trashed_at` are timestamps or `null`. Archive means keep but hide; it never changes an application's `status`, so restoring returns it to its stage. A trashed job can be restored until `jobs.trash_days` have passed, then storage cleanup deletes it.

`target_id` on a suggestion is a row id in the table named by `section` (`experiences.id` or `education.id`). It is `null` for `summary` and `skills`.

## Health

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `GET /v1/health` | none | `{ "status": "ok", "version": "0.1.0", "schema_version": 1 }` | Reads `PRAGMA user_version` |

Electron calls this after the port line to confirm readiness.

## Resume (`resume.go`)

The master resume is `profile` plus `experiences` plus `education`. One profile exists in practice.

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `GET /v1/resume` | none | `{ "profile": Profile \| null, "experiences": [Experience], "education": [Education], "skills": ["go"] }`. `skills` is derived: the de-duplicated union of `profile.skills` and `experiences.skills`. | SELECT `profile`, `experiences` and `education` (each by `sort_order`) |
| `PUT /v1/resume/profile` | Profile fields without read-only fields. `full_name` required. | Profile. Creates the row if none exists. | UPSERT `profile`, sets `updated_at` |
| `GET /v1/resume/experiences` | none | `[Experience]` ordered by `sort_order` | SELECT `experiences` |
| `POST /v1/resume/experiences` | Experience without read-only fields or `profile_id`. `company_name` and `job_title` required. A `null` `end_date` means the role is current. | `201` Experience. `sort_order` defaults to the next value. `409` if no profile exists. | INSERT `experiences` |
| `PUT /v1/resume/experiences/{id}` | Experience fields (full replace) | Experience. `404` if missing. | UPDATE `experiences` |
| `DELETE /v1/resume/experiences/{id}` | none | `204` | DELETE `experiences` |
| `GET /v1/resume/education` | none | `[Education]` ordered by `sort_order` | SELECT `education` |
| `POST /v1/resume/education` | Education without read-only fields or `profile_id`. `institution` required. | `201` Education. `409` if no profile exists. | INSERT `education` |
| `PUT /v1/resume/education/{id}` | Education fields (full replace) | Education | UPDATE `education` |
| `DELETE /v1/resume/education/{id}` | none | `204` | DELETE `education` |

Validation: `end_date` must not precede `start_date`. `gpa` is between 0 and 5. Skills are trimmed, lower-cased for matching, and de-duplicated. Experience `description` and `impact` are the text AI suggestions target.

## Resume import and generation (`resume_import.go`)

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `POST /v1/resume/import` | `{ "filename": "cv.pdf", "data_base64": "..." }` or `{ "text": "..." }`, plus `"engine": "rules"` (default) or `"auto"`. PDF, `.txt` and `.md`, at most 5 MB. | `{ "text", "draft": { "profile", "experiences", "education", "skills", "warnings" }, "engine", "notice" }`. `422` for a PDF without a text layer. With `auto`, a model structures the text when one is available; values not found in the text are dropped, and failures fall back to rules with a `notice`. | None (preview) |
| `POST /v1/resume/import/apply` | `{ "draft": {...}, "replace": false, "save_copy"?: { "version_label", "content" } }` | Same as `GET /v1/resume`. Empty draft fields keep current values. `replace` deletes existing experience and education first. `422` names the bad entry with `details.experience_index` or `details.education_index`. | UPSERT `profile`, INSERT `experiences`, `education`, optionally `resumes` |
| `POST /v1/resume/summary` | `{ "engine"?: "auto" }` | `{ "summary", "engine" }`. A draft summary for review; not saved. Rules fallback: latest title, years, top skills. | Reads master tables |
| `GET /v1/resume/preview` | none | `{ "markdown" }` rendered master resume | Reads master tables |
| `POST /v1/resume/export` | `{ "format": "pdf" }` or `"md"` | Raw bytes with `Content-Disposition` | Reads master tables |

## Saved resumes (`resume.go`)

Saved resume documents used as the base for tailoring.

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `GET /v1/resumes` | none | `[ResumeVersion]` without `content`, newest first | SELECT `resumes` |
| `POST /v1/resumes` | `{ "version_label", "target_position"?, "content"?, "from_master"? }`. Either `content` or `"from_master": true` is required. `from_master` renders the master resume to text. | `201` ResumeVersion | Reads master tables when `from_master`. INSERT `resumes`. |
| `GET /v1/resumes/{id}` | none | ResumeVersion with `content` | SELECT `resumes` |
| `PUT /v1/resumes/{id}` | `{ "version_label"?, "target_position"?, "content"? }` | ResumeVersion | UPDATE `resumes`, sets `updated_at` |
| `DELETE /v1/resumes/{id}` | none | `204`. Revisions, tailored resumes, and applications that referenced it keep their rows with `base_resume_id` set to `null`. | DELETE `resumes` (`ON DELETE SET NULL` on dependents) |

## Jobs (`jobs.go`)

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `POST /v1/jobs/parse` | `{ "raw_text": "..." }` | `{ "description": "", "req_tech_skills": [], "pref_tech_skills": [], "req_soft_skills": [], "pref_soft_skills": [], "requirements": [], "inferred": { "company_name", "position_title", "location", "work_mode" } }` | None (stateless preview) |
| `POST /v1/jobs` | `{ "company_name", "position_title", "source": "pasted" \| "manual", "raw_text"?, "listing_url"?, "location"?, "work_mode"?, "employment_type"?, "pay_min"?, "pay_max"?, "close_date"? }`. `raw_text` is required for `pasted`. A missing `company_name` or `position_title` is taken from the parser's inferred values, else `422`. | `201` Job. Runs `parser` to fill `description`, the four skill lists, and `requirements`. | INSERT `jobs` |
| `GET /v1/jobs` | Query: `q` (company or title substring), `source`, `archived` (omit to exclude archived jobs, `include`, or `only`), `trashed` (omit to exclude trashed jobs, or `only` for the Trash), `limit`, `offset` | `[Job]` without `raw_text` and `description`, newest `searched_at` first | SELECT `jobs` |
| `GET /v1/jobs/{id}` | none | Job | SELECT `jobs` |
| `PUT /v1/jobs/{id}` | Any subset of the writable Job fields. Changing `raw_text` re-runs the parser unless the skill lists are also supplied. | Job | UPDATE `jobs` |
| `DELETE /v1/jobs/{id}` | none | `204`. `409` if an application references the job. | DELETE `jobs`. Cascades to `revisions`, `suggestions`, `tailored_resumes`. `applications.job_id` is `RESTRICT`. |
| `POST /v1/jobs/{id}/analyze` | none | see below | SELECT `jobs`, `profile`, `experiences`. No writes. |
| `POST /v1/jobs/{id}/archive` | `{ "archive_application"?: true }` | Job with `archived_at` set. With `archive_application`, the job's application is archived too. | UPDATE `jobs.archived_at`, optionally `applications.archived_at` |
| `POST /v1/jobs/{id}/unarchive` | none | Job with `archived_at` null. The job's application is restored too, if it is archived. | UPDATE `jobs.archived_at`, `applications.archived_at` |
| `POST /v1/jobs/{id}/trash` | none | Job with `trashed_at` set. `409` with `details.application_id` if an application references the job. | UPDATE `jobs.trashed_at` |
| `POST /v1/jobs/{id}/restore` | none | Job with `trashed_at` null. An archived job returns to the archived list. | UPDATE `jobs.trashed_at` |

Pasted and manual jobs always create a new row. Only connector imports upsert, on `(source, external_id)`.

Archive, trash and restore examples:

```jsonc
// POST /v1/jobs/7/archive   { "archive_application": true }
// 200
{ "id": 7, "company_name": "Acme", "position_title": "Data Analyst", "archived_at": "2026-10-10T18:00:00Z", "trashed_at": null, ... }

// POST /v1/jobs/7/trash   (job has an application)
// 409
{ "error": { "code": "conflict", "message": "job has an application", "details": { "application_id": 3 } } }

// GET /v1/jobs?trashed=only
// 200
[{ "id": 9, "company_name": "Globex", "position_title": "Analyst", "trashed_at": "2026-10-08T14:12:00Z", ... }]
```

Analyze compares each job skill list with the master resume's skills. The master skills are `experiences.skills` plus `profile.clearance_certs`. Evidence also searches experience `description` and `impact` text. The analysis is deterministic, never calls an AI engine, and targets under 1.5 s.

```json
{
  "job_id": 1,
  "required": {
    "tech": { "matched": [{ "skill": "go", "experience_ids": [2, 5] }], "missing": ["kubernetes"] },
    "soft": { "matched": [], "missing": ["mentoring"] }
  },
  "preferred": {
    "tech": { "matched": [], "missing": ["terraform"] },
    "soft": { "matched": [], "missing": [] }
  },
  "score": 0.62,
  "duration_ms": 14
}
```

`score` is the fraction of required (tech and soft) skills matched.

## Revisions (`revisions.go`)

A revision is one suggestion-generating pass for a job.

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `POST /v1/revisions` | `{ "job_id": 1, "base_resume_id"?: 1, "engine": "auto" }`. `engine` is `auto` (default), `local`, `remote`, or `rules`. | `202` Revision with `status: "queued"`. `engine` holds the engine chosen at creation. `503 ai_unavailable` if a forced engine cannot run. | INSERT `revisions`. Suggestions are INSERTed as the engine produces them. |
| `GET /v1/revisions` | Query: `job_id` | `[Revision]` newest first | SELECT `revisions` |
| `GET /v1/revisions/{id}` | none | Revision plus `"suggestions": [Suggestion]` | SELECT `revisions`, `suggestions` |
| `POST /v1/revisions/{id}/cancel` | none | Revision with `status: "canceled"`. `409` unless `queued` or `running`. | UPDATE `revisions` (`status`, `completed_at`) |
| `DELETE /v1/revisions/{id}` | none | `204`. Tailored resumes built from it keep their rows with `revision_id` set to `null`. | DELETE `revisions` (cascades to `suggestions`) |
| `POST /v1/revisions/{id}/suggestions/{sid}/accept` | none | Suggestion with `state: "accepted"` | UPDATE `suggestions.state` |
| `POST /v1/revisions/{id}/suggestions/{sid}/reject` | none | Suggestion with `state: "rejected"` | UPDATE `suggestions.state` |
| `POST /v1/revisions/{id}/suggestions/{sid}/edit` | `{ "edited_text": "..." }` (non-empty) | Suggestion with `state: "edited"` | UPDATE `suggestions.edited_text`, `state` |

Generation runs in the background. Electron polls `GET /v1/revisions/{id}` until `status` is `done`, `failed`, or `canceled`. In `auto` mode, if the chosen engine errors mid-run, the backend retries with the next engine and updates `revisions.engine` to the engine that produced the suggestions. A `failed` revision has `error` set. Each suggestion has `section` and `target_id` pointing at its source text, with `original_text` copied from it. The prompt contains only master resume text and the job text.

## Tailored resumes (`tailored.go`)

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `POST /v1/tailored-resumes` | `{ "revision_id": 1 }` | `201` TailoredResume. `409 conflict` with `details.pending` (count) if any suggestion is `pending`, or if the revision is not `done`. | Reads `revisions`, `suggestions`, `resumes` (or master tables). INSERT `tailored_resumes` with `job_id` and `base_resume_id` copied from the revision, and `base_content` set to the resume text the revision started from. |
| `GET /v1/tailored-resumes` | Query: `job_id` | `[TailoredResume]` without `content` and `base_content` | SELECT `tailored_resumes` |
| `GET /v1/tailored-resumes/{id}` | none | TailoredResume | SELECT `tailored_resumes` |
| `GET /v1/tailored-resumes/{id}/diff` | none | `{ "hunks": [{ "section", "target_id", "status": "unchanged\|changed\|added\|removed", "before", "after", "words": [{ "op": "eq\|add\|del", "text" }] }] }` | SELECT `tailored_resumes`. Compares `base_content` with `content`. Computed in `diff`, not stored. |
| `POST /v1/tailored-resumes/{id}/export` | `{ "format": "pdf" }` or `"md"` | Raw bytes. `Content-Type: application/pdf` or `text/markdown`, plus `Content-Disposition: attachment; filename="..."`. | SELECT `tailored_resumes`. No writes. |
| `DELETE /v1/tailored-resumes/{id}` | none | `204`. Applications that used it keep their rows with `tailored_resume_id` set to `null`. | DELETE `tailored_resumes` |

Final text per suggestion: `edited_text` if `state` is `edited`, `proposed_text` if `accepted`, `original_text` if `rejected`. The base is the `resumes.content` of the revision's `base_resume_id`, or the rendered master resume when that is `null`. `content` is stored in full.

Export returns bytes. Electron shows the save dialog and writes the file, so Go never accepts a filesystem path from the UI. After saving, Electron records the path with `PUT /v1/applications/{id}` (`exported_pdf_path`) when the tailored resume belongs to an application. On PDF failure the response is `500` and the UI offers `md`.

## Applications (`applications.go`)

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `GET /v1/applications` | Query: `status`, `q` (company or title substring), `archived` (omit to exclude archived applications, `include`, or `only`) | `[Application]`, each with `company_name` and `position_title` from the job | SELECT `applications` JOIN `jobs` |
| `POST /v1/applications` | `{ "job_id", "base_resume_id"?, "tailored_resume_id"?, "status"?, "date_applied"?, "next_step_date"?, "notes"? }`. `status` defaults to `Saved`. `profile_id` is set to the current profile. | `201` Application. `422` for an unknown job or resume id. `409` with `details.application_id` if the job is already tracked. | INSERT `applications` |
| `GET /v1/applications/{id}` | none | Application | SELECT `applications` |
| `PUT /v1/applications/{id}` | Any subset of `status`, `date_applied`, `next_step_date`, `notes`, `base_resume_id`, `tailored_resume_id`, `exported_pdf_path` | Application. Setting `status` to `Applied` with no `date_applied` sets today's date. | UPDATE `applications`, sets `updated_at` |
| `DELETE /v1/applications/{id}` | none | `204` | DELETE `applications` |
| `POST /v1/applications/bulk-status` | `{ "ids": [1, 2], "status": "Rejected" }` (1 to 1000 ids) | `[Application]` updated | UPDATE `applications` |
| `POST /v1/applications/{id}/archive` | none | Application with `archived_at` set. `status` is unchanged. | UPDATE `applications.archived_at` |
| `POST /v1/applications/{id}/unarchive` | none | Application with `archived_at` null | UPDATE `applications.archived_at` |
| `POST /v1/applications/bulk-archive` | `{ "ids": [1, 2], "archived": true }` (1 to 1000 ids). `false` restores. | `[Application]` updated | UPDATE `applications.archived_at` |

One job has at most one application; the database enforces it with a unique index.

```jsonc
// POST /v1/applications   { "job_id": 7 }   (job already tracked)
// 409
{ "error": { "code": "conflict", "message": "job already has an application", "details": { "application_id": 3 } } }

// POST /v1/applications/3/archive
// 200
{ "id": 3, "job_id": 7, "status": "Interviewing", "archived_at": "2026-10-10T18:00:00Z", ... }

// POST /v1/applications/bulk-archive   { "ids": [3, 4], "archived": false }
// 200
[{ "id": 3, "status": "Interviewing", "archived_at": null, ... }, { "id": 4, "status": "Applied", "archived_at": null, ... }]
```

## Connectors (`connectors.go`)

The only routes that can reach a job site. Each call needs the connector enabled. Greenhouse is on by default but never searches on its own: the UI calls `fetch` only when the user presses Search, never when job search opens. Requests carry only public board tokens and job ids; keywords and the resume stay on the computer. The user can turn the connector off with `PUT /v1/connectors/greenhouse` and `{ "enabled": false }`. Imports write `source = 'greenhouse'`, `external_id` (the Greenhouse job id), `listing_url`, `company_name`, `position_title`, `location`, `raw_text`, then run the parser for `description`, the skill lists, and `requirements`.

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `GET /v1/connectors` | none | `[{ "id": "greenhouse", "name": "Greenhouse", "enabled": true }]` | Reads `settings` (`connectors.greenhouse.enabled`) |
| `PUT /v1/connectors/{id}` | `{ "enabled": true }` | Connector | UPSERT `settings` |
| `POST /v1/connectors/{id}/fetch` | `{ "query": { "keywords": ["backend engineer"], "location": "", "work_mode": "", "categories"?: ["cybersecurity"], "boards"?: ["acme"], "limit"?: 100 } }`. With `boards` and `categories` both omitted, every curated board plus `connectors.greenhouse.boards` is searched. Keywords default to `target_positions`, then the latest job title; `work_mode` defaults to `preferred_mode`. | `{ "postings": [{ "external_id", "board", "position_title", "company_name", "listing_url", "location", "work_mode", "updated_at", "raw_text", "score", "title_score", "skill_score" }], "matched", "boards_searched", "boards_total", "postings_scanned", "errors": [{ "board", "message" }], "keywords_used", "work_mode_used" }`. `raw_text` is filled for the top-ranked postings only. Each posting also has `saved_job_id` and `archived` when it is already in `jobs`. `422` when no keywords can be found. | Reads `settings`, `profile`, `experiences`. No writes. Listings cached on disk. |
| `POST /v1/connectors/{id}/import` | `{ "external_ids": ["123"] }` (required, 1 or more) from a recent search. | `{ "imported": [Job], "refreshed": [Job] }`. `422` for an id not in recent results. | Upsert `jobs` on `(source, external_id)`. Existing rows get a new `searched_at`. |
| `GET /v1/connectors/greenhouse/postings/{external_id}` | none | One posting from recent search results with `raw_text` filled, for reading in the app. `404` if it is not in recent results. | None. Fetches the description if not cached. |
| `GET /v1/connectors/greenhouse/categories` | none | `{ "categories": [{ "id": "ai_ml", "name": "AI and machine learning", "parent": "technical", "board_count": 30, "enabled": false }], "custom_boards": [], "verified_at": "", "notice": "..." }`. The notice states the lists are not exhaustive. `parent` is `null` for top-level categories. | Reads `connectors.greenhouse.categories` from `settings`. Category data comes from embedded `boards.json`. |
| `PUT /v1/connectors/greenhouse/categories/{category}` | `{ "enabled": true }` | The category entry. `404` for an unknown category id. | Updates `connectors.greenhouse.categories` in `settings`. `enabled` marks an industry the user is open to. The backend does not apply it to searches; the UI pre-selects these industries as the search's `categories`, which the user can change for one search. |
| `POST /v1/connectors/greenhouse/boards` | `{ "board": "acme" }` | `{ "board": "acme", "company": "Acme Inc", "open_postings": 42 }`. `422` if the board does not exist. | Appends to `connectors.greenhouse.boards` in `settings` after validating the token upstream |
| `DELETE /v1/connectors/greenhouse/boards/{board}` | none | `204` | Removes the token from `connectors.greenhouse.boards` |

Errors: `409` if the connector is disabled, `422` for a bad query, `502` with the upstream status on failure or rate limit. Requests use HTTPS with a timeout and a response size cap.

## AI (`ai.go`)

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `GET /v1/ai/status` | none | `{ "mode": "auto", "active_engine": "local", "local": { "model": "", "installed": true, "running": false }, "remote": { "configured": true, "needs_key": false, "base_url": "", "model": "", "has_key": true } }`. `needs_key` is true for a hosted provider (not loopback) with no key in memory; such a provider is skipped in `auto` mode and a forced `remote` returns `503`. | Reads `settings` |
| `PUT /v1/ai/mode` | `{ "mode": "auto" }`. One of `auto`, `local`, `remote`, `rules`. | Same as status | UPSERT `settings` (`ai.mode`) |
| `PUT /v1/ai/remote` | `{ "base_url": "https://api.example.com/v1", "model": "name", "api_key": "..." }`. `api_key` is optional (omit for keyless Ollama). `base_url` must be HTTPS unless the host is loopback. | Same as status. The key is never echoed back. | UPSERT `settings` (`ai.remote.base_url`, `ai.remote.model`). **The key is held in memory only and never written to the DB.** |
| `DELETE /v1/ai/remote` | none | `204` | Clears the remote settings and drops the in-memory key |
| `POST /v1/ai/test` | `{ "engine": "local" }` or `"remote"` | `{ "ok": true, "latency_ms": 840, "sample": "pong", "error": null }`. Failures return `ok: false` with `error`, still `200`. | None |

Electron calls `PUT /v1/ai/remote` at every launch to supply the stored key. See "API keys and secrets" in architecture.md.

## Models (`models.go`)

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `GET /v1/models` | none | `[{ "id", "repo", "file", "size_bytes", "min_ram_gb", "installed": false, "fits_this_machine": true, "selected": false }]` | Catalog from embedded `catalog.json`, installed state from disk, `selected` from `settings` (`ai.selected_model`) |
| `GET /v1/models/{id}` | none | One model entry | Same |
| `POST /v1/models/{id}/download` | none | `202` `{ "download_id": "abc" }`. `409` if already installed or downloading. | None. Writes a `.part` file in the data dir. |
| `GET /v1/models/downloads/{download_id}` | none | `{ "state": "running\|verifying\|done\|failed\|cancelled", "bytes": 0, "total": 0, "error": null }` | None |
| `DELETE /v1/models/downloads/{download_id}` | none | `204`. Stops the transfer and keeps the `.part` file so a later download can resume. | None |
| `DELETE /v1/models/{id}` | none | `204`. Stops the local server first if this model is loaded. | Deletes the file. Clears `ai.selected_model` in `settings` if selected. |
| `PUT /v1/models/token` | `{ "token": "hf_..." }` (optional Hugging Face token for gated models) | `204` | In memory only |
| `DELETE /v1/models/token` | none | `204` | Drops the in-memory token |

Selecting a model is a settings change: `PUT /v1/settings` with `ai.selected_model`. Downloads use HTTP range requests to resume, verify SHA-256 against the catalog, then rename the `.part` file atomically. A hash mismatch ends in `failed` and the file is deleted.

## Settings (`settings.go`)

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `GET /v1/settings` | none | Map of every known key to its typed value, with defaults filled in | SELECT `settings`, defaults applied in `store/settings.go` |
| `PUT /v1/settings` | A subset of known keys | Full settings map | UPSERT `settings` in one transaction. Every value is validated before any is written, so a `422` (unknown key, secret-looking key, wrong type, or below a minimum) changes nothing. |

The table stores values as text. The API converts them to JSON types.

| Key | Type | Default | Seeded by `001_init.sql` |
|-----|------|---------|--------------------------|
| `ai.mode` | string: `auto`, `local`, `remote`, `rules` | `"auto"` | Yes |
| `ai.selected_model` | string | `""` | Yes |
| `ai.idle_minutes` | integer, at least 1 | `5` | Yes |
| `ai.remote.base_url` | string | `""` | No |
| `ai.remote.model` | string | `""` | No |
| `jobs.retention_days` | integer, at least 1 | `30` | Yes |
| `jobs.trash_days` | integer, at least 1 | `7` | Yes |
| `connectors.greenhouse.enabled` | boolean | `true` | Yes |
| `connectors.greenhouse.boards` | array of strings | `[]` | Yes |
| `connectors.greenhouse.categories` | array of strings: industries the user is open to, `[]` for all | `[]` | No |

Settings never hold secrets.

## Storage (`storage.go`)

| Route | Request | Response | DB |
|-------|---------|----------|----|
| `GET /v1/storage` | none | `{ "data_dir", "db_bytes", "models_bytes", "cache_bytes", "partial_downloads_bytes", "job_count", "total_bytes" }` | `COUNT` on `jobs`, file sizes from disk |
| `POST /v1/storage/cleanup` | `{ "targets": ["cache", "partial_downloads", "old_jobs", "trash", "unused_models"], "older_than_days"?: 30, "dry_run": false }`. `older_than_days` defaults to `jobs.retention_days` and must be at least 1. | `{ "freed_bytes": 0, "removed": { "old_jobs": 12, "trash": 2, "files": 3 }, "dry_run": false }` | `old_jobs`: DELETE from `jobs` where `searched_at` is older than the cutoff, the job is neither archived nor trashed, and no `applications` row references it. `trash`: DELETE trashed jobs whose `trashed_at` is older than `jobs.trash_days` (`older_than_days` does not apply). To empty the Trash at once, the UI deletes each trashed job with `DELETE /v1/jobs/{id}`. Cascades to `revisions`, `suggestions`, `tailored_resumes`. Then `VACUUM`. |

The UI calls with `dry_run: true` first to preview removals.

## Route summary

| Group | Routes |
|-------|--------|
| Health | 1 |
| Resume | 10 |
| Resume import and generation | 5 |
| Saved resumes | 5 |
| Jobs | 11 |
| Revisions | 8 |
| Tailored resumes | 6 |
| Applications | 9 |
| Connectors | 9 |
| AI | 5 |
| Models | 8 |
| Settings | 2 |
| Storage | 2 |

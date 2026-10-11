# Phase 7: Application tracker

Build on `main` after [phase 4](phase04.md) merges, from [api.md](api.md) ("Applications", "Jobs") and [schema.md](schema.md). Can run in parallel with phases [5](phase05.md), [6](phase06.md) and [8](phase08.md). Overview and schedule: [roadmap.md](roadmap.md).

## Goal

The user tracks every application in one table: its stage, the date applied, the next step, notes, and which resume was sent. Finished applications are archived, which hides them without changing their stage. This meets the application part of FR-5.

## Scope

In:

- The applications store and all 9 Applications routes.
- Job `archive` and `unarchive`.
- Job `trash` and `restore`, if time allows (first to cut; see [roadmap.md](roadmap.md)).
- The `tracker` view, a Track button on saved jobs and tailored resumes, and recording the exported PDF path from phase 6.

Out: CSV export, reminders and notifications, a kanban board, contacts (after the MVP). Garbage collection of trashed and old jobs is phase 9.

## Rules

From [schema.md](schema.md) and [api.md](api.md):

- **One application per job.** The unique index on `applications(job_id)` enforces it. Creating a second returns `409` with `details.application_id`, and the UI opens the existing one.
- **Status is the pipeline stage:** `Saved` (default), `Applied`, `Interviewing`, `Offered`, `Rejected`. Any stage may move to any other; the tracker does not enforce an order, because real pipelines skip and go back.
- **Applied date.** Setting `status` to `Applied` with no `date_applied` sets today's local date. The UI sends the local date; the backend uses its own local date only when the request has none.
- **Archive is not a status.** `archived_at` hides an application; restoring returns it at the same stage.
- **Job archive** hides a saved job and, with `archive_application`, its application. Unarchiving a job restores its application too.
- **Trash** applies to jobs only. A job with an application cannot be trashed (`409`); archive it instead. A trashed job is hidden and can be restored until `jobs.trash_days` pass; phase 9's cleanup deletes it after that.

## Routes

All as written in [api.md](api.md): the 9 Applications routes, and `POST /v1/jobs/{id}/archive`, `/unarchive`, `/trash`, `/restore`.

```jsonc
// POST /v1/applications   { "job_id": 7, "tailored_resume_id": 3 }
// 201
{ "id": 5, "job_id": 7, "profile_id": 1, "base_resume_id": null, "tailored_resume_id": 3,
  "status": "Saved", "date_applied": null, "next_step_date": null, "exported_pdf_path": null,
  "notes": null, "archived_at": null, "created_at": "...", "updated_at": "..." }

// PUT /v1/applications/5   { "status": "Applied" }
// 200: date_applied set to today

// GET /v1/applications?archived=only
// 200: [Application], each with company_name and position_title
```

- `GET /v1/applications` sorts by `next_step_date` (soonest first, nulls last), then `updated_at` newest first. Add the order to api.md.
- `tailored_resume_id` must belong to the same job; otherwise `422`. Add the rule to api.md.
- Bulk routes take 1 to 1,000 ids and run in one transaction; an unknown id fails the whole request with `404` and `details.id`.

| Status | `code` | When |
|--------|--------|------|
| 404 | `not_found` | Unknown application or job id, including one id in a bulk request |
| 409 | `conflict` | Second application for a job; trashing a job with an application; no profile |
| 422 | `validation_failed` | Unknown status, bad date, unknown job or resume id, tailored resume for another job, empty or oversized `ids` |

## Files, in build order

Progress:

- [ ] 1. `models.go` Application
- [ ] 2. `application.go` (store)
- [ ] 3. `job.go` (store) archive and trash
- [ ] 4. `applications.go` (api)
- [ ] 5. `jobs.go` (api) archive and trash
- [ ] 6. `ipc-handlers.js`, `preload.js`
- [ ] 7. `tracker.js`
- [ ] 8. Track buttons and archive actions
- [ ] 9. Exported PDF path
- [ ] Tests and tooling

### Backend

#### 1. `backend/internal/models/models.go`

- `Application` with JSON tags matching api.md, plus `CompanyName` and `PositionTitle` (`company_name`, `position_title`) filled by list queries.

#### 2. `backend/internal/store/application.go`

**Goal:** the applications table.

- `CreateApplication` (sets `profile_id` to the single profile), `GetApplication`, `ListApplications` (filters `status`, `q` on company or title, `archived`: omitted, `include`, `only`; joins `jobs`), `UpdateApplication` (any subset; sets `updated_at`), `DeleteApplication`.
- `BulkStatus(ids, status)` and `BulkArchive(ids, archived)` in one transaction each; return the updated rows.
- `ArchiveApplication`, `UnarchiveApplication`.
- On the unique index error, look up the existing application and return a conflict error carrying its id, so the API can fill `details.application_id`.

#### 3. `backend/internal/store/job.go`

- `ArchiveJob(id, withApplication)`, `UnarchiveJob(id)` (also clears the application's `archived_at`), `TrashJob(id)` (`ErrConflict` with the application id when one exists), `RestoreJob(id)`. Each in one transaction where it touches two tables.

#### 4. `backend/internal/api/applications.go`

**Goal:** the 9 routes.

- Validate statuses, dates and ids in the handler; check `tailored_resume_id` belongs to `job_id`.
- Fill `date_applied` when status becomes `Applied` and the request has no date.

#### 5. `backend/internal/api/jobs.go`

- The four job routes over step 3. Replace their `501` stubs.

### Electron

#### 6. `app/main/ipc-handlers.js` and `app/preload/preload.js`

- Channels: `applications:list`, `applications:create`, `applications:get`, `applications:update`, `applications:delete`, `applications:bulkStatus`, `applications:archive`, `applications:unarchive`, `applications:bulkArchive`, `jobs:archive`, `jobs:unarchive`, `jobs:trash`, `jobs:restore`.
- Preload: `api.applications.*` and the four job methods on `api.jobs`.

### Renderer

#### 7. `app/renderer/views/tracker/tracker.js`

**Goal:** the tracker table.

- Route `#/tracker`.
- Columns: company, position, status, date applied, next step, resume (tailored resume name or "Master"), notes. Click a company or position to open the job in the Jobs view.
- Inline edits: status as a select, dates as `<input type="date">`, notes as a text area that saves on blur. Each edit is one `PUT`; on error, put the old value back and show the message.
- Filters: status (multi-select), search by company or title, and an Active, Archived, All switch.
- Select rows with checkboxes for bulk status and bulk archive.
- Show next steps due today or overdue first, compared with the local date (the full build compared with UTC and marked today's steps overdue in the US evening).
- Empty state: "No applications yet. Track a job from the Jobs or Tailor view."

#### 8. `job-parser.js` and `diff-view.js`

- **Track** on a saved job creates an application (`Saved`). If the job is tracked already, it opens the tracker on that row.
- **Track with this resume** on a built tailored resume creates an application with `tailored_resume_id`, or sets it on the existing application after asking.
- **Archive** and **Unarchive** on a saved job, with "Also archive the application" checked by default when one exists. The Saved list gets an Active, Archived switch.
- **Move to Trash** and a Trash list with Restore, if trash is built. Trash on a tracked job shows the `409` message and offers Archive.

#### 9. Exported PDF path

- After a tailored resume export saves (phase 6), if an application uses that tailored resume, main calls `PUT /v1/applications/{id}` with `exported_pdf_path`. The tracker shows "PDF saved" with "Show in folder".
- If phase 6 merges after this phase, do this step in phase 6 instead.

### Tests and tooling

- `application_test.go` (store): CRUD, list filters and order, the join fills company and title, unique index conflict carries the existing id, bulk all-or-nothing, archive keeps status.
- `job_test.go` (store): archive with and without the application, unarchive restores the application, trash conflict, restore of an archived job returns it to the archived list.
- `api_test.go`: each route; `Applied` fills today; tailored resume from another job `422`; bulk with an unknown id writes nothing.
- Manual: move one application through every stage, archive and restore it, and check its stage is unchanged.

## Done when

- An application moves through Saved, Applied, Interviewing, and Offered or Rejected, with the applied date set automatically.
- Archiving hides an application without changing its status, and restoring brings it back at the same stage.
- A job can have only one application, and the UI leads to the existing one.
- The tracker shows which resume was sent and where its PDF was saved.
- `go test ./...` and `npm test` pass.

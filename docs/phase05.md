# Phase 5: Tailoring loop (rules engine)

Build on `main` after [phase 4](phase04.md) merges, from [api.md](api.md) ("Revisions", "Tailored resumes"), [architecture.md](architecture.md) ("AI layer", "Core workflow") and [schema.md](schema.md). Overview and schedule: [roadmap.md](roadmap.md).

## Goal

For a saved job, the app proposes changes to the resume. The user reviews each one side by side, accepts, rejects or edits it, and saves the result as a tailored resume. Every change goes through review; nothing is applied without the user's choice. This meets NFR-4 and the review half of FR-3.

This phase uses only the `rules` engine, so the whole loop works with no model and no network. Phase 8 adds local and remote models behind the same `Provider` interface.

## Scope

In:

- The `ai` package's `Provider` interface, engine order, and the `rules` engine.
- `render.Apply`: apply resolved suggestions to a resume text.
- Revisions and suggestions in the store; the 8 Revisions routes, run in the background.
- Tailored resumes in the store; 5 Tailored resume routes (create, list, get, `diff`, delete).
- The `diff` package (section and word level).
- The `diff-view` view, which the UI calls Tailor.

Out: local and remote models (phase 8), export (phase 6), linking a tailored resume to an application (phase 7).

## Schema change (recommended)

Add to `suggestions` in `001_init.sql`, with [schema.md](schema.md) and the Suggestion shape in [api.md](api.md):

```sql
engine TEXT NOT NULL DEFAULT 'rules' CHECK (engine IN ('local','remote','rules')),
```

In phase 8 a round can mix model suggestions with rules suggestions that fill items the model skipped (build-findings section 1, decision 5). The column lets the review label each one. It is cheap now and needs a migration after D3.

## Routes

All as written in [api.md](api.md). Notes:

```jsonc
// POST /v1/revisions   { "job_id": 1 }
// 202
{ "id": 4, "job_id": 1, "base_resume_id": null, "engine": "rules", "status": "queued", "error": null,
  "created_at": "2026-10-24T15:00:00Z", "completed_at": null }

// GET /v1/revisions/4   (after it finishes)
// 200
{ "id": 4, "status": "done", "engine": "rules", ...,
  "suggestions": [
    { "id": 11, "revision_id": 4, "section": "skills", "target_id": null, "engine": "rules",
      "original_text": "excel, python, sql", "proposed_text": "sql, excel, python",
      "state": "pending", "edited_text": null }] }

// POST /v1/tailored-resumes   { "revision_id": 4 }   (2 suggestions still pending)
// 409
{ "error": { "code": "conflict", "message": "2 suggestions still need a decision", "details": { "pending": 2 } } }
```

- `engine` in the request: `auto` (default) or `rules` in this phase. `local` and `remote` return `503 ai_unavailable` until phase 8.
- `suggestions` is always present in `GET /v1/revisions/{id}`, `[]` when empty (the full build found an omitted field broke the UI).
- A round with no suggestions finishes `done` with `[]`. The UI says the resume already fits as well as rules can tell.
- Accept, reject and edit return `409` unless the revision is `done`.
- `base_content` is the base resume text when the tailored resume is built: the saved resume's `content` if `base_resume_id` is set, else `render.Master` of the master at that moment.

| Status | `code` | When |
|--------|--------|------|
| 404 | `not_found` | Unknown revision, suggestion, job or tailored resume |
| 409 | `conflict` | No profile; cancel when not `queued` or `running`; deciding a suggestion before `done`; building with pending suggestions or a revision not `done` |
| 422 | `validation_failed` | Empty `edited_text`, unknown `engine` |
| 503 | `ai_unavailable` | A forced engine that cannot run |

## How a round runs

1. `POST /v1/revisions` inserts a `queued` revision and returns `202`.
2. A background goroutine marks it `running`, loads the job, the master and the analysis, builds the targets, and asks the engines in order.
3. Each draft becomes a `pending` suggestion. The revision becomes `done` with `completed_at`, or `failed` with `error`.
4. The UI polls `GET /v1/revisions/{id}` every second until `done`, `failed` or `canceled`.
5. Cancel calls the round's `context.CancelFunc`; the goroutine stops and marks it `canceled`.
6. At startup, rounds left `queued` or `running` (the app quit mid-round) are marked `failed` with "interrupted".

**Targets** are the pieces of the master a suggestion may replace: the summary (empty if none, so the proposal is new text), each experience's `description` and `impact`, and the skills line. Each target's original text appears verbatim in `render.Master`'s output, so `render.Apply` can find it.

**Rules engine.** Rules never invent content:

- **Skills line:** reorder so the job's matched skills come first. Same items, none added or removed.
- **Experience text:** reorder lines so lines that show matched skills come first. No line is added, removed or reworded. Single-line text is skipped.
- **Summary:** if a summary exists, append "Relevant strengths for this role: <top 5 matched skills>."; if none exists, propose "<target position or job title> with hands-on experience in <top 5 matched skills>." Skipped when nothing matched.

## Files, in build order

Progress:

- [ ] 1. Schema: `suggestions.engine`
- [ ] 2. `models.go` Revision, Suggestion, TailoredResume
- [ ] 3. `render.go` Apply
- [ ] 4. `provider.go`
- [ ] 5. `fallback.go`
- [ ] 6. `engine.go`
- [ ] 7. `revision.go`, `suggestion.go` (store)
- [ ] 8. `tailored_resume.go` (store)
- [ ] 9. `diff.go`
- [ ] 10. `revisions.go` (api)
- [ ] 11. `tailored.go` (api)
- [ ] 12. `main.go`: interrupted rounds
- [ ] 13. `ipc-handlers.js`, `preload.js`
- [ ] 14. `worddiff.js`
- [ ] 15. `diff-view.js`
- [ ] Tests and tooling

### Backend

#### 1. `backend/internal/db/migrations/001_init.sql`

Add `suggestions.engine` as above; update `schema.md` and `api.md`.

#### 2. `backend/internal/models/models.go`

- `Revision`, `Suggestion`, `TailoredResume`, with JSON tags matching api.md. `Revision` has `Suggestions []Suggestion` with `json:"suggestions"` (no `omitempty`) for the detail route.
- `Suggestion.Final() string`: `edited_text` if edited, `proposed_text` if accepted, `original_text` if rejected.

#### 3. `backend/internal/render/render.go` (Apply)

**Goal:** build tailored text from a base and the user's decisions.

- `type Change struct { Original, Final string }`.
- `Apply(base string, changes []Change) string`: for each change with an `Original`, replace its first occurrence; a change with an empty `Original` (a new summary) is inserted as `## Summary` before the first `## ` heading.
- Return the list of changes whose `Original` was not found, so the API can report them. This happens when the base is a saved resume the user edited by hand: suggestions come from the master, and their text may not be in it.

#### 4. `backend/internal/ai/provider.go`

**Goal:** the interface every engine implements.

```go
type Target struct {
    Section  string // summary, experience, skills
    TargetID *int64 // experiences.id for experience targets
    Label    string // "Data Analyst at Acme (responsibilities)", used in prompts in phase 8
    Original string
}

type Request struct {
    Job      models.Job
    Analysis analyze.Result
    Profile  *models.Profile
    Targets  []Target
}

type Draft struct {
    Section  string
    TargetID *int64
    Original string
    Proposed string
    Engine   string
}

type Provider interface {
    Name() string // local, remote, rules
    Ready() bool  // can run now, without network or model checks that take long
    Suggest(ctx context.Context, r Request) ([]Draft, error)
}
```

- `BuildTargets(models.Master) []Target` lists the targets described above.
- `ErrUnavailable` for an engine that cannot run.

#### 5. `backend/internal/ai/fallback.go`

**Goal:** the `rules` engine, always available.

- Implements the three rules above. Pure functions over the request; no I/O.
- Never proposes a change equal to the original.

#### 6. `backend/internal/ai/engine.go`

**Goal:** choose and run engines in order, so callers never pick an engine themselves.

- `type Engines struct { Local, Remote, Rules Provider }`. In this phase `Local` and `Remote` are nil.
- `Order(mode string) []string`: `auto` is local, remote, rules (skipping engines not `Ready`); `local`, `remote` and `rules` force one engine.
- `Run(ctx, mode, req) ([]Draft, string, error)`: try each engine in order; on error, log to stderr and try the next. Return the drafts and the engine that produced them. Then fill gaps: targets the model skipped get the rules draft, labeled `rules` (phase 8 relies on this; test it now with a fake provider).

#### 7. `backend/internal/store/revision.go` and `suggestion.go`

- Revisions: `CreateRevision`, `GetRevision` (with suggestions), `ListRevisions(jobID)`, `SetRevisionStatus` (with `error` and `completed_at`), `DeleteRevision`, `FailInterrupted()`.
- Suggestions: `InsertSuggestions` (one transaction), `DecideSuggestion(id, state, editedText)`, `CountPending(revisionID)`.

#### 8. `backend/internal/store/tailored_resume.go`

- `CreateTailored`, `GetTailored`, `ListTailored(jobID)` (without `content` and `base_content`), `DeleteTailored`.

#### 9. `backend/internal/diff/diff.go`

**Goal:** the data for a side-by-side view of base and tailored text.

- `Words(before, after string) []Word`: word-level diff by longest common subsequence over tokens (words and whitespace kept as tokens, so the text rebuilds exactly). `Word{Op: "eq"|"add"|"del", Text}`.
- `Hunks(base, tailored string) []Hunk`: split both texts into blocks at `## ` and `### ` headings, pair blocks by heading, and give each pair a status (`unchanged`, `changed`, `added`, `removed`) and its words. `section` is the `## ` heading in lower case; `target_id` is filled when a block matches an experience target.
- LCS is O(n·m); a resume block is a few hundred words, which is fine. Diff per block, never the whole document at once.

References: [Longest common subsequence (Wikipedia)](https://en.wikipedia.org/wiki/Longest_common_subsequence).

#### 10. `backend/internal/api/revisions.go`

**Goal:** the 8 Revisions routes and the background runner.

- A `runner` on the server holds a map from revision id to `CancelFunc`, with a mutex. Start each round in a goroutine with a context derived from the server's base context (not the request's), so it outlives the request and stops at shutdown.
- Limit to one running round at a time; a second waits as `queued`. Rules are fast, but phase 8's local model is not.
- Recover panics inside the goroutine and mark the round `failed`.

#### 11. `backend/internal/api/tailored.go`

**Goal:** build, read, diff and delete tailored resumes.

- Create: `409` with `details.pending` if any suggestion is pending; build `base_content`; turn each suggestion into a `render.Change` with its `Final()`; `render.Apply`; store. Changes not found in the base are returned in the response as `warnings` (add the field to api.md's TailoredResume create response).
- `diff`: `diff.Hunks(base_content, content)`.

#### 12. `backend/cmd/fistbump-core/main.go`

- Call `store.FailInterrupted()` after opening the database, before serving.

### Electron

#### 13. `app/main/ipc-handlers.js` and `app/preload/preload.js`

- Channels: `revisions:create`, `revisions:list`, `revisions:get`, `revisions:cancel`, `revisions:delete`, `suggestions:accept`, `suggestions:reject`, `suggestions:edit`, `tailored:create`, `tailored:list`, `tailored:get`, `tailored:diff`, `tailored:delete`.
- Preload: `api.revisions.*`, `api.suggestions.*`, `api.tailored.*`.

### Renderer

#### 14. `app/renderer/js/worddiff.js`

**Goal:** word-level highlights for one suggestion, computed in the page.

- The same LCS as `diff.Words`, on `original_text` and the proposed or edited text. Suggestions are short, so this avoids a request per card. Returns `{ op, text }` items for the view to render with `textContent` in `<ins>` and `<del>` elements.

#### 15. `app/renderer/views/diff-view/diff-view.js`

**Goal:** the Tailor screen.

- Route `#/tailor`, and a "Tailor resume" button on a saved job that opens it for that job.
- Start: choose the base (master, or a saved resume version), press Generate. Show progress while polling; Cancel calls cancel.
- Review: one card per suggestion with the section label, the original and proposed text side by side, word highlights, and Accept, Reject and Edit. Edit opens a text area with the proposed text; Save sends `edited_text`. A counter shows decisions left; "Accept all remaining" is a shortcut.
- Build: enabled when no suggestion is pending. After building, show the tailored resume side by side with its base using the `diff` route: each hunk in two columns, changed words highlighted, unchanged hunks collapsed.
- Show any `warnings` from create above the diff.
- List earlier rounds and tailored resumes for the job, with delete.

Styles: `styles/components.css` for cards, the two-column diff, and highlight colors that keep 4.5:1 contrast in both themes.

### Tests and tooling

- `fallback_test.go`: skills reorder keeps the same items; experience reorder adds and removes no lines; summary appended or created; nothing proposed when nothing matched; no proposal equals its original.
- `engine_test.go` with fake providers: order per mode, fall through on error, gap fill labels rules drafts, a forced engine that is not ready returns `ErrUnavailable`.
- `render_test.go`: `Apply` replaces first occurrences, inserts a summary before the first section, reports changes not found.
- `diff_test.go`: words rebuild both texts exactly; hunks pair by heading; added and removed blocks.
- `revision_test.go`, `suggestion_test.go`, `tailored_resume_test.go` (store).
- `api_test.go`: a full round with rules (create, poll to `done`, accept all, build, diff); `409` paths; cancel; interrupted rounds marked `failed` at startup.

## Done when

- With no model and no network, a saved job goes through Generate, review, Build, and a side-by-side diff.
- The diff matches the decisions: accepted text appears, rejected text does not, edited text appears as edited.
- Building is blocked while any suggestion is pending.
- Quitting mid-round leaves the round `failed`, not stuck.
- `go test ./...` and `npm test` pass.

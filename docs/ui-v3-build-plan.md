# UI v3 Build Plan: "Paper on Night"

Status: proposal, 2026-11-25. Supersedes the structure in `docs/ui-overhaul-plan.md` (sidebar and drawers), which predates the approved direction.

Approved mockups (open in a browser):
- Home: `docs/mockups/home-v3.html`
- Job: `docs/mockups/job-v3.html`
- Tailor: `docs/mockups/tailor-v3.html`

Exploration history and rejected directions: `docs/ui-design-explorations.md`.

---

## 1. What we are building

**Structure: the job is the unit.** Every job moves through one path: Saved, Tailored, Applied, Interview, Offer. Today the app splits that path across seven places (Find, Open a link, Paste, Saved, Resume, Tailor, Tracker). v3 has three:

| Destination | What it holds | Replaces |
|---|---|---|
| **Home** | What needs you, the next 7 days, pipeline, Pico's tip, new matches | Home |
| **Jobs** | The list of all jobs (search, filter, stage). Opening one goes to the **Job page**, and its Tailor review opens from there | Find jobs, Saved jobs, Tracker, Tailor |
| **Resume** | Master resume and saved versions | Master resume |

Adding a job happens in the **omnibox** in the bottom bar: type to search boards, paste a URL to import, or paste or drop posting text to parse. This replaces Find, Open a link and Paste as separate screens. Settings sits behind the gear.

**Look: paper on night.**
- **Surfaces:** A navy canvas with a faint dot grid. Content sits on cream paper panels with soft offset shadows.
- **Ink:** Gold hand-drawn marks (underlines, the circle around today, checks).
- **Type:** Bricolage Grotesque for display; system UI for body text.
- **Color:** Pico's own palette (navy, white, gold), plus coral for urgency and mint for success.
- **Pico:** One Pico per screen. Pico speaks in a gold bubble, and the seasonal art changes each month.

**Interaction rules.**
- One filled primary button per screen. Its label follows the job's stage: "Tailor," then "Review 2 changes," then "Export PDF," then "Mark applied."
- Inline actions on calendar items: Draft, Prep, Tailor.
- Every AI or rules change is reviewed in place, on the resume, with Accept, Reject, Edit and Undo.
- Fit is one number everywhere. Where a projection exists, it shows as now and after ("76% → 84%").

---

## 2. Decisions needed before coding

1. **Branch.** I recommend a fresh `feat/ui-v3` from `main`. `feat/ui-overhaul-poc` carries five passes of CSS (`revamp.css`, `finish.css`, `revamp-chrome.js`) built for directions that were rejected. Porting from it costs more than starting clean. Anything useful, such as the bottom-bar markup and the spring indicator, can be cherry-picked.
2. **Themes.** The look depends on two planes, canvas and paper. Options:
   - (a) Map all 23 themes to canvas and paper tokens in the first release.
   - (b) Ship Classic plus High Contrast first, with the other themes falling back to a generated mapping and hand-tuned later.

   I recommend (b).
3. **Screens not yet mocked.** The Jobs list, Resume and Settings need mockups in this style before their phases begin.

---

## 3. Data: what exists and what is missing

What the mockups need, checked against the current API (`app/renderer/js/tauri-bridge.js`, `docs/api.md`, `docs/schema.md`):

| Mockup element | Source today | Gap |
|---|---|---|
| Up next / needs you | `pipeline()` in `views/home/home.js` (due steps, pending suggestions, untailored jobs) | None |
| Next 7 days: follow-ups | `applications.next_step_date` | **No label or time.** "Interview · 2:00 pm" needs `next_step_label` and `next_step_time` columns (migration plus `applications:update`) |
| Next 7 days: posting closes | `jobs.close_date` | None (often null for pasted jobs) |
| Holidays (Thanksgiving) | None | Small bundled US holiday table in the renderer; no backend needed |
| Pipeline tiles by stage | `applications.list`, `jobs.list`, `revisions.list` | "Tailored" is derived (a job with a finished revision and no `Applied` status). Fine in the renderer, but N+1 `revisions.list` calls. **Add `jobs:list` option `with_stage`** to return it in one query |
| Applications this month, reached interview | `applications:stats` | Check it returns monthly counts; extend if not |
| Pico tip: "Power BI in 4 of 7 jobs" | `jobs:analyze` per job (deterministic, about 14 ms each) | **Add `jobs:gapSummary`**: missing required tech skills counted across active jobs. A client-side loop works as a stopgap |
| New for you (saved search) | `findMatches()` in `home.js`, `savedSearches` new-posting highlight | None |
| Missing-skill tag on a match | `suggest` returns `fit` per posting | Confirm `fit` includes the missing list; expose it if not |
| Job: requirement checklist with evidence | `jobs:analyze` gives matched skills with `experience_ids`, plus missing and unmentioned | Skill-level only. The mockup's sentence-level rows ("Present findings monthly", "close match") need a **requirement matcher** over `jobs.requirements`. v1 shows skills with evidence; sentence matching is a later phase |
| Job: pay vs typical, cost of living | `cost:analyze`, `cost:compare` | None |
| Job: notes | `applications.notes` (one text field) | The mockup shows dated notes. v1 keeps one text field; dated entries are optional later |
| Fit "84% after changes" | None | **Add a projected score**: run the analyze matcher against the draft with accepted and pending suggestions applied (`revisions:projectedScore`, or return it with `revisions.get`) |
| Per-change "+5%" | None | Same matcher, per suggestion. Optional; hide it if too slow |
| Tailor: resume rendered with inline changes | `revisions.get` (suggestions with section and before/after), `diff-view.js`, `worddiff.js` | None for data. New rendering |

Backend work is small and deterministic. No AI calls are added.

---

## 4. Phases

Each phase ships behind the existing UI until it is complete, keeps all 23 themes rendering (even if not fully tuned), and passes `e2e/smoke.spec.js`.

### Phase 0: Foundations
- Fresh branch. Write `DESIGN.md` from the mockups: tokens, type scale, paper and ink components, motion rules, the Pico rule.
- Tokens in `styles/themes.css`: `--canvas`, `--canvas-2`, `--paper`, `--paper-2`, `--ink`, `--ink-2`, `--accent` (gold), `--urgent` (coral), `--good` (mint), `--line`. Map Classic and High Contrast by hand, and generate the rest.
- Shared primitives in `styles/base.css`: `.paper`, `.btn-primary` (one per screen), `.chip`, a hand-drawn ink SVG set (underline, circle, check, half, open), and focus, selection and scrollbar styling.
- Remove dead CSS (`.paste-plane` typo, sidebar tree rules) as the views move over.

### Phase 1: Shell and routing
- Bottom bar in `index.html`: logo, Home, Jobs, Resume, the omnibox (center), and the gear. Remove the sidebar and top toolbar.
- Omnibox: detect URL, then search text, then pasted posting text, routing to the existing `job-parser.js` paths. `⌘K` focuses it; `command-palette.js` folds into it.
- Router in `app.js`: `home`, `jobs`, `job/:id`, `job/:id/tailor`, `resume`, `settings`. Back links replace sidebar context. Keep `⌘1–3` shortcuts.
- Keep every keyboard shortcut and accessibility preference (Reduce Motion, Text Size, Underline Links, High Contrast).

**Phase 1 as built:**
- **Window strip:** The sidebar is gone. A 44px window strip replaces it as a drag region; it holds the macOS window buttons and the Windows caption buttons.
- **Jobs sections:** On the Jobs, Tracker and Tailor views, a Saved · Find · Paste | Tracker · Tailor control sits centered in that strip. Tracker and Tailor stay there until Phases 3 and 5 fold them into the Job page and the Jobs list.
- **Jobs button:** It returns to the last place you were under it, starting at Saved.
- **Settings:** It is still a tab of the Jobs view, opened from the gear.
- **Shortcuts:** ⌘1–5 are Home, Jobs, Resume, Tracker and Tailor; ⌘K focuses the omnibox.
- **Removed:** `js/sidebar.js`. Recent searches and saved versions remain on Home and in Find's History.

### Phase 2: Home (`views/home/home.js`)
- Rebuild render around five blocks: greeting with status line, Up next card, seasonal Pico with tip bubble, Next 7 days, pipeline tiles, New for you.
- Data: reuse `pipeline()`, `findMatches()`, `resumeGaps()`; add the calendar builder and the gap summary.
- Empty states use existing art: `33-empty-tracker.png` (no jobs), `08-no-results.png` (no matches), `10-cant-connect.png` (search offline).
- Backend: `next_step_label`/`next_step_time` migration, `jobs:gapSummary`, `with_stage`.

**Phase 2 as built:** renderer only, no backend changes.
- **Next steps:** Labels come from the application's stage. Applied is "Follow up," Interviewing is "Interview," Offered is "Reply to the offer."
- **Pipeline stages:** Computed in the renderer, capped at 60 jobs.
- **Pico's tip:** Runs `jobs:analyze` over up to 25 active jobs and names the most common missing required skill, if at least two jobs share it. "Not now" dismisses that skill on this computer.
- **Holidays:** A bundled US list.
- **New users:** They see how it works in three steps, and Pico greets them with this month's art.
- **Saved versions:** They left Home and live in the Resume view. Recent searches show in the matches panel when the profile has no target roles.
- **Deferred:** The `next_step_label`/`next_step_time` columns, `with_stage` and `jobs:gapSummary` wait until the Jobs list (Phase 5) needs them or Home gets slow.
- **Still to do:** Translations for the new Home strings are deferred to Phase 6.

### Phase 3: Job page (new `views/job/job.js`)
- Pull the detail pane out of `job-parser.js` and the per-job row out of `tracker.js` into one page: header and facts, decision card, stage path, requirements, pay and place, notes.
- The stage path replaces the tracker's per-row stage select. "Change stage" opens a small menu, and "More actions" holds archive, trash, reload and open listing.
- The primary button follows the stage, using existing calls: `revisions` create (Tailor), open review, export (`tailored`), `applications.update` (Mark applied).

**Phase 3 as built:**
- **New view:** `views/job/job.js` at `#/job/<id>`. It has:
  - title and facts;
  - a decision card with fit (required technical skills on the resume) and one stage-driven primary button, plus More actions: open tailored resume, open in tracker, open the listing, archive or unarchive, trash, delete;
  - a stage path that marks a skipped Tailored stop honestly, with a stage select;
  - requirements with resume evidence;
  - pay and place;
  - autosaving notes (the first note starts tracking);
  - the posting, which can be expanded.
- **Entry points:** Saved jobs, "View saved job" in search results, a newly saved pasted job, Home tiles and rows, and the Tracker's company and title links all open the Job page.
- **Saved tab:** It becomes a full-width list.
- **Still in the old views:** Search results keep the old detail pane for unsaved postings. Reloading a posting from its board stays in Find.
- **Deferred:** The projected "after" fit needs a matcher over the tailored draft, so it waits for Phase 4.

### Phase 4: Tailor (`views/diff-view/diff-view.js`)
- Render the tailored resume as a paper sheet with suggestions inline (highlight for insert, ink strike for delete), using `worddiff.js`.
- Margin notes: one per suggestion, gold when active, with the reason, Accept/Reject/Edit, and Undo after a decision. Keep J/K/A/R/E.
- Fit meter in the header (projected score). Export is disabled with a reason until every change is decided, then Pico's finish line appears.
- Keep the engine badge ("Built-in rules · no AI" or the model name) always visible.
- Replace native `confirm()`/`alert()` here with inline confirmations.

**Phase 4 as built:**
- **Proofing desk:** The review stage now shows the master resume on a paper sheet. Each suggestion sits where it applies: added words highlighted, removed words struck, and after a decision the result with a faint underline.
- **Margin notes:** Each change gets a note in the sheet's right margin, on the same row, with its reason, the skills it brings in, Accept, Reject and Edit. Decided notes fold to a line. J/K/A/R/E still work, and focus moves on immediately so quick key presses land on the next change.
- **Bar over the desk:** It shows where suggestions are written (rules, local or remote), the count decided, and a fit meter. The meter uses the same analysis score as the side panel, plus any required skill an accepted change brings in, and shows "if you accept the rest" only when that is higher. "Build tailored resume" stays disabled, with the reason written, until every change is decided; then Pico gives the finish line.
- **Unchanged:** Rounds, cover letter and Q&A, the keyword bridge, editing by hand, the tailored resume view (changes, editor, PDF) and the side panel are as they were. Their v3 restyle moves to Phase 5 with the rest of the older surfaces.
- **Placement limits:** Suggestions are placed by section and experience id, then by matching their original text to a line. Anything unmatched still appears in its section.

### Phase 5: Jobs list, Resume, Settings (after mockups)
- **Jobs:** one list for all jobs, grouped "Needs you" then the rest, with filters (stage, remote, fit, has pay) and an optional board view. This absorbs the Tracker table, bulk actions, and Saved / Archived / Trash scopes.
- **Resume:** master resume as a paper document with inline editing, plus saved versions.
- **Settings:** the same paper panels, a visual theme swatch gallery, AI engine and privacy at the top.

### Phase 6: Hardening
- Tune the remaining themes, run a contrast audit (`impeccable detect` plus a manual check), check narrow windows (1024px and below), and confirm Reduce Motion.
- Update `e2e/` fixtures and README screenshots. Update `docs/user-guide.md`.

---

## 5. Risks

- **23 themes:** Paper and ink on every theme is the largest unknown. Plan option (b) contains it.
- **Feature parity:** Tracker bulk actions, history and filters must land in the Jobs list (Phase 5). Until then, keep the old Tracker reachable from Jobs.
- **Performance:** Home now makes more calls. Add `with_stage` and `gapSummary` before Home ships, and keep the 30-minute match cache.
- **Projected fit:** If the matcher is slow on long resumes, show "after" only once suggestions finish, never blocking the review.

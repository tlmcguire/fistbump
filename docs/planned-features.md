# Planned Features Roadmap

This document plans the next wave of Fistbump improvements, aligned with [PRODUCT.md](../PRODUCT.md): **local-first by default**, **human-approved AI edits**, **fast deterministic analysis**, and **zero cloud lock-in**.

## Phase A — Job Discovery & Multi-ATS Connectors

### 1. Lever & Ashby Connectors
- **Goal**: Expand job search and unified paste-a-link support beyond Greenhouse to cover Lever and Ashby public job boards.
- **Backend**:
  - Implement `backend/internal/connectors/lever/` and `backend/internal/connectors/ashby/` conforming to `connectors.Connector`, reusing SQLite caching and rate-limit backoff.
  - Extend the link resolver to auto-detect Greenhouse, Lever, and Ashby job URLs.
  - Add curated Lever and Ashby board lists alongside Greenhouse in `boards.json`.
- **Frontend**:
  - Update `looksLikeJobLink()` in `job-parser.js` to recognize Lever and Ashby job URLs in the unified search bar.
  - Display subtle source badges (`Greenhouse`, `Lever`, `Ashby`) on search results.
- **Tests**: Unit tests for Lever/Ashby response parsing, pay extraction, link resolution, and mocked search fan-out.

### 2. Saved Searches & "New Since Last Check" Badges
- **Goal**: Let users pin recurring search filter sets and spot newly posted roles since their last run.
- **Database**: Add `saved_searches` table (`id`, `name`, `query_json`, `last_run_at`, `seen_keys_json`, `created_at`).
- **API**: `GET/POST/DELETE /v1/saved-searches` and `POST /v1/saved-searches/{id}/run`.
- **Frontend**: Pin button on the search toolbar, sidebar list under **Jobs**, and a `New` pill on postings not present in the previous run.

### 3. Company Watchlists
- **Goal**: Let users add any supported ATS board by company URL or board token, even if it is not in the curated directory.
- **Backend**: Persist custom boards in `settings` (`connectors.custom_boards`) and include them in search queries and autocomplete.
- **Frontend**: Simple "Add company board" control in Settings / Search filters with validation.

## Phase B — Resume Builder, Tailoring & Export

### 4. Consecutive Resume Imports (Back-to-Back Without Leaving the Page)
- **Goal**: Let users import or paste multiple resumes back-to-back on the Master Resume page without having to navigate away and return.
- **Frontend**:
  - Track active in-flight imports with a dedicated `importing` flag in `app/renderer/views/resume-builder/resume-builder.js` instead of disabling `Import file`, `Paste text`, and drag-and-drop whenever `importSection` is open.
  - Ensure starting a new file import, paste, or drag-and-drop cleanly replaces any open import review or paste form and resets scroll position.
- **Tests**: Add unit and E2E tests covering consecutive file/paste imports on the Master Resume view.

### 5. Custom Resume Sections & Section Ordering
- **Goal**: Support optional sections (Projects, Certifications, Publications, Volunteer Work, Awards) and user-defined section order.
- **Database**: Add `resume_sections` (`id`, `profile_id`, `kind`, `title`, `sort_order`, `visible`) and `resume_section_items` tables.
- **Backend**: Update `render.Master` and `pdf.RenderStyled` to honor section order and render custom sections across all 5 PDF styles.
- **Frontend**: Up/down section reordering controls in Master Resume and custom section editor cards.

### 6. Live 1-Page PDF Fit Check & Deterministic ATS Audit
- **Goal**: Give immediate, deterministic feedback on resume length and formatting without calling an LLM.
- **Backend**: Add a metrics endpoint returning exact `page_count`, `last_page_fill_pct`, and deterministic warnings (missing contact info, bullets lacking metrics, overlong summary, or lines spilling onto page 2).
- **Frontend**: Compact status badge beside the PDF preview style selector (e.g., `1 page · 88% full` or `2 pages · 4 lines over`).

### 7. DOCX Resume Export
- **Goal**: Export Master and Tailored resumes as clean, ATS-safe single-column `.docx` files alongside PDF and Markdown.
- **Backend**: Pure-Go `.docx` generator (`backend/internal/docx/`) using `archive/zip` and OOXML templates matching the selected style (Classic, Modern, Minimal, Compact, Executive).
- **Tauri & Frontend**: Add `Export DOCX` option to Resume Builder and Tailor export controls.

### 8. Targeted Cover Letter & Application Q&A Drafts
- **Goal**: Reuse Fistbump's local-first AI + human approval workflow for short application prompts ("Why this role?", "Describe a relevant project") and tailored cover letters.
- **Backend**: Add `application_drafts` table linked to `job_id` and `tailored_resume_id`, reusing `ai.Engines` with strict resume-grounding prompts and guardrails.
- **Frontend**: New **Answers & Cover Letter** tab inside Tailor with editable cards, word count, and one-click copy.

## Phase C — Application Tracker & Interview Prep

### 9. CSV Export & Full Local Backup / Restore
- **Goal**: Ensure zero lock-in by letting users export their tracker to CSV and create or restore full local ZIP backups of their SQLite database.
- **Backend / Tauri**: Add `GET /v1/applications/export?format=csv` (RFC 4180 CSV) plus Tauri backup export/restore commands.
- **Frontend**: **Backup & Export** section in Settings and **Export CSV** button in Tracker.

### 10. Interview Prep Sheet per Tracked Application
- **Goal**: Consolidate everything needed for an interview into one local printable view.
- **Frontend & Backend**: For any tracked job (especially `Interviewing`), display the saved posting, the exact tailored resume version submitted, matched/missing skills, company notes, and optional local-AI practice questions targeting skill gaps.

### 11. Follow-Up Reminders & `.ics` Calendar Export
- **Goal**: Let users export `next_step_date` deadlines or interviews as standard `.ics` files for Apple Calendar, Google Calendar, or Outlook without OAuth or cloud sync.
- **Backend / Frontend**: Generate RFC 5545 `.ics` events directly from any Tracker row with a `next_step_date`.

### 12. Application Funnel & Skill-Gap Trends
- **Goal**: Turn local application history into actionable signal.
- **Backend**: `GET /v1/applications/stats` returning stage conversion counts (`Saved -> Applied -> Interviewing -> Offered`) and the top recurring missing skills across saved jobs.
- **Frontend**: Compact local analytics card on Home / Tracker.

## Phase D — Compensation & Offer Comparison

### 13. Side-by-Side Offer Comparison
- **Goal**: Compare 2–3 saved or offered jobs side by side using `backend/internal/cost`.
- **Backend**: Extend `cost.Input` with optional overrides (`FilingStatus`, `CustomRent`, `Bonus`, `AnnualEquity`, `PreTax401kPct`) and add `POST /v1/cost/compare`.
- **Frontend**: Comparison table view accessible from Saved Jobs and Tracker showing gross pay, bonus/equity, effective tax rate, rent, monthly budget, and net savings side by side with cited sources.

## Suggested Implementation Order
1. **Phase A (Lever & Ashby + Watchlists)** — highest immediate impact on job discovery.
2. **Phase B (1-Page PDF Fit Check + Custom Sections + DOCX Export)** — completes the resume authoring loop.
3. **Phase C (CSV/ZIP Backup + `.ics` Export + Interview Prep)** — strengthens the Tracker and interview workflow.
4. **Phase D (Side-by-Side Offer Comparison + Cover Letter / Q&A Drafts)** — high-value decision and application tools.

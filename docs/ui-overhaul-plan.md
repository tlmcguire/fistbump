# Fistbump UI Overhaul Plan

**Status:** Proposal / Architecture Blueprint  
**Target:** Post-`v0.5.0` visual and UX modernization  
**Scope:** Renderer shell, all five primary views (`Home`, `Jobs`, `Master Resume`, `Tailor`, `Tracker`), `Settings`, accessibility controls, performance guardrails, and full 23-theme migration.

---

## 1. Visual Overhaul & Concept Conflict Resolution

The five visual concepts explore bold improvements to Fistbump's layout and craft, but they intentionally introduced conflicting patterns (different sidebars, mixed modal vs. drawer styles, dark vs. light inputs, and varying top-bar hierarchies). Before writing code, we must resolve those conflicts into **one unified design system**.

### 1.1 Resolved Design Conflicts

| Conflict Across Concepts | Canonical Decision | Why |
| :--- | :--- | :--- |
| **Sidebar:** Slim icon-only rail (Home, Tracker) vs. wider labeled rail (Master Resume) vs. collapsible tree (current app) | **Collapsible Dual-Mode Rail** (`220px` labeled default, collapses to `64px` icon rail via `⌘\` or narrow viewport `< 1040px`). | Preserves discoverability for new users (clear labels + `⌘1–5` shortcuts) while giving power users and smaller laptops maximum canvas space. Keeps `Saved searches`, `Saved versions`, and `Settings` reachable without hiding features. |
| **Jobs sub-navigation:** Separate sidebar tree items (`Find`, `Paste`, `Saved`) vs. top segmented bar (`Discover \| Saved \| Paste`) | **Single `Jobs` entry in the left rail + top sliding segmented bar (`Discover \| Saved \| Paste`).** | Removes sidebar clutter and eliminates the confusing "Jobs view owns Settings" routing quirk. `Settings` becomes a clean top-level destination at the bottom of the rail. |
| **Input & Editor Surfaces:** Dark-integrated editors (Concepts 1–5) vs. high-contrast white inputs (current `base.css`) | **Theme-adaptive semantic field tokens (`--field-surface`, `--field-text`, `--field-border`).** | Dark themes get deep, low-glare tinted surfaces (no blinding white rectangles on navy/Dracula/Ocean Abyss); light themes get crisp light surfaces; High Contrast mode switches to maximum-contrast solid fields with 2px borders. |
| **Secondary Panels:** Centered modal dialogs vs. right-hand slide-over drawers | **Slide-over drawers for workspace editors** (Interview Prep, Offer Comparison, Honest Keyword Bridge); **compact centered dialogs only for destructive confirmations** (`UI.ask`). | Drawers preserve surrounding context (e.g., viewing your Tracker row while editing its Prep Sheet) and dismiss effortlessly via backdrop click, `Esc`, or top-right `×`. |
| **Action Placement:** Top toolbar buttons vs. floating bottom glass dock (Jobs concept) | **Sticky contextual action bar at the top of detail panes** (no floating bottom dock over scrolling text). | Avoids obscuring the bottom of long job descriptions or resume previews, works reliably with keyboard tab order, and avoids WebKit `backdrop-filter` scroll repaint cost. |
| **Pico Mascot & Brand Assets:** Redesigned/stylized logos or mascots in concept mockups vs. Fistbump's existing brand artwork | **Keep the exact existing Fistbump logo and all existing Pico mascot artwork unchanged** (`assets/` & `app/renderer/views/graphics/mascots/`). Keep surfaces solid/opaque wherever Pico appears (`Home` hero, empty states, thinking APNG, Settings cleanup) and reserve subtle `backdrop-filter` strictly for modal/drawer overlays. | Preserves brand continuity and Pico's signature look without redrawing assets or creating boxy seams around Pico's transparent/navy-keyed illustrations across Classic and seasonal themes. |
| **Global Search vs. View Search:** Floating `⌘K` command bar on Home vs. per-view search inputs | **Global `⌘K` Quick Jump palette (modal overlay) + lightweight view-local filter input.** | Lets power users jump to any job, application, resume version, or paste a URL from *any* screen without duplicating search bars in the header. |

### 1.2 Brand Identity & Asset Guardrails
- **Logo (Unchanged):** Retain the existing Fistbump fist-bump logo mark across the sidebar header, macOS/Windows/Linux app icons, DMG background, and release graphics—never replace or redraw it.
- **Pico Mascot (Unchanged):** Retain all existing Pico mascot assets verbatim (`intro/meet-pico.png`, `monthly/01..12.png`, `fun-screen/*`, `animations/thinking-anim.png`, `05-searching-wide.png`, `08-no-results.png`, `10-cant-connect.png`, `14-writing.png`, `17-tailoring-in-progress.png`, `20-pdf-exported.png`, `28-cleaning-up.png`, `29-something-went-wrong.png`, `33-empty-tracker.png`, `34-no-changes.png`). Containers holding Pico must match `--bg` cleanly with no drop-shadow frame around the illustration itself.

### 1.3 Unified Visual Grammar
- **Typography:** Keep self-hosted **Bricolage Grotesque** for display/section headings and native system UI font (`-apple-system` / `Segoe UI` / `Ubuntu`) for dense body copy, tables, and inputs; monospace strictly for Markdown source editing.
- **Elevation Hierarchy (3 Tiers Only):**
  1. **Canvas (`--bg`)** — Base window background.
  2. **Surface (`--panel`)** — Cards, tables, drawers with a 1px `--panel-line` border and subtle `--lit` top-edge highlight.
  3. **Raised (`--raised`)** — Active sliding pill thumbs, popovers, dropdowns, and confirmation dialogs.
- **Unified Controls:** Every mutually exclusive mode switch uses the single sliding-pill segmented component (`.segmented` / `.tabs` driven by `segments.js`).

---

## 2. Feature Parity & Better Visual Communication

The overhaul must preserve **100% of existing Fistbump functionality**—nothing gets dropped or buried. Below is the parity map showing how every existing feature survives and becomes clearer.

### 2.1 Home
| Existing Feature | Overhaul Treatment & Visual Communication |
| :--- | :--- |
| Time-away greeting & Pico monthly/fun poses | Retained in the **Hero Card** on a clean flat surface matching `--bg` so Pico blends seamlessly. |
| "Keep reading" remembered listing | Elevated into the Hero Card's primary CTA ("Continue where you left off") alongside a 1-click Tailor shortcut. |
| "Needs you" (overdue steps, pending suggestions, untailored jobs) | Upgraded to a structured **Focus Timeline Card** with distinct color-coded status dots (coral = overdue, gold = due today, mint = ready to review). |
| "Jobs that fit you" (cached Greenhouse/Lever/Ashby matches + countdown retry) | Horizontal card strip with visual **Fit Score micro-rings**, salary/COL verdict pills, and 1-click `Save` / `Tailor` buttons. Rate-limit countdown stays inline on the refresh pill. |
| "Finish your resume" gaps & completion summary | Compact **Resume Readiness ring** (`94%`) with clickable gap pills that jump directly to the missing field in Master Resume. |
| Saved versions & Recent searches | Docked two-column footer bento cards; clicking runs the search or opens the version in 1 click. |

### 2.2 Jobs (`Discover | Saved | Paste`)
| Existing Feature | Overhaul Treatment & Visual Communication |
| :--- | :--- |
| Search bar + direct job URL detection (Greenhouse, Lever, Ashby) | Unified pill search bar with an inline badge when a pasted URL is recognized (`Link detected — press Enter to import`). |
| Multi-ATS filters, card display toggles, pinned searches | Compact inline quick-filter chips (`Greenhouse`, `Lever`, `Ashby`, `Remote`, `Min Fit`) plus an expandable **All Filters** popover that auto-closes on outside click or `Esc`. |
| Search history & Saved searches | Slide-down popover from the search bar with 1-click re-run and pin/unpin. |
| Cost-of-living, salary, & benefits analysis | Replaced dense text blocks with a **Visual Pay & Living Gauge** (horizontal bar showing Market Verdict + Monthly Take-Home + Leftover) while keeping expandable source citations. |
| Skill Match Breakdown | Clean **Skill Match Matrix**: solid mint pills for matched skills, subtle amber outlined pills for missing skills, with required vs. preferred clearly grouped. |
| Saved jobs scopes (`Active \| Archived \| Trash`) + bulk trash/delete | Preserved as the unified sliding segmented toggle + contextual bulk action bar when rows are checked. |
| Paste a job (preview, hidden-text warning, editable inferred fields, save) | Integrated as the `Paste` segment in Jobs with generous vertical spacing, clear hidden-text alert banner, and 1-click `Save` or `Save & Tailor`. |

### 2.3 Master Resume
| Existing Feature | Overhaul Treatment & Visual Communication |
| :--- | :--- |
| Profile, Summary, Experience, Education, Skills, Custom Sections, Section Order | Left **Section Outline** with drag-reorder handles + click-to-scroll; center **Inline Dark/Theme-Adaptive Cards** (no stark form walls). |
| Multi-format import (PDF, DOCX, ODT, TXT, MD) & Paste import | Compact `Import` split button in the studio header; shows live parse progress and diff preview before applying. |
| 1-Page Fit Audit | Promoted from a hidden button result to a persistent **1-Page Fit Meter** bar at the top of the editor (green = fits cleanly, amber = close, coral = overflows 1 page with 1-click tips). |
| Saved Resume Versions (`Create`, `Show/Hide`, `Load`, `Delete`) | Persistent **Version Pills** at the bottom of the outline rail so switching or comparing versions takes 1 click. |
| Live Text / PDF preview + 5 PDF styles + Export PDF / DOCX | Right-hand **Live Paper Preview** with `Text \| PDF` sliding toggle, bottom style switcher (`Classic \| Modern \| Minimal \| Compact \| Executive`), and 1-click `Export PDF` / `Export DOCX`. |

### 2.4 Tailor
| Existing Feature | Overhaul Treatment & Visual Communication |
| :--- | :--- |
| Job picker, Base resume picker, Engine status chip | Compact top **Studio Context Bar** showing `Company · Role`, base resume selector, and live AI engine badge (`Local AI · Ready` / `Remote AI` / `Built-in Rules`). |
| Suggestion Rounds (`Round 1`, `Your edit 1`) & Review Cards | Unified top segmented workspace: **`Suggestions \| Diff \| Edit \| PDF \| Kit`**. Suggestion cards show **Before → Proposed** side-by-side with inline word-diff highlights and floating `Accept / Edit / Skip` controls. |
| Honest Keyword Bridge + 1-click Undo / Remove note | Collapsible card in the right inspector rail directly under the **Fit Score Ring** (`+14%` delta indicator when suggestions are accepted). |
| Cover Letter, Q&A, Likely Questions & Interview Prep (versioned, auto-saved, Instant vs. AI, `.txt/.md/.doc` + `Export all`) | Dedicated **`Kit`** workspace tab with sliding `Instant \| AI` pill, 1-click `Generate kit`, auto-saved seamless cards, inline editable questions, live AI elapsed timers & stall warnings, and truthful engine badges (`Local AI · 2.4s` vs `Instant`). |
| Track with this resume | Single button in the studio header that updates Tracker and Home live via `fistbump:applications`. |

### 2.5 Tracker
| Existing Feature | Overhaul Treatment & Visual Communication |
| :--- | :--- |
| Pipeline stats & top skill gaps | Upgraded to a proportional **Pipeline Funnel Bar** (clickable stage segments that filter the view) + subtle skill-gap chips. |
| `Active \| Archived \| All` scope toggle & Search | Retained in the top bar alongside a new **`Board \| Table`** sliding view switcher (Table remains default for high-volume keyboard editing; Board gives visual pipeline clarity). |
| Inline status dropdown, dates, `.ics` export, notes, bulk actions, CSV export | Preserved in Table view; Kanban cards expose 1-click `Prep`, `Tailor`, `.ics`, and drag-to-stage. |
| Side-by-side Offer Comparison | Opens as a spacious **Slide-Over Drawer** (or wide sheet) with clear best-in-row highlights. |
| Editable, auto-saved Interview Prep Sheet (synced with Tailor, `Edit \| Preview`, versions, `Generate with AI`, Print, `.md`) | Opens as a **Frosted Slide-Over Drawer** so the user keeps their Tracker list visible while editing prep notes. |

### 2.6 Settings
- Promoted to a dedicated top-level route in the bottom of the left rail (no longer nested inside Jobs).
- Preserves all 8 cards: **Appearance & Accessibility** (with live mini-swatch theme cards instead of a 23-item text `<select>`!), **Search Preferences**, **Company Watchlist**, **History**, **Storage & Cleanup + JSON Backup/Restore**, **AI Suggestions** (with live timed test + stall detection), **Local Models**, **Updates**, and **About**.

---

## 3. Accessibility Plan

Accessibility is a first-class invariant, not a post-hoc pass. Every new component must satisfy these requirements before merging:

1. **Contrast Guarantees Across All 23 Themes:**
   - Body text, inputs, badges, and muted secondary labels must meet **WCAG 2.1 AA** (`>= 4.5:1` normal text, `>= 3:1` large text and UI boundaries).
   - High Contrast mode (`Prefs.highContrast`) must enforce **WCAG AAA** (`>= 7:1` primary text, explicit `1.5px–2px` solid borders on all cards, pills, and inputs, and zero translucent washes).
2. **Full Preservation of User Accessibility Preferences:**
   - **`textSize`** (`small`, `default`, `large`, `xlarge`): All layout heights (`min-height` instead of fixed `height` on buttons/chips/rows) and card paddings must scale with `rem`/`em` so large text never clips or overlaps.
   - **`reduceMotion`**: Disables all slide, rise, spring, and blur transitions; falls back to crisp `120ms` opacity fades or instant state swaps. `window.UI.motion()` gates all JS-driven animations.
   - **`underlineLinks`**: Applies consistently to all link-style buttons, company/posting links, and Tracker row actions across every theme.
   - **`highContrast`**: Overrides translucent surfaces (`color-mix`) with solid high-contrast surface tokens.
3. **Keyboard-First & Screen-Reader Parity:**
   - **Global shortcuts:** `⌘/Ctrl+1..5` for primary views, `⌘/Ctrl+,` for Settings, `⌘/Ctrl+K` for Quick Jump, `/` for view search, `⌘/Ctrl+\` to toggle sidebar collapse, `Esc` to close the topmost drawer/modal/popover.
   - **Roving tabindex (`ArrowLeft` / `ArrowRight`)** on every `.segmented` / `.tabs` control and Kanban column navigation.
   - **Focus management:** Opening any slide-over drawer or dialog traps focus cleanly and restores focus to the triggering button on close.
   - **Live announcements:** AI elapsed timers, auto-save status (`Saved v2`), and stall errors use polite `role="status"` / assertive `role="alert"` regions (throttled so screen readers aren't spammed every second—announce on start, at the 20s stall warning, and on completion/error).

---

## 4. UI/UX & Interaction Design Plan

1. **Click-Budget Discipline:**
   - Target **<= 2 clicks** for every top-5 daily workflow:
     - *Find → Save & Tailor* (1–2 clicks)
     - *Tailor → Generate Kit* (1 click)
     - *Tracker → Edit Prep Sheet* (1 click, auto-saved)
     - *Tracker → Archive / Delete / Jump to Tailor* (1 click inline)
     - *Switch Instant / AI or View Scope* (1 click sliding pill)
2. **Zero-Friction Editing & Auto-Save:**
   - All long-form editors (Tailor drafts, Q&A titles, Tracker prep sheets, Tracker notes) auto-save cleanly with 600ms debounce + flush-on-blur/close. Explicit buttons are reserved for **creating a new version snapshot** (`+ Save v2`) or **exporting**.
3. **Honest AI Transparency:**
   - Never claim AI ran when rules/Instant fallback ran. Every AI action displays:
     - Live elapsed timer (`0 s`, `1 s`, …)
     - Clear slow-model warning at `20–25 s`
     - Hard stall error at `90 s` with actionable next steps (`Cancel`, `Settings → AI`, or `Switch to Instant`)
     - Truthful engine badge on completion (`Local AI · 2.4 s`, `Remote AI · 1.1 s`, or `Instant`).
4. **Effortless Dismissal & Zero Layout Shift:**
   - Every modal, drawer, and filter popover supports **3 dismissal paths**: `Esc`, click-outside backdrop, and a top-right `×` button.
   - `scrollbar-gutter: stable` on all scrollable panes eliminates horizontal jumps when switching tabs or expanding panels.
5. **Internationalization (i18n) Continuity:**
   - All new labels, empty states, and tooltips must be added cleanly to `en`, `es`, `fr`, and `de` locale tables (enforced by existing `app/tests/i18n.test.js`) and never hardcode English-only widths that break German/French strings.

---

## 5. Efficiency & Performance Plan

1. **Zero-Framework Vanilla Discipline:**
   - Keep the renderer dependency-free (no React/Tailwind/bundler overhead) so cold launch stays instantaneous and the Tauri bundle stays small.
   - Modularize shared UI primitives into tiny, reusable helpers in `app/renderer/js/` (`segments.js`, `drawer.js`, `command-palette.js`, `ui.js`).
2. **GPU-Friendly Motion in WebKit/Tauri:**
   - Animate **only** compositor properties (`transform`, `opacity`). Avoid animating `width`/`height`/`box-shadow` on large lists; keep `backdrop-filter` strictly on static modal/drawer overlays (never on scrolling cards).
   - Remove animation classes after completion (`UI.play` / `UI.stagger`) so view switches never replay stale animations.
3. **Data & Rendering Efficiency:**
   - **Lazy PDF rendering:** Only invoke `previewPdf` when the PDF tab/canvas is visible, and debounce live preview refreshes in Master Resume (`800ms`) so typing stays 60fps.
   - **Event-driven cross-view sync:** Standardize on `fistbump:jobs`, `fistbump:applications`, `fistbump:resume`, `fistbump:versions`, `fistbump:drafts`, and `fistbump:storage-cleaned` so every view clears stale state immediately without redundant polling.
   - **Incremental DOM updates:** When toggling a single suggestion, filter chip, or Kanban card, patch only the affected card/row instead of rebuilding the entire view.
4. **Automated Quality Guardrails:**
   - Keep all existing JS (`node --test`), Rust (`cargo test`), and Go (`go test ./...`) suites green at every milestone.
   - Add automated unit assertions for: zero hardcoded hex colors in component CSS (outside `themes.css`), ARIA attributes on all drawers/segmented controls, and i18n coverage.

---

## 6. Theme Migration Plan (All 23 Themes + `auto-season`)

Fistbump currently ships **23 themes** (`classic`, 10 style themes: `bubblegum`, `techno`, `cyber`, `retro`, `vaporwave`, `dracula`, `matcha`, `midnight-gold`, `ocean-abyss`, `arcade`; plus 12 seasonal themes: `january`–`december` and `auto-season`).

### 6.1 The Problem Today
- `finish.css` and `views.css` still contain scattered hardcoded colors (e.g., `rgba(255, 255, 255, 0.06)`, `#153678` on overlays, stark white `--field` on dark themes), causing inconsistent contrast between dark themes (`midnight-gold`, `dracula`, `ocean-abyss`, `cyber`) and light themes (`classic` light overrides, `matcha`, `bubblegum`, spring/summer seasonal themes).

### 6.2 Semantic Token Contract (`themes.css`)
Migrate every theme to define a **single, small set of base palette tokens**—and let `base.css` / `finish.css` derive all component surfaces automatically via CSS `color-mix()`:

| Token | Role | Derived / Theme-Owned |
| :--- | :--- | :--- |
| `--bg` | Window canvas | Theme-owned |
| `--sidebar` | Navigation rail background | Theme-owned |
| `--panel` | Primary card & table surface | Theme-owned |
| `--raised` | Modals, drawers, active pill thumb, popovers | Theme-owned |
| `--text` / `--muted` / `--faint` | 3-tier ink hierarchy | Theme-owned |
| `--accent` / `--accent-hover` / `--accent-ink` | Primary CTA & active indicators | Theme-owned |
| `--good` / `--warn` / `--bad` | Semantic status (match, gap, danger) | Theme-owned (with safe defaults) |
| `--field-surface` / `--field-ink` / `--field-border` | Integrated theme-native inputs & textareas | Derived via `color-mix(in srgb, var(--bg) 55%, var(--panel))` for dark themes; crisp white/off-white for light themes (`data-color-scheme="light"`) |
| `--pill-track` | Segmented toggle track background | Derived via `color-mix(in srgb, var(--text) 7%, transparent)` |
| `--panel-line` / `--line-strong` | Hairline & emphasized borders | Derived via `color-mix(in srgb, var(--text) 10% / 18%, transparent)` |

### 6.3 Migration Steps & Verification
1. **Tag every theme with `data-color-scheme="dark" | "light"`** in `prefs.js` so `color-mix` field/shadow/highlight formulas adapt automatically without per-theme CSS overrides.
2. **Audit & purge hardcoded colors** from `base.css`, `views.css`, and `finish.css` (zero raw hex/rgba surface values outside `themes.css`).
3. **Add a Contrast Audit script** (Node + Playwright) that loads all 23 themes, measures computed WCAG contrast ratios for body text, muted text, primary buttons, inputs, and badges, and flags any pair below `4.5:1`.
4. **Refresh visual regression screenshots** (`docs/images/themes/*.png`) using the existing Playwright screenshot pipeline so every theme is visually verified before release.

---

## 7. Phased Implementation Roadmap

Each phase ships independently with full test parity so `main` stays releasable at all times:

1. **Phase 1 — Token Contract & Theme Migration (`themes.css`, `base.css`, `finish.css`)**
   - Introduce `data-color-scheme`, semantic `--field-surface` / `--pill-track` / `--panel-line` tokens, purge hardcoded surface colors, and run the 23-theme contrast audit.
2. **Phase 2 — Navigation Shell, Collapsible Rail & `⌘K` Quick Jump (`index.html`, `app.js`, `sidebar.js`)**
   - Move `Settings` to its own top-level rail item, unify `Jobs` into `Discover | Saved | Paste`, add collapsible rail (`⌘\`), and build the `⌘K` Quick Jump overlay.
3. **Phase 3 — Reusable Slide-Over Drawer & Tracker Pipeline (`drawer.js`, `tracker.js`)**
   - Convert Tracker's **Interview Prep Sheet** and **Offer Comparison** into frosted slide-over drawers, add the proportional **Pipeline Funnel Bar**, and add the `Board | Table` Kanban switcher.
4. **Phase 4 — Tailor Studio & Master Resume Live Studio (`diff-view.js`, `resume-builder.js`)**
   - Unify Tailor into the single top segmented bar (`Suggestions | Diff | Edit | PDF | Kit`), add dark/theme-integrated editors, and upgrade Master Resume with the persistent **1-Page Fit Meter** and floating paper PDF canvas.
5. **Phase 5 — Jobs Discovery Polish, Accessibility & Visual QA (`job-parser.js`, Playwright tests)**
   - Add the visual **Pay & Cost of Living Gauge**, **Skill Match Matrix**, full keyboard/screen-reader sweep, i18n updates (`es`/`fr`/`de`), and updated screenshots + docs.

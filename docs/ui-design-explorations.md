# Fistbump UI Design Explorations & Feedback Log

This document records every UI overhaul direction explored (both implemented proofs of concept on `feat/ui-overhaul-poc` and brainstormed/mocked-up concepts) alongside direct user feedback and the distilled design principles that emerged.

---

## Non-Negotiable Guardrails

- **Keep exact brand & mascot assets**: Preserve the existing Fistbump logo (`app/renderer/views/graphics/logo.png`), the existing `#i-*` icon sprite, and all Pico mascot artwork unchanged.
- **Keep 100% feature parity & accessibility**: All 23 themes, High Contrast, Reduce Motion, Text Size, Underline Links, keyboard shortcuts, and all frontend/backend features remain intact.
- **Branch safety**: All experimental redesign work lives on `feat/ui-overhaul-poc` so `main` stays untouched.

---

## Distilled Design Principles (From User Feedback)

### What to Aim For
- **Open, indie, and welcoming**: warm, handcrafted desktop feel with real personality rather than sterile enterprise software or generic SaaS web-app vibes.
- **Practical, well thought out, and frictionless**: every square inch should earn its keep and serve the workflow cleanly without decorative empty margins.
- **Genuinely fresh and structurally different**: not just the existing layout reorganized or reskinned with minor CSS tweaks; rethink how each view is structured and interacted with.
- **Grounded, non-floating chrome**: navigation and toolbars should sit solidly in place rather than floating over scrolled content.
- **Conventional & immediately intuitive**: single-window views rather than multi-window OS emulation, while still feeling unmistakably native macOS.
- **Fine-motor friendly accessibility**: generous hit targets, zero drag-required interactions, clear visible controls (no hover-only traps), and full keyboard support.
- **Fluid, purposeful animations**: physical, refined motion everywhere (respecting Reduce Motion).

### What to Avoid
- **No left sidebar** (feels too similar to the existing app and confines horizontal space).
- **No heavy top navigation bar** (user wasn't sold on top-bar navigation).
- **No constricting narrow column setups** (rigid multi-column corridors limit possibilities and feel claustrophobic).
- **No floating elements over content** (floating docks/HUD pills feel like they are always in the way).
- **No Netflix-style horizontal scrolling carousels/ribbons** (strongly disliked).
- **No wasteful minimalist empty margins** (concepts that leave half the screen blank don't use space efficiently).
- **No AI-generated design tropes** (no curved/inset left-border accent bars on rounded items, no neon outer glows, no gradient washes on flat nav pills, no gratuitous hover-lift bounces).

---

## Part 1: Implemented Proofs of Concept (`feat/ui-overhaul-poc`)

### Pass 1: Initial Overhaul PoC (`45501d6`)
- **What was tried**: Theme-adaptive dark-native input fields, collapsible left icon rail (`⌘\`), global `⌘K` Quick Jump Command Palette, Tracker visual Pipeline Funnel bar, `Table | Board` Kanban view, and frosted Slide-Over Drawers.
- **User feedback**:
  > *"It’s not a proper revamp. It’s basically the current UI plus some quirks. But I wanted a full revamp."*

### Pass 2: Recessed Dock, Floating Canvas & Bento Home (`310d374` → `22f4b55`)
- **What was tried**: Recessed matte left dock + floating rounded `<main>` workspace canvas, Fistbump logo moved to top of sidebar, inset gold left-border active indicators, 12-column Bento Grid on Home with live Readiness Ring, card-based Job Discovery stream, and glowing active selections.
- **User feedback**:
  > *"Don’t do this left highlighting thing. Avoid AI generated design tropes."*
- **Follow-up (`22f4b55`)**: Removed curved `inset 3px 0 0` left-border accent stripes, gradient nav washes, neon outer glows, and `translateY` hover lifts.
- **User feedback**:
  > *"This is better, but I truly mean a complete revamp. Keep the logos and icons, but completely change the layout. Make it look ultra premium and refined. Include beautiful animations everywhere. Avoid web app vibes."*

### Pass 3: Unified Native macOS Top Titlebar & Floating Nav Dock (`8d87b5e`)
- **What was tried**: Removed the left sidebar completely and replaced it with a unified `52px` macOS top titlebar with the Fistbump logo beside the traffic lights, a centered floating segmented pill dock (`Home · Find · Saved · Paste · Resume · Tailor · Tracker`) with a spring-animated sliding active indicator (`revamp-chrome.js`), right-aligned `⌘K` + Recents popover + Settings, and a full-bleed asymmetric 2-column Home canvas.
- **User feedback**:
  > *"Continue working. I’m not sure I like the top bar, so first, brainstorm some ideas for layouts and revamps and give pros and cons."*

### Pass 4: Grounded Bottom Instrument Bar (`eaaf905`)
- **What was tried**: Moved navigation out of the top bar into a grounded `48px` bottom instrument bar (zero sidebar, zero top nav bar, zero floating overlap), gave Home a full-width open hero strip above a 60/40 hairline workspace, and grounded Tailor's Decide Bar in flow.

### Pass 5: Tactile Visual Language Pass on Bottom Bar (`5f39bb1`)
- **What was tried**: Overhauled primitives on top of the grounded bottom bar: machined pill primary buttons, recessed pill search capsules, soft recessed form wells, full-pill segmented controls, thin native overlay scrollbars, subtle top-lit ambient studio depth, Home Spotlight Continue banner, horizontal pill chips for Recent Searches, removed legacy `border-left` on Tailor's `.reason` callout, and added automatic staggered cascades in `revamp-chrome.js`.
- **User feedback**:
  > *"It looks better, but I think you’re limiting yourself with reusing the same patterns and styles. I think the app should reuse the same icons, logo, and mascot. But otherwise, hammer at it and make it feel fresh and new. Not just reorganized."*

---

## Part 2: Brainstormed Layouts & Visual Concepts

### Round 1: 5 Native Desktop Layout Directions (with Concept Images)
1. **Ultra-Slim Icon Rail + Contextual Sub-Pane** (Bear / Xcode style `56px` left icon rail)
2. **Frameless Canvas + Floating Bottom Dock** (Apple Keynote / Stage Manager frosted bottom pill)
3. **Full-Height Vibrancy 3-Column Studio** (macOS Mail / Things 3 three-column layout)
4. **Breadcrumb Zen Instrument** (Raycast / iA Writer `40px` breadcrumb titlebar)
5. **Translucent Compact Rail + Sliding Studio Inspector** (Craft / Figma Desktop)
- **User feedback**:
  > *"I don’t really love any of these."*
  > *"I don’t like the column setups, the minimalist ones, etc. They’re either too busy or too minimal. The columns confine space and really limit the possibilities. I want the app not to feel constricting."*

### Round 2: 3 Unconfined, Non-Columnar Concepts (with Concept Images)
- **A. Spatial Sheets & Depth Stack**: Full-width Jobs gallery with a large centered reading sheet lifting cleanly over a blurred backdrop (`<` / `>` arrows + `Esc`).
- **B. Open Editorial Stage & Horizontal Ribbons**: Zero vertical splits; wide Home hero above full-width horizontal scrolling ribbons (Netflix-style).
- **C. Unconfined Document Desk + Floating Studio HUD**: Centered resume/diff desk with Match Score, Keyword Bridge, and Preview PDF in a floating bottom HUD bar.
- **User feedback**:
  > *"Also none of those. I strongly dislike the Netflix style one. What apps are renowned for their design that are applicable enough that we could adapt a fusion of them?"*

### Round 3: Renowned App Study & 3 Fusions (with Concept Images)
- **Reference apps studied**: Things 3 (in-place expanding items & warmth), Craft (spatial drill-down pages), Arc (auto-hiding chrome & Spaces), Raycast (stack navigation & bottom Action Bar), Linear (crisp hierarchy & List/Board speed), Amie / Ulysses / Kaleidoscope (indie warmth, serene writing, diff clarity).
- **Fusion 1 (Arc + Things 3 + Ulysses)**: Whisper-thin auto-hiding left edge + open canvas with Things 3-style in-place expanding cards.
- **Fusion 2 (Craft + Raycast + Linear)**: Full-window spatial drill-down (`← Jobs`) + slim Raycast-style bottom Action Bar (`Tailor Resume ↵`, `Actions ⌘K`).
- **Fusion 3 (Things 3 + Amie + Kaleidoscope)**: Single top-left workspace switcher pill (`Fistbump · Home ⌄`) + warm indie studio canvas with Pico and expanded priority card.
- **User feedback**:
  > *"Maybe it’s just the concept images, but a lot of these feel like they aren’t using the space efficiently, or in ways that are genuinely useful. I want the app to feel open, indie, and welcoming. But I also want it to be practical, well thought out, and frictionless."*

### Round 4: Practical Open-Indie 4-View Mockups (Borderless Left Rail)
- **What was explored**: 4 cohesive mockups (Home, Jobs, Tailor, Tracker) using a quiet, borderless `176px` same-canvas left rail and dense-yet-breathable practical workspaces.
- **User feedback**:
  > *"I don’t like the sidebar. This is too similar to what we have already. It needs to be different."*

### Round 5: Zero-Sidebar, Zero-Top-Bar Practical Mockups
- **1. Grounded Bottom Instrument Bar + Full-Canvas Practical Home**: Content starts immediately below traffic lights across 100% width; slim grounded bottom bar holds logo, nav, `⌘K`, and Settings.
- **2. Floating Frosted Bottom Dock + Full-Canvas Practical Jobs**: Full-width Jobs with a floating frosted glass dock `14px` above the bottom edge.
- **User feedback**:
  > *"These are better. I don’t like the floating elements. They feel like they are always in the way of things."*

### Round 6: Office Desktop / Mini-OS Concept + Fine-Motor Accessibility (with Concept Image)
- **What was explored**: An indie "Career Desk" where Pico sits on the open navy desk, multiple movable/snappable windows (`Jobs` beside `Tailor`) can be viewed simultaneously, and a grounded `46px` Launch Bar sits at the bottom. Included a fine-motor accessibility plan: zero drag-required (`40×40px` titlebar snap-left/right/full buttons + keyboard shortcuts), 1-click `Auto-Tile` mode to prevent overlapping windows, wide `14px` resize zones with step controls, and zero hover-only traps.
- **User feedback**:
  > *"Nevermind with that idea. Let’s do something more conventional."*

### Round 7: Full Structural "Current Look → New Revamp" Plan (Pending Build)
- **What was planned** (rebuilding actual DOM structures and interactions per view rather than CSS-only reskinning):
  - **Global Shell**: Warm Print & Hardware visual system (3 clear surface planes, display + tabular-mono metadata), streamlined **5-item grounded Bottom Command Bar** (`Home · Jobs · Resume · Tailor · Tracker`, moving `Saved` and `Paste` inside Jobs).
  - **Home**: Daily Career Briefing with split Hero + Jump Back In Spotlight Card, 4-stat clickable Pipeline Pulse Strip, rich Matched Role cards with inline 1-click `Save` / `Tailor`, visual Resume Readiness progress bar, and clickable pill tags for Recent Searches & Saved Versions.
  - **Jobs**: Inline `Find | Saved | Paste` header + 1-click quick filter pills (`Remote`, `Last 7d`, `70%+ Fit`, `Has Pay`), structured Role Dossier with Hero Header, 3-tile At-a-Glance Strip, and tabbed `Description` vs. `Skills & Pay Breakdown`; side-by-side Paste & live preview.
  - **Master Resume**: Section-tabbed Document Studio (`Profile · Experience · Education · Skills · Versions`) with collapsible inline entry cards and a drafting-table PDF stage with 1-click style pills and live `1-Page Fit` badge.
  - **Tailor**: Clean top mode switcher (`Resume Diff · Live PDF · Application Kit`), live animated Fit Delta meter (`76% → 92% ↑`), high-clarity Before/After diff cards, large `Accept / Reject / Edit` pills, and inline Keyword Bridge pill bank.
  - **Tracker**: Proportional clickable Stage Funnel with conversion rates, breathable card-rows (no cramped inline textareas in table cells), rich Kanban board, and frosted Slide-Over Dossier (`Overview & Notes · AI Interview Prep · Offer Compare`).
  - **Settings**: Categorized native preferences with a **Visual Theme Swatch Gallery** for all 23 themes instead of a `<select>` dropdown, plus a visual storage breakdown bar.

### Round 8: Skeuomorphism Brainstorm *(Brainstorming Only, Not Approved for Implementation)*
- **Prompt context**:
  > *"Solely for brainstorming (do not take this as a sign to add it), what is a skeuomorphism concept that could apply?"*
- **Concept explored ("Tactile Career Desk & Retro Hardware Companion")**:
  - Finely grained matte desk blotter surface adapting to all 23 themes.
  - Matte-anodized hardware bottom console with gently beveled push-buttons, warm-gold LED active dots, and mechanical keycap shortcuts.
  - Cotton bond paper for Master Resume with a visible second sheet peeking underneath on 2-page overflow and a brass paperclip for Saved Versions.
  - Editor's proofing desk for Tailor (highlighter additions, ink strikethroughs, physical analog VU Fit dial, mechanical `[A]/[R]/[E]` keycaps).
  - Tactile index-card dossiers with ink-stamp match badges (`94% FIT`) and bookmark ribbons for Jobs.
  - Tabbed filing dividers and a clipped lined notepad for Tracker Notes & AI Interview Prep.
  - Paint-chip theme swatch tray and rocker switches in Settings.
- **Note**: Kept strictly as creative exploration; heavy textures/bevels must be balanced carefully against all 23 themes and High Contrast accessibility.

### Round 9: Job-Centric Restructure (Mockups v1 → v2)
- **What was explored**: Organize the app around the job as one object moving through stages (Saved → Tailored → Applied → Interview → Offer), with three destinations (Today/Home, Jobs, Resume) and one omnibox for search/link/paste in the bottom bar.
  - **v1**: Large display type, stage-colored kanban board, gradient "next" banner, conic fit ring.
  - **v2**: Bare full-width list, one accent, hairline rows.
- **User feedback**:
  > *v1: "I don't like these elements. I don't like the layout. The layout should feel natural and frictionless. You should hardly notice it is there."*
  > *v2: "Nope. Ugly."*

### Round 10: Visual World Swatches
- **What was explored**: The same Today screen in three looks: A paper desk (light cream), B night studio (layered navy with gold), C color block (saturated blue band).
- **User feedback**:
  > *"None. I want the home to still give useful information. I prefer the bottom bar... I still think it could be more artistic, less minimal, and have more visual flair."*

### Round 11: "Paper on Night" (Approved Direction)
- **What was built**: `docs/mockups/home-v3.html`, `job-v3.html`, `tailor-v3.html`.
  - **Look**: Navy canvas with a dot grid, cream paper panels, and gold hand-drawn ink marks. One seasonal Pico per screen, speaking in a gold bubble.
  - **Home**: Up next, a 7-day calendar with inline actions, a pipeline of company tiles, Pico's skill-gap tip and new matches.
  - **Job**: Stage path, requirement checklist with resume evidence, pay and place, notes.
  - **Tailor**: A proofing desk, with the resume on paper, inline highlights and strikes, margin notes and a live fit meter.
- **User feedback**:
  > *"It looks better."* (Home iterated once via critique, then approved; Job + Tailor approved to plan the build.)
- **Build plan**: `docs/ui-v3-build-plan.md`.

### Additional Principles Learned
- **Neither loud nor bare**: Flair should come from crafted detail (ink marks, paper, Pico, real-world awareness such as holidays and the seasons), not from gradients, glows or saturated stage colors.
- **Home must be useful**: Show real information (dates, pipeline, gaps, matches), not a single focal card.
- **Bottom bar confirmed**: The grounded bottom bar is preferred over top or side navigation.

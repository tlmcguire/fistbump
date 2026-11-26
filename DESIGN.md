---
name: Fistbump
description: "Paper on Night": a local-first job search desk where cream paper sits on a navy night, marked in Pico's gold ink.
colors:
  canvas: "#111851"
  canvas-2: "#1c2668"
  canvas-deep: "#0b113b"
  on-canvas: "#eef0ff"
  on-canvas-2: "#c9cff2"
  paper: "#fff8ea"
  paper-2: "#f6ecd6"
  ink: "#151a4a"
  ink-2: "#575c86"
  rule: "rgba(21, 26, 74, 0.13)"
  mark: "#f4b53f"
  mark-ink: "#7a4d00"
  urgent: "#ef6b4a"
  urgent-ink: "#a5361a"
  success: "#4fc79a"
  success-ink: "#1d6b4e"
typography:
  display-xl:
    fontFamily: "Bricolage Grotesque, system-ui, sans-serif"
    fontSize: "48px"
    fontWeight: 800
    lineHeight: 1.08
    letterSpacing: "-0.02em"
  page-title:
    fontFamily: "Bricolage Grotesque, system-ui, sans-serif"
    fontSize: "44px"
    fontWeight: 800
    lineHeight: 1.08
    letterSpacing: "-0.02em"
  display-l:
    fontFamily: "Bricolage Grotesque, system-ui, sans-serif"
    fontSize: "30px"
    fontWeight: 780
    lineHeight: 1.15
    letterSpacing: "-0.02em"
  card-title:
    fontFamily: "Bricolage Grotesque, system-ui, sans-serif"
    fontSize: "26px"
    fontWeight: 760
    lineHeight: 1.15
    letterSpacing: "-0.02em"
  display-m:
    fontFamily: "Bricolage Grotesque, system-ui, sans-serif"
    fontSize: "22px"
    fontWeight: 760
    lineHeight: 1.2
    letterSpacing: "-0.02em"
  bubble-title:
    fontFamily: "Bricolage Grotesque, system-ui, sans-serif"
    fontSize: "19px"
    fontWeight: 780
    lineHeight: 1.2
  panel-title:
    fontFamily: "Bricolage Grotesque, system-ui, sans-serif"
    fontSize: "17px"
    fontWeight: 720
    lineHeight: 1.3
  body:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.5
  document:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, system-ui, sans-serif"
    fontSize: "14.5px"
    fontWeight: 400
    lineHeight: 1.6
  row:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 650
    lineHeight: 1.4
  meta:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 500
    lineHeight: 1.4
  small:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, system-ui, sans-serif"
    fontSize: "12px"
    fontWeight: 650
    lineHeight: 1.3
rounded:
  tag: "8px"
  segment: "9px"
  tile: "10px"
  control: "12px"
  note: "14px"
  bubble: "22px"
  paper: "20px"
  pill: "999px"
spacing:
  gutter: "18px"
  page-x: "44px"
  paper-x: "22px"
  paper-y: "18px"
components:
  paper:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.paper}"
    padding: "18px 22px"
  button-primary:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    rounded: "{rounded.control}"
    padding: "12px 22px"
  button-mark:
    backgroundColor: "{colors.mark}"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    padding: "12px 22px"
  button-quiet:
    backgroundColor: "{colors.paper-2}"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    padding: "8px 14px"
  bubble:
    backgroundColor: "{colors.mark}"
    textColor: "{colors.ink}"
    rounded: "{rounded.bubble}"
    padding: "16px 18px"
  tile:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    rounded: "{rounded.tile}"
    size: "38px"
  tag-urgent:
    textColor: "{colors.urgent-ink}"
    rounded: "8px"
    padding: "3px 9px"
  fact:
    textColor: "{colors.on-canvas}"
    rounded: "{rounded.pill}"
    padding: "5px 12px"
---

# Fistbump design system: "Paper on Night"

Source of truth in code: `app/renderer/styles/v3.css` (tokens and primitives) and the `#ink-*` symbols in `app/renderer/index.html`. Approved mockups: `docs/mockups/home-v3.html`, `job-v3.html`, `tailor-v3.html`. A view opts in by adding the `v3` class to its container.

## Overview

A job search is stressful. Fistbump should feel like a warm desk at night: calm, personal and on your side. Pico, the retro-PC mascot, is drawn in navy ink, white and gold, and the whole interface borrows that drawing language. Cream paper panels sit on a navy canvas. Gold hand-drawn marks (an underline, a circle around today, checks) are the flair. Pico appears once per screen, speaking in a gold bubble.

This is an Operate-mode app. Flair never costs scanability: every panel carries real information (dates, stages, gaps, evidence), and every screen has one primary action.

Layout rules the user set, which hold for every screen:
- Navigation lives in a grounded bottom bar: logo, Home, Jobs, Resume, the omnibox, Settings. There is no sidebar and no top nav bar.
- Nothing floats over content.
- No narrow column corridors, and no large empty margins.

## Colors

Two planes, never mixed up:
- **Canvas** (`canvas`, `canvas-2` at the top, `canvas-deep` for the bottom bar) carries `on-canvas` and `on-canvas-2` text, facts, the greeting and Pico.
- **Paper** (`paper`, `paper-2` for wells and quiet buttons) carries `ink` and `ink-2` text.

Accents come from Pico's art:
- `mark`, gold, is ink marks, Pico's bubble, the gold primary button on the canvas, and the one highlighted tile.
- `urgent`, coral, means due or missing. On paper, its text uses `urgent-ink`.
- `success`, mint, means matched or done. On paper, its text uses `success-ink`.

**Themes.** Classic uses the hand-tuned values above. The other 22 themes derive theirs from their own `--bg`, `--text` and `--muted`: paper is warm white tinted 7% toward the theme background, and ink is the background darkened 28%. All 22 are dark themes. `.high-contrast` overrides every theme with white paper, black ink and darker accent inks.

**Contrast**, measured across all 23 themes plus High Contrast:
- `ink` on `paper` is at least 9.7:1.
- `ink-2` on `paper` is at least 5.3:1.
- Every tag's text on its tint is at least 4.8:1.
- The bubble text is at least 6.2:1.

**Bright themes.** Bubblegum and Retro have bright backgrounds. In those two, the canvas does not lighten toward the top, and `mark` is lifted (`#ffe58a`, `#ffd86e`) so gold links and marks on the canvas stay at 5:1 or more.

**Older components.** A `.paper-adapt` panel re-points the original tokens (`--text`, `--muted`, `--line`, `--control`, `--field` and the rest) at ink and paper. Older components inside it, such as Settings rows, storage tables, model lists and resume forms, then read correctly without being rewritten. New components use the v3 tokens directly.

Stage is never shown by hue alone. A stage is a word, and color only reinforces it.

## Typography

Bricolage Grotesque (self-hosted, variable) is the display voice: the greeting, titles, panel titles, big numbers and tile letters. Body text, meta text and controls use the platform UI font. Numbers that change or line up use `font-variant-numeric: tabular-nums` (`.num`).

Scale:
- 48: greeting and job title
- 44: page titles (Your jobs, Your resume, Settings)
- 30: the name on a resume sheet
- 26: Up next card title
- 22 and 19: Pico's bubble
- 17: panel title
- 15: body
- 14.5: resume and posting text
- 14: row labels
- 13: meta
- 12: small uppercase labels Keep a clear step between levels. A panel title must never be the same size as a card title. Themes with `--theme-font-display` (for example Cyber's monospace) replace the display face. That is intended.

## Layout

- A 12-column grid with an 18px gutter and 44px page padding. Panels span columns; each row has one dominant panel.
- **Home:** greeting and Up next (8 columns) beside Pico's corner (4); the next 7 days at full width; pipeline (6) beside new matches (6).
- **Job:** title and facts (8) beside the decision card (4); the stage path at full width; requirements (7) beside pay and notes (5).
- **Jobs list:** one full-width paper list under the title, stage tabs and filters; "Needs you" first.
- **Tailor:** the resume sheet, with each margin note in the sheet's right column on the same row as its change.
- **Settings:** paper panels in two balanced columns under the title and jump links.
- **Resume:** sections on paper (wide column) beside readiness, preview and versions (narrow column).
- The bottom bar is 56–60px, solid `canvas-deep` with a hairline top border. Content scrolls under nothing.
- Below 1024px wide, the grid collapses to one column in reading order.

## Elevation & Depth

Paper lifts off the canvas with a soft, offset shadow (`--shadow-paper`): a 1px inner highlight on the top edge, a long blurred drop below, and a short contact shadow. Primary buttons and the bubble get a smaller version (`--shadow-control`). There are no glows, no zero-offset colored halos, no blur or glass, and no nested paper. Inside paper, structure comes from dashed rules (`.ruled`), never from cards inside cards.

## Shapes

- **Radii:** paper 20, bubble 22, controls 12, tiles 10, tags 8, facts are pills.
- **Ink marks** are hand-drawn SVG strokes with round caps, in `currentColor`:
  - `#ink-underline` under the last word of a greeting or title;
  - `#ink-circle` around today's date;
  - `#ink-check`, `#ink-half` and `#ink-open` for requirement status;
  - `#ink-star` for the empty Offer slot;
  - `#ink-leaf` for autumn decoration.
- Use a mark to point at one thing. Never repeat it as a pattern.

## Components

- **Paper** (`.paper`, `.paper-head`): the only container. A `.paper-head` holds an h2 and an optional right-aligned link or `.aside`.
- **Buttons** (`.btn`): one `.btn-primary` per screen.
  - `.btn-primary` is ink on paper. `.btn-mark` is gold, for the primary on the canvas.
  - `.btn` alone is quiet (paper-2). `.btn-text` is a text action.
  - Keyboard hints sit in `<kbd>` inside the button.
  - Disabled is an empty dashed outline, and the reason is written nearby ("Decide 2 changes to export").
- **Tags** (`.tag`, `-urgent`, `-good`, `-mark`): short status words on paper, such as "missing: Power BI" or "close match".
- **Facts** (`.fact`): pills on the canvas for company, place, pay and dates.
- **Tiles** (`.tile`, `-mark`, `-slot`): a company's initial. They make up pipeline stacks and job rows. The dashed slot is an empty stage.
- **Pico's bubble** (`.bubble.tail-right` or `.tail-down`): gold, with its tail pointing at Pico. It holds one insight and at most two buttons. One per screen.
- **Pico** (`.pico`): the seasonal monthly art on Home, and situational poses on other screens (`14-writing.png` in Tailor, `20-pdf-exported.png` on finish). Never more than one Pico visible at once.
- **Motion:** One authored moment per screen, such as the fit number counting up or pipeline tiles settling. Entrances use `.enter`. Motion stops under `.reduce-motion` or `prefers-reduced-motion`.

## Do's and Don'ts

Do:
- Give every panel real information and an action where one makes sense (Draft, Prep, Tailor, Strengthen).
- Show fit as one number, with "now → after" when there is a projection.
- Put every change the user must approve in context (on the resume), with Accept, Reject, Edit and Undo.
- Use real-world awareness: holidays, the season, "posted today."

Don't:
- Use stage-colored cards, side-stripe accents, gradient banners, glows, hover lifts, or progress rings.
- Use eyebrow or kicker labels above headings.
- Show two Picos or two speech bubbles on one screen.
- Use a sidebar, a top nav bar, floating docks or HUDs, horizontal carousels, or narrow column corridors.
- Use hue as the only signal.

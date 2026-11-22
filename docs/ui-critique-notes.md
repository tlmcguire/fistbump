# Fistbump UI critique (2026-11-22)

Method: dual-agent. Score 22/40 (Acceptable). Rendered: Home, Master resume, Tracker, Tailor (empty), Find jobs, Paste, Settings. Source only: populated Tailor diff cards, tracker rows, search results.

## Priority issues
- [P1] Privacy and approval positioning invisible; no AI settings group (engine, model download, remote key opt-in), no engine badge on Tailor, 6 disabled "future" delete buttons in Settings (index.html:276-323). Commands: shape, clarify.
- [P1] Tailor (core payoff) is least designed: no toolbar, bare job picker, default buttons, tiny status line, red/green-only ins/del (WCAG 1.4.1), no gap context, no keyboard flow (J/K, A/R/E), "Accept all remaining" invites rubber-stamping. Commands: layout, polish, animate.
- [P2] Tracker raw: no toolbar, .due row unstyled, stage select per row, no Pico empty state (33-empty-tracker.png exists). Commands: layout, colorize, onboard.
- [P2] Inconsistent chrome: toolbar missing on Tailor/Tracker; base.css:1402 `.paste-plane` typo (HTML uses `.paste-pane`); one #i-file icon for 4 nav items; native confirm()/alert() (~7 sites, diff-view.js:153,281,294, job-parser.js:953,959) vs Settings' button.confirming. Commands: polish, adapt.
- [P3] Gold accent overused; --faint/--dim/--muted text under 4.5:1 (detector: 3.4-3.6:1 on p.group-note and .setting.future labels); 11-12px labels; disabled opacity .45; flat type scale 12/14/15 on Resume. Commands: audit, colorize, typeset.

## Detector evidence
index.html: skipped-heading (h1 Settings -> h3 Recent searches). Live: flat-type-hierarchy (Resume), tiny-text #data-dir 11.5px, low-contrast p.group-note 3.6:1, low-contrast .setting.future labels 3.4:1 (+11). Home/Jobs clean; Tailor/Tracker scans unverified. All genuine.

## Heuristics
1 Status 2 | 2 Real world 3 | 3 Control 2 | 4 Consistency 2 | 5 Prevention 2 | 6 Recognition 2 | 7 Efficiency 1 | 8 Minimalism 3 | 9 Recovery 3 | 10 Help 2

## Persona red flags
Alex: no shortcuts in review loop, no tracker sort. Sam: color-only ins/del and overdue, faint contrast. Jordan: blank Master resume form, unexplained Round/Rules/match %. Morgan: 6-9 clicks listing to Tailor to Track, job re-picked, no visible engine/privacy state.

## Minor
Stale phase comments in index.html; "Rules" engine label; "↗" glyph at tracker.js:197; Home hero ~40% of viewport; Settings only a footer icon; role=tab without tabpanels.

## Questions
What on Tailor says "private, human-approved" with the logo covered? Should gold mean only "your decision needed"? A sidebar chip for local model status?

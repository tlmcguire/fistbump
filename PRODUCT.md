# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users
Active job seekers applying to many roles. They keep one master resume, tailor it per posting, and want to stay in control of the final text and of where their data goes. They use it on their own Windows, macOS or Linux desktop.

## Product Purpose
Fistbump is a local-first desktop job application assistant. It stores a master resume, parses job postings, shows skill gaps, and generates resume revisions the user reviews before exporting a PDF. It also tracks applications. Success: the user sends a tailored, truthful resume faster, and never loses control of the text or the data.

## Positioning
Privacy and human-approved edits together. Data and inference stay on the user's machine (local llama-server model, or the user's own remote key as an explicit opt-in), and every AI change is a suggestion the user accepts, rejects or edits. A cloud resume tool could not truthfully claim both.

## Operating Context
Desktop Tauri v2 app (vanilla JS renderer, Rust host, Go backend over localhost, SQLite). Workflow: master resume, then job posting (pasted, or searched via Greenhouse boards), then skill-gap analysis, then AI revision review in a diff view, then PDF export, then tracker. Skill-gap analysis is deterministic and never calls a model (target under 1.5 s).

## Capabilities and Constraints
- Windows, macOS, Linux; install under 250 MB excluding model weights, which download on demand.
- Remote AI is opt-in with the user's own key; no data leaves the machine otherwise.
- Every AI change must be accept/reject/edit; the user owns the final text.
- Platform is Tauri v2, recorded as `web`; the stack is vanilla JS, Rust, Go and SQLite and is already decided.
- Docs: `docs/architecture.md`, `docs/api.md`, `docs/schema.md`.

## Brand Commitments
- Name: Fistbump. Mascot is a friendly retro-PC character named Pico ("Meet Pico, your job-search sidekick"); use the name Pico in copy.
- Pico has a set of 39 pose illustrations, five animations and milestone award art, with usage documented in `app/renderer/views/graphics/mascots/README.md`.

## Evidence on Hand
Brand assets only: logo, Pico poses, animations and awards in `app/renderer/views/graphics/`, plus the banner in `docs/images/github-banner.png`. No testimonials, customer counts or benchmarks exist; do not fabricate them.

## Product Principles
1. The user decides: AI proposes, the user approves every word.
2. Local by default; anything that leaves the machine is explicit and opt-in.
3. Fast, explainable analysis over opaque model output.
4. Friendly guidance (Pico) without getting in the way of a stressful task.

## Accessibility & Inclusion
Target WCAG 2.1 AA.

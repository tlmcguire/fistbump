# fistbump

![Fistbump, with Pico giving a fistbump](docs/images/github-banner.png)

A local-first job application assistant. Stores a master resume, parses job postings, shows skill gaps, and generates resume revisions that the user reviews before exporting a PDF. Data stays on the user's machine unless a remote AI provider is configured.

Stack: Tauri v2 (Rust host + vanilla JS frontend), Go backend, SQLite storage.

## What it looks like

| | |
|---|---|
| ![Home: what needs you today](docs/images/home.png) | ![Master resume](docs/images/resume.png) |
| **Home.** Follow-ups that are due, suggestions to decide, and jobs to tailor. | **Master resume.** Your one source of truth, with a live PDF preview. |
| ![Saved jobs with the full posting](docs/images/jobs.png) | ![Tailoring a resume: suggestions with the skill gap beside them](docs/images/tailor.png) |
| **Saved jobs.** Search a board, open a link or paste a posting. | **Tailor.** Every change is a suggestion you accept, reject or edit; the skill gap sits beside it. |
| ![Tracker](docs/images/tracker.png) | ![Settings, with the AI engine and storage](docs/images/settings.png) |
| **Tracker.** Each application, its stage and its next step. | **Settings.** Rules only, a local model or a remote provider, and what is stored where. |

## Install

The installers are on the releases page: [github.com/tlmcguire/fistbump/releases](https://github.com/tlmcguire/fistbump/releases).

| Platform | Installer | Architectures |
|----------|-----------|---------------|
| Windows 10 and 11 | NSIS `.exe` | x64 |
| macOS 12 and later | `.dmg` | arm64 and x64, as two files |
| Linux (Ubuntu 22.04 and later, Fedora 36 and later) | `.AppImage` | x64 |

The builds are unsigned, so the first launch asks you to open the app yourself. The [user guide](docs/user-guide.md) has the steps for each platform.

## Documentation

| Doc | Contents |
|-----|----------|
| [docs/user-guide.md](docs/user-guide.md) | How to install, set up the resume, tailor it to a job and track applications |
| [docs/release-checklist.md](docs/release-checklist.md) | The manual check to run on every platform before publishing a release |
| [docs/architecture.md](docs/architecture.md) | System design, app lifecycle, Greenhouse search strategy, API key handling, file structure, Q&A |
| [docs/api.md](docs/api.md) | Backend routes, request and response shapes, error format, DB interactions |
| [docs/schema.md](docs/schema.md) | SQLite tables, relationships, constraints, settings keys |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Contribution guide |

## Developer setup

You need Rust (stable), Go 1.26, and Node 22. On Linux, also install Tauri's system packages (`libwebkit2gtk-4.1-dev`, `libappindicator3-dev`, `librsvg2-dev`, `patchelf`, `libsecret-1-dev`).

```bash
npm install          # install dependencies and sync PDF.js assets
npm run build:go     # build the Go backend into bin/<platform>-<arch>/
npm run dev          # start the Tauri app
```

The pictures above come from `node scripts/screenshots.js`, which fills a throwaway data folder with the sample resume and a few jobs and photographs each view into `docs/images/`.

`npm run build:go` builds the backend for this computer. To build for another platform or architecture, name it: `node scripts/build-go.js --platform darwin --arch x64`.

## Repository layout

| Path | Contents |
|------|----------|
| `src-tauri/` | Tauri v2 Rust host: Go backend supervisor, IPC commands, OS keychain, menus, updater, icons |
| `app/` | Vanilla JS/HTML/CSS renderer views, Tauri bridge, PDF.js preview, and renderer tests |
| `backend/` | Go module (`fistbump-core`): API, SQLite store, parser, analysis, AI providers, connectors |
| `e2e/` | Playwright Chromium smoke tests backed by the real `fistbump-core` binary |
| `scripts/` | Build, resource staging, PDF.js sync, and maintenance scripts |
| `docs/` | Design documentation |

The full tree is in [docs/architecture.md](docs/architecture.md#file-structure).

## Commands

| Command | Purpose |
|---------|---------|
| `npm run dev` | Start the Tauri desktop app in dev mode |
| `npm run build:go` | Build the Go backend into `bin/<platform>-<arch>/` |
| `npm run fetch:llama` | Put a pinned `llama-server` next to the backend so the local AI engine can run (`--all` for every platform) |
| `npm run stage:resources` | Stage `bin/<platform>-<arch>/` into `src-tauri/resources/bin/` for Tauri bundling |
| `npm run dist` | Build installers for the current platform (and signed updater artifacts when `TAURI_SIGNING_PRIVATE_KEY` is set) |
| `npm run dist:mac`, `npm run dist:win`, `npm run dist:linux` | Build installers for one platform |
| `cd backend && go test ./...` | Run Go backend tests |
| `npm run test:rust` | Run Rust host tests (`cargo test` in `src-tauri/`) |
| `npm test` | Run renderer and script tests |
| `npm run test:e2e` | Run Playwright Chromium end-to-end tests against `fistbump-core` |
| `node scripts/verify-greenhouse-boards.js` | Validate the Greenhouse board lists against the live API (`--offline` checks structure only) |

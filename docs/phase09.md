# Phase 9: Release

Build on `main` as phases 6, 7 and 8 merge, from [architecture.md](architecture.md) ("Build and packaging", "App lifecycle", "Testing") and [api.md](api.md) ("Storage"). The CI workflow (step 1) starts right after [phase 1](phase01.md) merges. Overview and schedule: [roadmap.md](roadmap.md).

## Goal

Installable builds for Windows, macOS and Linux, tested on each, with user documentation. The app reports its storage use and cleans up after itself, and recovers when the backend crashes. This meets FR-6, NFR-2, NFR-5 and Deliverable 3 (November 10).

## Scope

In:

- `.github/workflows/build.yml`: tests on every push and pull request; installers on a version tag.
- `electron-builder.yml` and pinned `electron` and `electron-builder` versions.
- The 2 Storage routes, job garbage collection, and a storage section in settings.
- Backend crash restart and the startup error screen.
- Install size measured against NFR-5.
- A release checklist, user documentation, and the `v0.1.0` release.

Out (after the MVP, build-findings 8.4 and 8.5): code signing and notarization, auto-update, licensing, data export and import, refusing a database newer than the app.

## Release targets

| Platform | Installer | Architectures |
|----------|-----------|---------------|
| Windows 10 and 11 | NSIS `.exe` | x64 |
| macOS 12 and later | `.dmg` | arm64 and x64, as two files |
| Linux (Ubuntu 20.04 and later, Fedora 36 and later) | `.AppImage` | x64 |

These match the proposal's Appendix A.2. Builds are unsigned for the MVP: macOS asks the user to right-click and choose Open the first time, and Windows SmartScreen shows "More info, Run anyway". The user guide says so with screenshots.

## Routes

As written in [api.md](api.md).

```jsonc
// GET /v1/storage
{ "data_dir": "/Users/ada/Library/Application Support/fistbump", "db_bytes": 245760,
  "models_bytes": 1117320736, "cache_bytes": 36700160, "partial_downloads_bytes": 0,
  "job_count": 42, "total_bytes": 1154266656 }

// POST /v1/storage/cleanup   { "targets": ["cache", "old_jobs", "trash"], "dry_run": true }
{ "freed_bytes": 36700160, "removed": { "old_jobs": 12, "trash": 2, "files": 517 }, "dry_run": true }
```

- **Targets.** `cache`: phase 2's Greenhouse cache (memory and the two `greenhouse_*_cache` tables). `partial_downloads`: `.part` files with no running download. `old_jobs`: jobs with no application, neither archived nor trashed, whose `searched_at` is older than `older_than_days` (default `jobs.retention_days`). `trash`: trashed jobs older than `jobs.trash_days`. `unused_models`: installed models other than `ai.selected_model`.
- Deleting jobs cascades to revisions, suggestions and tailored resumes. Jobs with applications are never removed (`RESTRICT`).
- After removing jobs, run `VACUUM` so the database file shrinks.
- **Automatic cleanup** at startup: `old_jobs` and `trash` only, logged to stderr. Files are removed only when the user asks.

| Status | `code` | When |
|--------|--------|------|
| 422 | `validation_failed` | Unknown target, `older_than_days` below 1 |

## Files, in build order

Progress:

- [ ] 1. `build.yml` (tests)
- [ ] 2. `package.json` pins and scripts
- [ ] 3. `build-go.js` folder names
- [ ] 4. `electron-builder.yml`
- [ ] 5. `gc.go`, `storage.go`
- [ ] 6. Storage section in settings (built early on `feature/settings-page`: sizes, cache cleanup, data folder; job and model cleanup wait for those features)
- [ ] 7. `backend.js` restart, startup error screen
- [ ] 8. `build.yml` (release job)
- [ ] 9. Size measurement
- [ ] 10. `docs/user-guide.md`, README
- [ ] 11. `docs/release-checklist.md` and the release
- [ ] Tests and tooling

### CI

#### 1. `.github/workflows/build.yml` (tests)

**Goal:** every push and pull request is tested on all three platforms. Start this right after phase 1 merges.

- Trigger: `push` to `main`, `pull_request`.
- Matrix: `ubuntu-latest`, `macos-latest`, `windows-latest`.
- Steps: checkout; `actions/setup-go` with `go-version-file: backend/go.mod`; `actions/setup-node` with Node 22 and npm cache; in `backend/`: `gofmt -l .` must print nothing, `go vet ./...`, `go test ./...`, and `go test -race ./...` on Linux only; at the root: `npm ci`, `npm test`, `npm run build:go`.
- Upload nothing in this job.

References: [GitHub Actions workflow syntax](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions), [actions/setup-go](https://github.com/actions/setup-go), [actions/setup-node](https://github.com/actions/setup-node).

### Packaging

#### 2. `package.json`

- Pin `electron` and `electron-builder` to exact versions (no `^`, no `*`).
- Scripts: `dist` (`npm run build:go && npm run fetch:llama && electron-builder`), `dist:mac`, `dist:win`, `dist:linux`.
- Fill `author`, `description`, and `build.appId` if not set in `electron-builder.yml`.

#### 3. `scripts/build-go.js`

**Goal:** folder names electron-builder can find.

- electron-builder's `${arch}` expands to Node's names (`x64`, `arm64`), not Go's (`amd64`). Write to `bin/<process.platform>-<process.arch>/` (`darwin-arm64`, `darwin-x64`, `win32-x64`, `linux-x64`) and map to `GOOS` and `GOARCH` when building. If phase 1 used Go's names, rename here and update `main.js`, which finds the binary by `process.platform` and `process.arch`.
- Accept `--platform` and `--arch` so CI can build the macOS x64 backend on an arm64 runner. No cgo, so cross-compiling just works.

#### 4. `electron-builder.yml`

**Goal:** each installer carries only its own platform's binaries.

```yaml
appId: com.fistbump.app
productName: FistBump
files:
  - app/**/*
  - "!app/tests/**"
asar: true
mac:
  target: [{ target: dmg, arch: [arm64, x64] }]
  category: public.app-category.productivity
  extraResources: [{ from: "bin/darwin-${arch}/", to: bin/ }]
win:
  target: [{ target: nsis, arch: [x64] }]
  extraResources: [{ from: "bin/win32-${arch}/", to: bin/ }]
linux:
  target: [{ target: AppImage, arch: [x64] }]
  category: Office
  extraResources: [{ from: "bin/linux-${arch}/", to: bin/ }]
```

- `main.js` finds the backend at `process.resourcesPath/bin/` when packaged and at `bin/<platform>-<arch>/` in development.
- Icons: `build/icon.icns`, `build/icon.ico`, `build/icon.png` (512 px) from the logo's source artwork, not the crop from the promo video (build-findings section 6).
- Installer art, found by electron-builder's defaults with no config: `build/background.tiff`, the DMG window's background (540×380 with a 2x copy inside, drawn for the default icon spots at (130, 220) and (410, 220)), and `build/installerSidebar.bmp` (164×314), Pico on the NSIS welcome and finish pages. The sidebar shows only with `nsis: { oneClick: false }`; the default one-click installer has no such pages.

References: [electron-builder configuration](https://www.electron.build/configuration), [file macros](https://www.electron.build/file-patterns#file-macros).

### Backend

#### 5. `backend/internal/store/gc.go` and `backend/internal/api/storage.go`

**Goal:** the storage report and cleanup.

- `gc.go`: `OldJobs(days, dryRun)`, `TrashedJobs(days, dryRun)` return the count and delete when not a dry run; `Vacuum()`.
- `storage.go`: sizes by walking `<data-dir>` (database files including `-wal` and `-shm`, `models/`, `cache/`, `*.part`), job count, and cleanup over the targets above. Call phase 2's `Cache.Clear()` for `cache`; its size comes from the two cache tables, not from `cache/`.
- Run `old_jobs` and `trash` cleanup at startup from `main.go`.

#### 6. `app/renderer/views/settings/settings.js` (storage section)

- Where data lives (the folder path and an "Open data folder" button calling `shell.openPath` from main), the sizes, and checkboxes for each target with a "Preview" (`dry_run: true`) that shows what would be removed, then "Clean up".
- One sentence on what leaves the computer: job searches (public board requests), model downloads, and the remote provider if configured.

### Electron

#### 7. `app/main/backend.js` and `app/main/main.js`

**Goal:** the crash recovery architecture.md plans.

- If the backend exits while the app is running, restart it with a new port and token, re-send the stored API key (phase 8), and tell the renderer (`backend:restarted`) so views reload their data. In-flight requests fail with a message the UI shows with a Retry button.
- More than 3 restarts in 60 s: stop and show the error screen.
- Error screen (startup failure or too many restarts): what happened, the last 40 lines of backend stderr, "Copy details", "Open data folder", and "Quit".

### Release

#### 8. `.github/workflows/build.yml` (release job)

**Goal:** a version tag produces installers.

- Trigger: tags matching `v*`.
- Matrix as above. Steps: the test steps, then `npm run fetch:llama` and `npm run build:go` for each target architecture, `npx electron-builder --publish never`, and `actions/upload-artifact` with the installers.
- A last job downloads the artifacts and creates a draft GitHub release with them. A person publishes it after the checklist (step 11).

#### 9. Size measurement

**Goal:** a recorded answer for NFR-5 (under 250 MB installed, without model weights).

- On each platform, record: installer size, installed size (the `.app` bundle, the install folder on Windows, the extracted AppImage), and the share from Electron, `fistbump-core`, and `llama-server`.
- The full build measured 307 MB for the unpacked Electron runtime alone, so the limit is likely exceeded. Record the numbers in the release notes and the options (for example, download `llama-server` on first use instead of bundling it). Do not drop features to meet the limit in this phase.

#### 10. `docs/user-guide.md` and `README.md`

**Goal:** the user documentation for D3.

- Install per platform, including the unsigned-app steps.
- First run: build the resume, find or paste a job, read the skill gap, tailor, review, export, track.
- AI: rules only by default; downloading a local model (size, RAM); adding a remote provider and what it receives.
- Where data lives per platform, how to back it up (copy the folder while the app is closed), and how to uninstall completely.
- Known limits: Greenhouse covers only companies that use it; skill matching is strongest for technical roles; unsigned builds.
- README: a short summary, the install link to the release, and developer setup (Go, Node 22, `npm run dev`).

#### 11. `docs/release-checklist.md` and the release

**Goal:** the same manual check on every platform before publishing.

- Fresh machine or user account; install from the draft release; the steps under "Done when" below; uninstall.
- Bump `version` in `package.json` and the backend's version string to `0.1.0`, tag `v0.1.0`, let CI build, run the checklist on each installer, then publish the draft release.

### Tests and tooling

- `gc_test.go`: old jobs removed, tracked jobs kept, archived jobs kept, trash removed after `trash_days` only, dry run removes nothing.
- `api_test.go`: storage report sizes, cleanup with each target, `422` cases.
- `app/tests/backend.test.js`: restart logic with a fake child process (exits are counted, the limit stops restarts).
- CI green on all three platforms is a requirement for merging from this phase on.

## Done when

- A fresh machine on each platform installs the release build and completes the core workflow: build a resume, save a job, see the skill gap, tailor and review, export a PDF, track the application. Once with rules only and once with a local model.
- Killing `fistbump-core` while the app runs restarts it, and the app keeps working.
- The storage section shows sizes and cleans up what the preview listed.
- Install sizes are recorded against NFR-5.
- The `v0.1.0` release has the three platforms' installers and links to the user guide.

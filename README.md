# fistbump

![Fistbump, with Pico giving a fistbump](docs/images/github-banner.png)

A local-first job application assistant. Stores a master resume, parses job postings, shows skill gaps, and generates resume revisions that the user reviews before exporting a PDF. Data stays on the user's machine unless a remote AI provider is configured.

Stack: Electron (vanilla JS) frontend, Go backend, SQLite storage.

## Install

The installers are on the releases page: [github.com/tlmcguire/fistbump/releases](https://github.com/tlmcguire/fistbump/releases).

| Platform | Installer | Architectures |
|----------|-----------|---------------|
| Windows 10 and 11 | NSIS `.exe` | x64 |
| macOS 12 and later | `.dmg` | arm64 and x64, as two files |
| Linux (Ubuntu 20.04 and later, Fedora 36 and later) | `.AppImage` | x64 |

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

You need Go 1.26 and Node 22.

```bash
npm install          # install the app's dependencies
npm run build:go     # build the Go backend into bin/<platform>-<arch>/
npm run dev          # start the app
```

`npm run build:go` builds the backend for this computer. To build for another platform or architecture, name it: `node scripts/build-go.js --platform darwin --arch x64`.

## Repository layout

| Path | Contents |
|------|----------|
| `app/` | Electron main process, preload bridge, renderer views |
| `backend/` | Go module (`fistbump-core`): API, SQLite store, parser, analysis, AI providers, connectors |
| `scripts/` | Build and maintenance scripts |
| `docs/` | Design documentation |

The full planned tree is in [docs/architecture.md](docs/architecture.md#planned-file-structure).

## Commands

| Command | Purpose |
|---------|---------|
| `cd backend && go test ./...` | Run Go tests |
| `node scripts/verify-greenhouse-boards.js` | Validate the Greenhouse board lists against the live API (`--offline` checks structure only) |
| `npm run fetch:llama` | Put a pinned `llama-server` next to the backend, so the local AI engine can run (`--all` for every platform) |
| `npm run dist` | Build the installers for the platform this computer runs |
| `npm run dist:mac`, `npm run dist:win`, `npm run dist:linux` | Build the installers for one platform only |
| `npm test` | Run the app's tests |
| `npm run test:e2e` | Run the end-to-end tests |

`npm run dev`, `npm run build:go`, `npm run fetch:llama`, `npm test` and `npm run test:e2e` are defined in `package.json`. `npm run dev` starts the app, `npm run build:go` builds the Go backend into `bin/`, and `npm run fetch:llama` puts the `llama-server` the local engine needs beside it.

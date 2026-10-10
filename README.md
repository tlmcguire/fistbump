# fistbump

A local-first job application assistant. Stores a master resume, parses job postings, shows skill gaps, and generates resume revisions that the user reviews before exporting a PDF. Data stays on the user's machine unless a remote AI provider is configured.

Stack: Electron (vanilla JS) frontend, Go backend, SQLite storage.

## Documentation

| Doc | Contents |
|-----|----------|
| [docs/architecture.md](docs/architecture.md) | System design, app lifecycle, Greenhouse search strategy, API key handling, file structure, Q&A |
| [docs/api.md](docs/api.md) | Backend routes, request and response shapes, error format, DB interactions |
| [docs/schema.md](docs/schema.md) | SQLite tables, relationships, constraints, settings keys |
| [docs/build-findings.md](docs/build-findings.md) | Differences between the full build and these plans, and open decisions |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Contribution guide |

## Repository layout

| Path | Contents |
|------|----------|
| `app/` | Electron main process, preload bridge, renderer views |
| `backend/` | Go module (`fistbump-core`): API, SQLite store, parser, analysis, AI providers, connectors |
| `scripts/` | Build and maintenance scripts |
| `docs/` | Design documentation |

The full planned tree is in [docs/architecture.md](docs/architecture.md#planned-file-structure).

## Run it

Requires Go 1.26+ and Node 22+ (`npm test` passes a glob to `node --test`, which needs Node 21 or later).

```bash
npm install
npm run dev        # builds the Go backend into bin/, then launches Electron
```

On npm 11+, install scripts are gated. If Electron's binary is missing, run `node node_modules/electron/install.js` (the approval is recorded in `package.json`).

### Local AI (optional)

```bash
npm run fetch:llama   # downloads llama-server for this platform into bin/<os>-<arch>/
```

Then download a model in Settings > Local models. A model already in the data directory's `models/` folder is selected automatically. Without a model, suggestions come from the rule-based engine or a configured remote provider.

### Demo searches

Searches in Jobs > Find known to return useful Greenhouse results. Use these for demos, and re-check them before each one, since postings change daily.

| Keywords | Location | Notes |
|----------|----------|-------|
| Software Engineer | Charleston, SC | |

## Commands

| Command | Purpose |
|---------|---------|
| `npm run dev` | Build the backend and launch the app |
| `npm test` | JS tests (IPC route table, preload/handler parity) |
| `npm run test:go` | Go tests |
| `npm run build:go -- --all` | Cross-compile the backend for every OS and arch |
| `npm run dist` | Package with electron-builder |
| `node scripts/verify-greenhouse-boards.js` | Validate the Greenhouse board lists against the live API (`--offline` checks structure only) |

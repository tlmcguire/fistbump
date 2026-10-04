# fistbump

A local-first job application assistant. Stores a master resume, parses job postings, shows skill gaps, and generates resume revisions that the user reviews before exporting a PDF. Data stays on the user's machine unless a remote AI provider is configured.

Stack: Electron (vanilla JS) frontend, Go backend, SQLite storage.

## Documentation

| Doc | Contents |
|-----|----------|
| [docs/architecture.md](docs/architecture.md) | System design, app lifecycle, Greenhouse search strategy, API key handling, file structure, Q&A |
| [docs/api.md](docs/api.md) | Backend routes, request and response shapes, error format, DB interactions |
| [docs/schema.md](docs/schema.md) | SQLite tables, relationships, constraints, settings keys |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Contribution guide |

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

`npm run dev`, `npm run build:go`, and `npm test` are defined in `package.json`. The scripts they call are not implemented yet.

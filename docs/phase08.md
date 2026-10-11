# Phase 8: Local and remote AI

Build on `main` after [phase 5](phase05.md) merges (it needs the `Provider` interface and engine order), from [architecture.md](architecture.md) ("AI layer", "API keys and secrets"), [api.md](api.md) ("AI", "Models", "Settings") and [build-findings.md](build-findings.md) section 4. Can run in parallel with phases [6](phase06.md) and [7](phase07.md). Overview and schedule: [roadmap.md](roadmap.md).

## Goal

Suggestions come from a model running on the user's computer, or from a remote provider the user configures with their own key, with the rules engine as the fallback. Model output is checked before it reaches the review, so it cannot add skills the resume does not show. This meets the "locally processed AI" part of FR-3 and NFR-1.

## Scope

In:

- `remote`: an OpenAI-compatible client, which also covers Ollama and other local servers.
- `local`: a `llama-server` child process, started on first use and stopped when idle.
- The model catalog, downloads with resume and SHA-256 checks, and the RAM check.
- Prompts, output checks (skill guard and grounding), and engine labels on suggestions.
- `secrets.js`: the remote API key encrypted with `safeStorage`.
- The 5 AI routes, 6 Models routes, and a settings view.
- `scripts/fetch-llama-server.js`, pinned to one llama.cpp build.

Out: the Hugging Face token routes (gated models; after the MVP), AI resume import and summaries (after the MVP), a hosted "ours" provider (after the MVP, build-findings 8.1).

## Decisions

Decide before step 1. The recommendation is in bold.

| Question | Options |
|----------|---------|
| Default local model | **Qwen2.5 1.5B Instruct Q4_K_M** (Apache 2.0, 1.1 GB, runs in 4 GB of RAM); Llama 3.2 3B (community license with attribution, 2 GB); Qwen2.5 3B is better but licensed for non-commercial use only (build-findings 8.3) |
| `llama-server`: bundle or download | **Bundle** in the installer (no extra download on first use, works offline; adds about 67 MB on macOS, measured in phase 9 against NFR-5); or download on first use |
| `auto` order | **Local, then remote, then rules**, as architecture.md says: data stays on the computer when it can. The proposal's fallback matrix lists remote first for bullet revisions; architecture.md supersedes it |
| One model call per target, or one per round | **One per target**: small models follow short prompts more reliably, a failure loses one item, and cancel is quicker. A round of 8 targets on a 1.5B model takes about 10 to 30 s |

## Routes

As written in [api.md](api.md), except the Hugging Face token routes, which stay `501`.

```jsonc
// GET /v1/ai/status
{ "mode": "auto", "active_engine": "local",
  "local": { "model": "qwen2.5-1.5b-instruct-q4", "installed": true, "running": false },
  "remote": { "configured": true, "needs_key": false, "base_url": "https://api.example.com/v1",
              "model": "name", "has_key": true } }

// PUT /v1/ai/remote   { "base_url": "http://127.0.0.1:11434/v1", "model": "llama3.2" }   (Ollama, no key)
// 200: status

// PUT /v1/ai/remote   { "base_url": "http://api.example.com/v1", "model": "x", "api_key": "..." }
// 422
{ "error": { "code": "validation_failed", "message": "base_url must use https unless the host is this computer", "details": { "field": "base_url" } } }

// POST /v1/ai/test   { "engine": "local" }
// 200
{ "ok": true, "latency_ms": 840, "sample": "pong", "error": null }

// POST /v1/models/qwen2.5-1.5b-instruct-q4/download
// 202
{ "download_id": "d1" }

// GET /v1/models/downloads/d1
{ "state": "running", "bytes": 312475648, "total": 1117320736, "error": null }
```

- `active_engine` is the first engine `auto` would use now.
- The API key is never returned, logged, or written to SQLite. `has_key` is all the renderer sees.
- Selecting a model is `PUT /v1/settings` with `ai.selected_model`; validate that the id is in the catalog and installed.

## Files, in build order

Progress:

- [ ] 1. `catalog.json`, `models.go`
- [ ] 2. `mem_*.go`
- [ ] 3. `download.go`
- [ ] 4. `remote.go`
- [ ] 5. `prompt.go`
- [ ] 6. `guard.go`
- [ ] 7. `local.go`
- [ ] 8. `engine.go` wiring
- [ ] 9. `ai.go`, `models.go` (api)
- [ ] 10. `fetch-llama-server.js`
- [ ] 11. `secrets.js`, `backend.js`
- [ ] 12. `ipc-handlers.js`, `preload.js`
- [ ] 13. `settings.js`
- [ ] 14. Engine labels in Tailor
- [ ] Tests and tooling

### Backend

#### 1. `backend/internal/ai/catalog.json` and `models.go`

**Goal:** the list of models the app can download, and which are installed.

- Each entry: `id`, `name`, `repo`, `file`, `revision` (a commit hash, so the file never changes under us), `sha256`, `size_bytes`, `min_ram_gb`, `license`. Start with the default model and one alternative.
- Embed with `go:embed`. `Installed(id)` checks `<data-dir>/models/<file>` exists with the catalog size. `FitsThisMachine` compares `min_ram_gb` with total RAM.
- Download URL: `https://huggingface.co/<repo>/resolve/<revision>/<file>`.

#### 2. `backend/internal/ai/mem_darwin.go`, `mem_linux.go`, `mem_windows.go`

**Goal:** total RAM, without cgo.

- macOS: `unix.SysctlUint64("hw.memsize")`. Linux: `unix.Sysinfo`. Windows: `GlobalMemoryStatusEx` from `kernel32.dll` through `windows.NewLazySystemDLL`. `golang.org/x/sys` is already an indirect dependency.

References: [golang.org/x/sys/unix](https://pkg.go.dev/golang.org/x/sys/unix), [golang.org/x/sys/windows](https://pkg.go.dev/golang.org/x/sys/windows), [Build constraints](https://pkg.go.dev/cmd/go#hdr-Build_constraints).

#### 3. `backend/internal/ai/download.go`

**Goal:** large downloads that resume, verify, and never leave a half file in place.

- Write to `<file>.part`. If it exists, send `Range: bytes=<size>-`; accept `206` and append, or `200` and start over.
- Hash while writing (`sha256` over the whole file, including the resumed part, by hashing the existing `.part` first). On completion, compare with the catalog: match renames `.part` to the final name (`os.Rename`, atomic on the same volume); mismatch deletes it and ends `failed`.
- State per download id in memory: `running`, `verifying`, `done`, `failed`, `cancelled`, with bytes and total. Cancel stops the transfer and keeps `.part`.
- One download at a time; `409` for a second or for an installed model.
- No timeout on the whole transfer; a 30 s timeout on no data received.

References: [HTTP range requests (MDN)](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Range_requests), [crypto/sha256](https://pkg.go.dev/crypto/sha256).

#### 4. `backend/internal/ai/remote.go`

**Goal:** a client for any OpenAI-compatible chat API.

- `Configure(baseURL, model, key)`; `ValidateBaseURL`: `https`, or `http` only for loopback hosts (`127.0.0.1`, `::1`, `localhost`).
- `Chat(ctx, messages, maxTokens, jsonMode) (string, error)`: `POST <base_url>/chat/completions` with `Authorization: Bearer <key>` when a key is set. With `jsonMode`, send `response_format: {"type": "json_object"}`; if the server rejects it with `400`, retry once without it.
- Redirects: allow only to the same host (`CheckRedirect`), so the key never goes elsewhere.
- Errors: trim the upstream message to 300 characters and remove the key if it appears.
- `Ready()`: configured, and has a key unless the host is loopback.
- `Suggest` runs one call per target through `prompt.go` and `guard.go`.

References: [OpenAI chat completions API](https://platform.openai.com/docs/api-reference/chat), [Ollama OpenAI compatibility](https://github.com/ollama/ollama/blob/main/docs/openai.md), [http.Client.CheckRedirect](https://pkg.go.dev/net/http#Client).

#### 5. `backend/internal/ai/prompt.go`

**Goal:** prompts that keep a small model on task.

- System message: rewrite one piece of resume text for the job; keep every fact; do not add skills, tools, numbers, titles or employers that are not in the text; return JSON `{"proposed": "..."}`.
- User message: the job title and company, matched skills, the target's label and original text. Missing skills are not sent, so the model is not tempted to add them.
- Per section: experience text keeps the same number of lines or fewer; the skills line may only be reordered; the summary is at most 3 sentences.
- Parse the JSON; on a parse error, try the first `{...}` in the reply; else drop the item.

#### 6. `backend/internal/ai/guard.go`

**Goal:** drop model proposals that add claims. A dropped proposal is never repaired; rules fill the item.

- **Skill guard:** run the phase 4 parser over the proposal; every skill it finds must be in the resume (listed skills, matched skills, certifications, or the resume text). Otherwise drop.
- **Skills line:** same items as the original, in any order.
- **Grounding:** at least 75% of the proposal's content words (lower case, stop words removed) appear in the resume text or matched skills.
- **No-op:** drop proposals equal to the original after trimming.
- Log each drop to stderr with the reason and the target label, never the text.

#### 7. `backend/internal/ai/local.go`

**Goal:** run `llama-server` only while it is needed.

- Find the binary next to `fistbump-core` (`os.Executable()`, then `llama-server` or `llama-server.exe` in the same folder). `Ready()`: binary present and a selected model installed.
- Start on first use: pick a free port (listen on `127.0.0.1:0`, read the port, close), run `llama-server --model <path> --host 127.0.0.1 --port <n> --ctx-size 4096`, poll `GET /health` until `200` (it answers `503` while loading) for up to 60 s.
- Talk to it with the `remote` client pointed at `http://127.0.0.1:<n>/v1`. No key is needed.
- Stop after `ai.idle_minutes` with no request, when the selected model changes or is deleted, and at shutdown. Stop means terminate, wait 5 s, then kill.
- Send its stdout and stderr to the backend's stderr with a `llama:` prefix.

References: [llama.cpp server README](https://github.com/ggml-org/llama.cpp/tree/master/tools/server), [os/exec](https://pkg.go.dev/os/exec).

#### 8. `backend/internal/ai/engine.go`

- Register `Local` and `Remote` in `Engines`. Phase 5's order and gap fill now apply: model drafts are labeled `local` or `remote`; items the model skipped or the guard dropped get rules drafts labeled `rules`. A round where the model contributed nothing is labeled `rules`.
- Revisions record the engine that produced most suggestions in `revisions.engine`; each suggestion has its own `engine` (phase 5's column).

#### 9. `backend/internal/api/ai.go` and `models.go`

- AI: status, mode, remote set and clear, test. `PUT /v1/ai/remote` validates and holds the key in memory only. `ai.remote.base_url` and `ai.remote.model` go to settings; the key does not.
- Models: list, get, download, download status, cancel, delete (stops `llama-server` first if the model is loaded; clears `ai.selected_model` if selected).
- Logging: never log request bodies (phase 1's rule); also make sure errors from `remote` never include the key.

### Tooling

#### 10. `scripts/fetch-llama-server.js`

**Goal:** a reproducible `llama-server` for each platform.

- A table of one pinned llama.cpp release tag and, per platform, the asset name and its SHA-256 (for example `llama-<tag>-bin-macos-arm64.zip`, `-bin-win-cpu-x64.zip`, `-bin-ubuntu-x64.zip`). Update the tag on purpose, never automatically (build-findings 7.2, item 15).
- Download, check the hash, extract `llama-server` and the shared libraries beside it into `bin/<os>-<arch>/`. The release zips ship libraries the binary loads at runtime; copying the binary alone fails.
- `npm run fetch:llama` runs it for this computer; phase 9's CI runs it per platform.

References: [llama.cpp releases](https://github.com/ggml-org/llama.cpp/releases).

### Electron

#### 11. `app/main/secrets.js` and `backend.js`

**Goal:** keep the remote key encrypted on disk and in Go memory only.

- `setRemoteKey(key)`: `safeStorage.encryptString`, write the ciphertext to `userData/secrets/remote.bin`. On Linux, if `safeStorage.getSelectedStorageBackend()` returns `basic_text`, do not write it; keep it for this session and tell the user it will not be remembered.
- `clearRemoteKey()`: delete the file and call `DELETE /v1/ai/remote`.
- At every backend start (and restart, in phase 9), after the health check, decrypt and call `PUT /v1/ai/remote` with the stored settings and key.
- No IPC method returns the key.

References: [safeStorage](https://www.electronjs.org/docs/latest/api/safe-storage).

#### 12. `app/main/ipc-handlers.js` and `app/preload/preload.js`

- Channels: `ai:status`, `ai:setMode`, `ai:setRemote` (base URL, model, key; the key goes to `secrets.js` and Go, never stored by the renderer), `ai:clearRemote`, `ai:test`, `models:list`, `models:download`, `models:downloadStatus`, `models:cancelDownload`, `models:delete`.
- Preload: `api.ai.*`, `api.models.*`.

### Renderer

#### 13. `app/renderer/views/settings/settings.js`

**Goal:** the settings view, starting with AI.

- Route `#/settings`.
- AI mode: Auto, Local, Remote, Rules only, with one line on what each sends where. "Remote" names the provider's host, so the user knows where the resume goes.
- Local model: the catalog with size, RAM needed, license, and whether it fits this computer; Download with a progress bar (poll every second), Cancel, Delete, Use this model.
- Remote provider: base URL, model, API key (a password field that is cleared after saving; shows "Key saved" when `has_key`), Save, Remove. Test buttons for local and remote show latency or the error.
- Phase 9 adds a storage section here.

#### 14. `app/renderer/views/diff-view/diff-view.js`

- Label each suggestion with its engine ("Local model", "Remote", "Rules").
- Before a round in Remote mode, show which host will receive the resume and the job text.

### Tests and tooling

- `remote_test.go` with an `httptest.Server` acting as an OpenAI-compatible API: the key header is sent; JSON mode retry on `400`; a redirect to another host is refused; the key never appears in errors; `http` refused for a non-loopback host.
- `download_test.go`: full download, resume from `.part` with `206`, server ignoring `Range` (`200`) starts over, hash mismatch deletes and fails, cancel keeps `.part`.
- `guard_test.go`: a proposal adding a missing skill is dropped; the skills line with an extra item is dropped; a reworded line under 75% grounding is dropped; a reorder passes.
- `prompt_test.go`: missing skills never appear in prompts; reply parsing with extra text around the JSON.
- `local_test.go`: use the test binary as a fake `llama-server` (the helper process pattern: `TestMain` checks an environment variable and serves `/health` and `/v1/chat/completions`). Covers start, health wait, idle stop, stop on model change.
- `engine_test.go`: a model that skips items gets rules gap fill with per-suggestion labels; a model that errors falls through.
- `api_test.go`: status reflects configuration; remote set and clear; `has_key` without the key; model select validates installation.
- Manual: download the default model in the app, run a round locally, then with Ollama or a hosted provider, then with the network off (falls back to rules, labeled).

## Done when

- A round runs on a model downloaded in the app, and on a remote provider.
- With the model removed or the network off, the round still completes on rules, and each suggestion's label says which engine wrote it.
- A proposal that adds a skill the resume lacks never reaches the review.
- The API key appears in no log line, database row, settings response, or renderer message, and survives a restart on macOS and Windows.
- `go test ./...` and `npm test` pass.

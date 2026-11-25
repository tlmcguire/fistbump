# Electron to Tauri v2 Migration Guide for Fistbump

This document outlines a complete, codebase-specific plan to migrate **Fistbump** (`v0.2.4`) from **Electron 44** (`app/main/`, `app/preload/`) to **Tauri v2** (`src-tauri/`), while preserving the existing Go backend (`backend/`), SQLite database, `llama-server` local AI runtime, and vanilla JS/HTML/CSS frontend (`app/renderer/`).

---

## 1. Executive Summary & Architectural Fit

Fistbump is already structured like a Tauri application:
- **Zero-bundler frontend (`app/renderer/`)**: Plain HTML, CSS, and vanilla JS loaded via `<script defer>` tags, communicating through a single abstraction layer (`window.api` in `app/preload/preload.js`, wrapped by `window.FistBumpAPI` in `app/renderer/js/api.js`).
- **Out-of-process Go backend (`backend/`)**: All business logic, SQLite storage, PDF generation, Greenhouse connectors, and `llama-server` orchestration live in `fistbump-core`, exposed over `127.0.0.1:<port>` with a per-launch `FISTBUMP_TOKEN` bearer token.
- **Thin Electron shell (`app/main/`)**: Electron is used only for process supervision, IPC proxying, native menus/dialogs, OS keychain encryption (`safeStorage`), window chrome/vibrancy, and self-updating.

### Expected Gains vs. Trade-offs

| Dimension | Current (Electron 44) | Target (Tauri v2) |
|---|---|---|
| **Shell binary / installer overhead** | ~100–130 MB (Chromium + Node.js) | ~5–15 MB (OS WebView + Rust binary); total installer size still includes `fistbump-core` + `llama-server` |
| **Idle RAM usage** | ~150–250 MB+ | ~40–90 MB (OS WebView) |
| **Frontend (`app/renderer/`)** | Vanilla JS/CSS/HTML | **Unchanged** (via a thin `window.api` shim) |
| **Backend (`backend/`)** | Go (`CGO_ENABLED=0`) | **Unchanged** (managed by Rust instead of Node) |
| **Rendering engine** | Bundled Chromium (consistent) | WKWebView (macOS), WebView2 (Windows), WebKitGTK (Linux) |
| **E2E testing** | Playwright `_electron.launch()` | Playwright can no longer launch the shell; requires `tauri-driver` (Linux/Win) or browser-mocked UI tests |

---

## 2. Component-by-Component Migration Map

| Current Electron File / Module | Tauri v2 Equivalent | Migration Complexity |
|---|---|---|
| `app/main/main.js` | `src-tauri/src/lib.rs` + `src-tauri/tauri.conf.json` | Medium |
| `app/main/backend.js` | `src-tauri/src/backend.rs` (`tokio::process::Command` + `reqwest`) | Medium |
| `app/main/ipc-handlers.js` | `src-tauri/src/commands.rs` (`#[tauri::command]`) | High (volume: ~70+ channels) |
| `app/preload/preload.js` | `app/renderer/js/tauri-bridge.js` (`window.__TAURI__` shim) | Low–Medium |
| `app/main/secrets.js` | `src-tauri/src/secrets.rs` (`keyring` crate) | Medium (key migration caveat) |
| `app/main/window-state.js` | `tauri-plugin-window-state` | Low |
| `app/main/menus.js` | `tauri::menu` (`MenuBuilder`, `SubmenuBuilder`) | Low–Medium |
| `app/main/error-screen.js` | Secondary `WebviewWindow` (`error.html`) | Low |
| `app/main/updates.js` | `tauri-plugin-updater` + `tauri-plugin-process` (`src-tauri/src/updates.rs` bridge) | Medium |
| `electron-builder.yml` | `src-tauri/tauri.conf.json` (`bundle`) + `tauri-action` | Medium |
| `e2e/*.spec.js` (Playwright) | `tauri-driver` (Win/Linux) + Chromium mock tests | High (macOS WebDriver gap) |

---

## 3. Target Repository Layout

```text
fistbump/
├── app/
│   ├── renderer/                  # Unchanged UI + new tauri-bridge.js
│   │   ├── index.html
│   │   ├── error.html
│   │   └── js/
│   │       ├── tauri-bridge.js    # NEW: implements window.api via Tauri invoke/listen
│   │       └── api.js             # Unchanged
│   ├── main/                      # Remove after migration is verified
│   ├── preload/                   # Remove after migration is verified
│   └── tests/                     # Port main-process unit tests to `cargo test`
├── backend/                       # Unchanged Go module (fistbump-core)
├── bin/                           # Built Go binary + llama-server & dylibs
├── scripts/
│   ├── build-go.js                # Keep (optionally output to src-tauri/binaries/ or keep bin/)
│   ├── fetch-llama-server.js      # Keep
│   └── adhoc-sign.js              # Replace with Tauri macOS signing/ad-hoc step in CI
├── src-tauri/                     # NEW: Rust host application
│   ├── Cargo.toml
│   ├── build.rs
│   ├── tauri.conf.json
│   ├── capabilities/
│   │   └── default.json
│   └── src/
│       ├── main.rs
│       ├── lib.rs                 # App lifecycle, single-instance, vibrancy, menus, navigation guard
│       ├── backend.rs             # Go process supervisor, PORT= parser, FISTBUMP_TOKEN, auto-restart
│       ├── commands.rs            # #[tauri::command] handlers replacing ipc-handlers.js
│       ├── secrets.rs             # OS keychain via `keyring` crate
│       └── updates.rs             # Thin bridge over tauri-plugin-updater + backend stop/restart
└── package.json                   # Replace electron/electron-builder with @tauri-apps/cli
```

---

## 4. Detailed Migration Plan

### 4.1 Project Initialization & Configuration (`src-tauri/`)

1. **Dependencies (`package.json`)**:
   - Remove `electron` and `electron-builder` from `devDependencies`.
   - Add `@tauri-apps/cli` (v2). Optionally add `@tauri-apps/api` if bundling, or enable `app.withGlobalTauri: true` in `tauri.conf.json` so the vanilla JS frontend can use `window.__TAURI__` with zero build step.
   - Update scripts:
     - `"dev"`: `"tauri dev"`
     - `"dist"`: `"npm run build:go && npm run fetch:llama && tauri build"`

2. **Cargo Crates (`src-tauri/Cargo.toml`)**:
   - `tauri = { version = "2", features = ["macos-private-api"] }` (needed if using transparent/overlay titlebars or vibrancy)
   - `tauri-plugin-single-instance = "2"` (replaces `app.requestSingleInstanceLock()`)
   - `tauri-plugin-window-state = "2"` (replaces `app/main/window-state.js`)
   - `tauri-plugin-dialog = "2"` (replaces Electron `dialog.showOpenDialog` / `showSaveDialog`)
   - `tauri-plugin-opener = "2"` (replaces `shell.openExternal` and `shell.showItemInFolder`)
   - `tauri-plugin-clipboard-manager = "2"` (replaces `clipboard.writeText` on the error screen)
   - `tauri-plugin-updater = "2"` + `tauri-plugin-process = "2"` (signed in-place updates via GitHub Releases `latest.json`)
   - `window-vibrancy = "0.5"` (macOS `Sidebar` vibrancy + Windows 11 `Mica`)
   - `tokio = { version = "1", features = ["process", "io-util", "sync", "time"] }`
   - `tokio-util = "0.7"` (for `CancellationToken` on `connectors:search` and `connectors:suggest`)
   - `reqwest = { version = "0.12", features = ["json", "stream"] }` (for Go backend HTTP calls and update downloads)
   - `keyring = "3"` (replaces Electron `safeStorage`)
   - `rand = "0.8"` / `hex = "0.4"` (32-byte `FISTBUMP_TOKEN` generation)
   - `serde` / `serde_json = "1"`
   - `base64 = "0.22"`

3. **Tauri Configuration (`src-tauri/tauri.conf.json`)**:
   ```jsonc
   {
     "productName": "FistBump",
     "version": "0.2.4",
     "identifier": "com.fistbump.app",
     "build": {
       "frontendDist": "../app/renderer"
     },
     "app": {
       "withGlobalTauri": true,
       "security": {
         "csp": "default-src 'self'; img-src 'self' data:; frame-src blob:; object-src blob:"
       },
       "windows": [
         {
           "label": "main",
           "title": "Fistbump",
           "width": 1240,
           "height": 820,
           "minWidth": 760,
           "minHeight": 520,
           "visible": false,
           "dragDropEnabled": false,
           "zoomHotkeysEnabled": false
         }
       ]
     },
     // Note: Enable `transparent(true)`, `titleBarStyle: Overlay`, `trafficLightPosition`,
     // and `decorations(false)` per-OS in Rust (`lib.rs`) so Linux stays opaque/decorated
     // without compositor glitches. Add `"core:webview:allow-set-webview-zoom"` plus dialog,
     // opener, and clipboard permissions in `src-tauri/capabilities/default.json`.
     },
     "bundle": {
       "active": true,
       "icon": [
         "../build/icon.icns",
         "../build/icon.ico",
         "../build/icon.png"
       ],
       "category": "Productivity",
       "resources": {
         "../bin/${os}-${arch}/": "bin/"
       }
     }
   }
   ```
   > **Note on `bin/` vs `externalBin`**: `fistbump-core` sits alongside `llama-server` and ~12 shared libraries (`libllama*.dylib`, `libggml*.dylib`, `libmtmd*.dylib` in `bin/darwin-x64/`, etc.). Using `bundle.resources` (instead of Tauri's `externalBin` sidecar suffixing) keeps `fistbump-core` and `llama-server` + its `.dylib`/`.dll`/`.so` files in the exact same `bin/` directory, matching `app/main/backend.js` and `scripts/fetch-llama-server.js` with minimal changes. (Verify `tauri.conf.json` resource globbing per target OS in CI, or stage the active platform's `bin/<os>-<arch>` folder into `src-tauri/resources/bin/` during the build script.)

---

### 4.2 User Data Directory Continuity (Critical)

In `app/main/main.js`:
```js
app.setPath('userData', process.env.FISTBUMP_USER_DATA || path.join(app.getPath('appData'), 'fistbump'));
```
By default, Tauri's `app.path().app_data_dir()` uses `bundle.identifier` (`com.fistbump.app`), which resolves to `~/Library/Application Support/com.fistbump.app` on macOS and `%APPDATA%\com.fistbump.app` on Windows.

**To avoid stranding existing users' SQLite databases, settings, and downloaded GGUF models**:
- Resolve the data directory in Rust as:
  1. Check `std::env::var_os("FISTBUMP_USER_DATA")` first (preserves test/dev overrides).
  2. Otherwise, use `app.path().data_dir()?.join("fistbump")` (lowercase `"fistbump"`, matching Electron's `app.getPath('appData') + '/fistbump'`).
- Ensure `std::fs::create_dir_all(&data_dir)` is called before spawning `fistbump-core`.

---

### 4.3 Go Backend Supervisor (`app/main/backend.js` -> `src-tauri/src/backend.rs`)

Port the `Backend` class to a `BackendManager` struct managed via `app.manage(...)`:
- **Binary resolution**:
  - Dev (`cfg!(debug_assertions)`): `<repo_root>/bin/<os>-<arch>/fistbump-core[.exe]`
  - Release: `app.path().resource_dir()?.join("bin").join(binary_name)`
- **Startup & Auth**:
  - Generate a 32-byte random hex token (`FISTBUMP_TOKEN`).
  - Spawn `fistbump-core --data-dir <data_dir>` with `FISTBUMP_TOKEN` in env (not CLI args) and piped `stdin`, `stdout`, `stderr`.
  - Read `stdout` lines until `^PORT=(\d+)$` matches (10s timeout), then poll `GET http://127.0.0.1:<port>/v1/health` with `Authorization: Bearer <token>`.
- **Stderr tail**:
  - Maintain a `VecDeque<String>` (cap 200 lines; expose last 40 lines to `error:details`). Keep across restarts just like `backend.js`.
- **Crash recovery**:
  - Monitor child exit in a background Tokio task. If unexpected exit occurs while not stopping:
    - Track restart timestamps in a 60-second sliding window (`MAX_RESTARTS = 3`).
    - On restart: generate a new port/token, re-call `restore_remote_key` from OS keychain, and emit `app.emit("backend:restarted", ())`.
    - If `>= 3` crashes in 60s: transition to `Failed` and open the startup error screen (`error.html`).
- **Graceful shutdown**:
  - On `RunEvent::ExitRequested` / `Exit`, close `child.stdin` (signaling `fistbump-core` to exit cleanly) and wait up to 5 seconds before `child.kill()`.

---

### 4.4 Preload & IPC Bridge (`app/preload/preload.js` + `app/main/ipc-handlers.js`)

#### Zero-Rewrite Frontend Strategy (`app/renderer/js/tauri-bridge.js`)
Keep `app/renderer/js/api.js` and all view scripts untouched by loading `js/tauri-bridge.js` immediately before `js/api.js` in `app/renderer/index.html` and `app/renderer/error.html`:

```js
// app/renderer/js/tauri-bridge.js
(() => {
  if (window.api) return; // Allows Electron or Playwright mocks to coexist during migration
  const { invoke } = window.__TAURI__.core;
  const { listen } = window.__TAURI__.event;
  const { getCurrentWebview } = window.__TAURI__.webview;

  const platformMap = { macos: 'darwin', windows: 'win32', linux: 'linux' };
  const os = window.__TAURI_INTERNALS__?.metadata?.currentPlatform || '';

  window.api = {
    platform: platformMap[os] || (navigator.userAgent.includes('Mac') ? 'darwin' : navigator.userAgent.includes('Win') ? 'win32' : 'linux'),
    setZoom: (factor) => {
      if ([0.9, 1, 1.15, 1.3].includes(factor)) {
        getCurrentWebview().setZoom(factor).catch(() => {});
      }
    },
    onBackendRestarted: (cb) => { listen('backend:restarted', () => cb()); },
    error: {
      details: () => invoke('error_details'),
      copyDetails: () => invoke('error_copy_details'),
      quit: () => invoke('error_quit'),
    },
    // Map remaining namespaces (connectors, resume, resumes, revisions, suggestions,
    // tailored, ai, shell, settings, storage, app, jobs, cost, models, applications)
    // directly to `invoke('channel_name', { ... })`
  };
})();
```

#### Rust Command Handlers (`src-tauri/src/commands.rs`)
1. **Direct REST proxy (`ROUTES` equivalent)**:
   - Instead of writing 65+ boilerplate functions, you can implement either typed `#[tauri::command]` functions per domain **or** a single validated `backend_request(channel, args)` command in Rust that mirrors the allowlist + `isId` / `AI_MODES` / `CLEANUP_TARGETS` validation in `ipc-handlers.js`.
   - Keeping validation in Rust ensures the renderer still never sees the backend `port` or `FISTBUMP_TOKEN`.
2. **Cancellable searches (`connectors:search`, `connectors:suggest`)**:
   - Store `active_search: Mutex<Option<CancellationToken>>` and `active_suggest: Mutex<Option<CancellationToken>>` in `AppState`.
   - Aborting the token cancels the in-flight `reqwest` future, closing the TCP socket so `fistbump-core` cancels its Greenhouse work—matching `AbortController` in `ipc-handlers.js`.
3. **File import (`resume:importFile`, `resume:importData`)**:
   - `resume:importFile`: Call `app.dialog().file().add_filter("Resumes", &["pdf", "txt", "md"]).blocking_pick_file()`, check 5 MB limit (`MAX_IMPORT_BYTES`), base64-encode, and `POST /v1/resume/import`.
   - `resume:importData`: Accept `{ filename, bytes: Vec<u8> }` (or base64 string from `File.arrayBuffer()`) and run the same size/extension validation.
4. **File export & PDF preview (`resume:export`, `tailored:export`, `resume:previewPdf`, `tailored:previewPdf`)**:
   - Parse `Content-Disposition` (`filename*` and `filename`) in Rust to suggest the default filename.
   - Show save dialog via `tauri-plugin-dialog` defaulted to `app.path().document_dir()`, write bytes with `std::fs::write`, and store `last_saved: Mutex<Option<PathBuf>>` so `shell:showItem` only reveals files Fistbump exported (or `exported_pdf_path` from `/v1/applications?archived=include`).
   - Reveal in Finder/Explorer/file manager using `tauri_plugin_opener::reveal_item_in_dir(&path)`.

---

### 4.5 Window Chrome, Vibrancy, Zoom & Navigation Guards

- **macOS (`darwin`)**:
  - Set `titleBarStyle: "Overlay"`, `hiddenTitle: true`, `trafficLightPosition: { x: 16, y: 19 }`, `transparent: true`.
  - Apply vibrancy in `setup()`:
    ```rust
    window_vibrancy::apply_vibrancy(&window, NSVisualEffectMaterial::Sidebar, Some(NSVisualEffectState::FollowsWindowActiveState), None)?;
    ```
  - Force dark appearance on macOS (`NSWindow` / `NSApp` dark appearance) to match `nativeTheme.themeSource = 'dark'`.
- **Windows 11 (`win32`)**:
  - Disable native decorations (`decorations: false` or custom titlebar) and call `window_vibrancy::apply_mica(&window, Some(true))` (dark mode Mica).
  - Because Electron's `titleBarOverlay` (native min/max/close buttons drawn over the 52px toolbar) does not exist identically in Tauri, add lightweight HTML window-control buttons (`minimize`, `maximize`, `close`) in `app/renderer/index.html` shown only on `win32`/`linux` and wired to `getCurrentWindow().minimize()` / `toggleMaximize()` / `close()`.
- **Drag regions**:
  - Electron uses CSS `-webkit-app-region: drag` (`.drag` in `index.html`). In Tauri v2, add `data-tauri-drag-region` to `.sidebar-top` and `.toolbar` elements in `app/renderer/index.html` (interactive child inputs/buttons inside `.toolbar` automatically remain clickable when `data-tauri-drag-region` is on the container, though verify `<form>` / `<input>` hit-testing on Windows WebView2).
- **Preventing pinch/keyboard zoom & external navigation**:
  - In `WebviewWindowBuilder`:
    - Disable default browser zoom shortcuts (`Cmd/Ctrl + +/-/0`) via a small JS `keydown` listener in `tauri-bridge.js` (and `zoom_hotkeys_enabled(false)` in Tauri).
    - Use `.on_navigation(|url| ...)` to block navigating the WebView away from `tauri://localhost` / `http://tauri.localhost`—if `url.scheme() == "https"`, open it via `tauri_plugin_opener::open_url` and return `false`.
    - Use `.on_new_window(|url, _| ...)` to deny `target="_blank"` / `window.open()` inside the app and forward `https:` URLs to the system browser.

---

### 4.6 Secrets & Keychain (`app/main/secrets.js`)

- Replace `safeStorage` (`ai-remote-key.bin`) with the `keyring` crate (`Entry::new("com.fistbump.app", "ai-remote-key")`):
  - macOS: Keychain
  - Windows: Windows Credential Manager
  - Linux: Secret Service (`libsecret`)
- **Upgrade caveat**: Electron's `safeStorage` encrypts `ai-remote-key.bin` using Chromium's OS-keychain-derived AES key (`v10`/`v11` prefix). A Rust Tauri binary **cannot** directly decrypt `ai-remote-key.bin` without reimplementing Chromium's key derivation.
  - **Recommended approach**: Document in release notes that users upgrading from Electron to Tauri will need to re-enter their optional remote AI API key once in Settings (local models and all SQLite data remain untouched).

---

### 4.7 Menus, Context Menus & Shortcuts (`app/main/menus.js`)

- **Application Menu**:
  - Build in Rust using `tauri::menu::{MenuBuilder, SubmenuBuilder, MenuItemBuilder, PredefinedMenuItem}`:
    - macOS app menu (`About Fistbump`, `Settings…` with `Cmd+,`, `Services`, `Hide`, `Hide Others`, `Show All`, `Quit`).
    - `File` (`Close` on macOS; `Settings` `Ctrl+,` + `Quit` on Win/Linux).
    - `Edit` (`Undo`, `Redo`, `Cut`, `Copy`, `Paste`, `Select All`).
    - `View` (`Reload` / `Toggle DevTools` only when `cfg!(debug_assertions)`, `Fullscreen`).
    - `Window` & `Help` (`Fistbump on GitHub` opening `https://github.com/tlmcguire/fistbump`).
  - Clicking `Settings` emits `app:open-settings` to the main window.
- **Context Menu**:
  - OS WebViews natively provide Cut/Copy/Paste/Select All on editable fields and Copy on text selections.
  - To preserve the `"Open in Browser"` and `"Copy Link"` right-click items on `https:` links (and suppress the default browser context menu like `Inspect Element` / `Reload` in release builds), listen for `contextmenu` in `tauri-bridge.js` and either render a lightweight custom menu or invoke a Rust `popup_context_menu` command using `tauri::menu::ContextMenu`.

---

### 4.8 Auto-Updater (`app/main/updates.js` -> `tauri-plugin-updater`)

**Selected approach: `tauri-plugin-updater` + `tauri-plugin-process`**
- Uses Tauri's built-in updater for signature verification, download progress, and native in-place restart across macOS, Windows, and Linux.
- **Setup requirements**:
  - Generate a Tauri `minisign` keypair, store the private key in GitHub Actions secrets, and add the public key to `src-tauri/tauri.conf.json` (`plugins.updater.pubkey`).
  - Enable `bundle.createUpdaterArtifacts: true` and publish `latest.json` (plus `.sig` files) alongside every GitHub Release.
- **1:1 UI bridge (`src-tauri/src/updates.rs`)**:
  - Wrap `tauri-plugin-updater` in Rust commands matching the existing Settings UI (`checkForUpdates`, `downloadUpdate`, `cancelUpdate`, `installUpdate`, `onUpdateProgress`), stopping `fistbump-core` (`backend.stop().await`) right before `update.install()` and `app.restart()`.

---

### 4.9 1:1 Parity Caveats & Exact Mitigations

To make the Tauri build a true **1:1 functional and visual copy** of the Electron app across macOS, Windows, and Linux, every subtle behavioral difference between Chromium/Electron and OS WebViews/Tauri must be addressed explicitly:

#### 1. Inline PDF Preview (`app/renderer/js/pdf-preview.js`) — **High Risk**
- **Current behavior**: Creates a `Blob` URL (`application/pdf`) and loads `<iframe src="${url}#toolbar=0&navpanes=0&view=FitH">` using Electron's bundled PDFium viewer.
- **Caveat**:
  - **Linux (WebKitGTK)** has no built-in inline PDF viewer for `<iframe>`; `blob:` PDFs render blank or trigger a download.
  - **macOS (WKWebView)** uses PDFKit, which ignores Chromium's `#toolbar=0&navpanes=0&view=FitH` fragments and can display an unwanted hover toolbar.
- **1:1 Fix**: Bundle `pdfjs-dist` (PDF.js) locally inside `app/renderer/` and render PDF pages to `<canvas>` inside `.pdf-body`. This guarantees identical, toolbar-less, fit-to-width previews on all three OSes with zero external requests.

#### 2. Drag Regions & Expanded PDF Overlay (`styles/base.css:109`, `styles/views.css:258`)
- **Current behavior**: `.drag { -webkit-app-region: drag; }` and `body.pdf-expanded .drag { -webkit-app-region: no-drag; }` prevent the toolbar drag region from swallowing clicks on the expanded PDF Close button.
- **Caveat**: Tauri v2 uses the HTML attribute `data-tauri-drag-region` rather than CSS `-webkit-app-region`, so `body.pdf-expanded .drag { -webkit-app-region: no-drag; }` **will have no effect**.
- **1:1 Fix**: Add `data-tauri-drag-region` to `.sidebar-top` and `.toolbar`, and in `pdf-preview.js`'s `setExpanded(on)`, toggle `data-tauri-drag-region` off while expanded (and restore it on collapse).

#### 3. Windows 11 `titleBarOverlay` & Windows 10 Fallback (`app/main/main.js`)
- **Current behavior**: Electron uses `titleBarOverlay: { color: '#00000000', symbolColor: '#f2f5fb', height: 52 }` to draw native Windows 11 minimize/maximize/close buttons (including Snap Layouts on hover) over the 52px toolbar, with Mica behind the window.
- **Caveat**: Tauri v2 has no built-in `titleBarOverlay`. Setting `decorations: false` removes native window controls and Windows 11 Snap Layouts. Also, `apply_mica` is Windows 11-only; Windows 10 needs a clean fallback.
- **1:1 Fix**:
  - Add custom HTML min/max/close buttons in `.toolbar` shown only on `platform-win32` (wired to `getCurrentWindow().minimize()` / `toggleMaximize()` / `close()`), or use `tauri-plugin-decorum` / Win32 `WM_NCHITTEST` if native Snap Layouts are required.
  - Only enable `transparent: true` on macOS/Windows (never Linux, where transparency can cause X11/Wayland compositor glitches), and fall back to solid `#0b2457` when `apply_mica` returns an error on Windows 10.

#### 4. macOS Dock & Window Close Lifecycle (`app/main/main.js`)
- **Current behavior**: On macOS, closing the last window keeps the app and `fistbump-core` running in the Dock; clicking the Dock icon reopens the window (`app.on('activate')`). Windows/Linux quit on `window-all-closed`.
- **Caveat**: Tauri v2 quits the entire process when the last window closes on macOS by default.
- **1:1 Fix**: On macOS, intercept `RunEvent::ExitRequested` (call `api.prevent_exit()` unless an explicit Quit was requested) and handle `RunEvent::Reopen` to recreate or show the main window. Stop `fistbump-core` only on actual app exit.

#### 5. HTML5 Drag-and-Drop for Resume Import (`resume:importData`)
- **Current behavior**: Dropping a `.pdf`, `.txt`, or `.md` file onto the window triggers standard DOM `drop` events and reads `File.arrayBuffer()`.
- **Caveat**: Tauri v2 defaults to `"dragDropEnabled": true`, which intercepts OS file drops at the native WebView level and **blocks** HTML5 `dataTransfer.files` drop events.
- **1:1 Fix**: Set `"dragDropEnabled": false` on the main window in `tauri.conf.json` so existing DOM drag-and-drop works unchanged.

#### 6. Page Zoom & Global Spellcheck (`js/prefs.js`, `app/main/main.js`)
- **Current behavior**: `webFrame.setZoomFactor` scales the UI to `0.9 | 1 | 1.15 | 1.3`, pinch/Ctrl+scroll/Cmd+Plus zoom is blocked, and `spellcheck: false` is set globally on `BrowserWindow`.
- **Caveat**: Tauri's `setZoom` requires `"core:webview:allow-set-webview-zoom"` in `src-tauri/capabilities/default.json`, and Tauri has no global `BrowserWindow` spellcheck toggle.
- **1:1 Fix**: Add the capability permission, disable zoom hotkeys (`zoom_hotkeys_enabled(false)` + JS `keydown` guard), and add `spellcheck="false"` to `<body>` in `app/renderer/index.html`.

#### 7. Settings "About" String & i18n Keys (`views/settings/settings.js:566`, `js/locales/{de,fr,es}.js`)
- **Current behavior**: Displays `Version {0} for {1} ({2}). Electron {3}, Chromium {4}.` (and translates that exact string in `de.js`, `fr.js`, and `es.js`).
- **1:1 Fix**: Return `{ version, tauri, webview, platform, arch, dataDir }` from `app:info` in Rust and update the About template in `settings.js` plus the matching keys in `de.js`, `fr.js`, and `es.js`.

#### 8. Pre-existing `Cmd/Ctrl+,` Menu Hook (`app/main/menus.js`, `js/app.js:149`)
- **Current behavior**: `menus.js` sends `app:open-settings` and `app.js` registers `window.api?.app?.onOpenSettings`, though `preload.js` left it unexposed.
- **1:1 Fix**: Expose `app.onOpenSettings: (cb) => listen('app:open-settings', () => cb())` in `tauri-bridge.js` so the native menu item and `Cmd/Ctrl+,` work seamlessly.

#### 9. Upgrade Continuity: Remote AI Key (`app/main/secrets.js`)
- **Current behavior**: `safeStorage` encrypts `ai-remote-key.bin` using Chromium's internal OS-keychain-derived AES key.
- **Caveat**: A Rust Tauri binary using `keyring` **cannot** decrypt Electron's `ai-remote-key.bin`.
- **1:1 Fix**: Keep the data directory at `<appData>/fistbump` so SQLite (`fistbump.db`), settings, and local GGUF models migrate with zero data loss, and note in release notes that users with a configured remote AI provider will need to re-enter their API key once after upgrading.

#### 10. Updater Signing, Transition Release Naming, DMG Badge & Linux Distro Support (`electron-builder.yml`, `updates.js`, `build.yml`)
- **Updater signing & `latest.json`**: `tauri-plugin-updater` requires storing the Tauri `minisign` private key in GitHub Actions secrets, setting `bundle.createUpdaterArtifacts: true`, and publishing `latest.json` + `.sig` files alongside every GitHub Release. It handles Windows NSIS silent updates, macOS `.app` replacement, and Linux AppImage replacement natively (no custom `MAC_INSTALL` script or NSIS `--force-run` workaround needed for Tauri-to-Tauri updates).
- **One-time Electron -> Tauri transition release**: Existing Electron (`v0.2.4`) installs check `updates.js`'s `INSTALLER` regex (`FistBump-<version>-(Windows-x64.exe|macOS-(arm64|x64).dmg|Linux-x86_64.AppImage)`). For the first Tauri release, either also publish renamed installer files matching that pattern or direct Electron users to download the Tauri installer from GitHub Releases.
- **macOS DMG volume badge & ad-hoc signing**: Tauri supports `build/background.tiff` in `bundle.macOS.dmg`, but does not natively apply `build/badgeIcon.icns` as the mounted volume icon (requires a small post-build script if you want that exact polish). Retain ad-hoc signing (`codesign --force --deep --sign -`) on the `.app` bundle and bundled `bin/` binaries/dylibs before DMG creation.
- **Linux baseline (Ubuntu 20.04 vs. 22.04)**: `README.md` currently lists Ubuntu 20.04+. Tauri v2 requires `libwebkit2gtk-4.1`, which is standard on **Ubuntu 22.04+** and **Fedora 36+**, but not available in Ubuntu 20.04's default repos. Minimum supported Ubuntu version will bump from 20.04 to **22.04**.

#### 11. Screenshot & E2E Tooling (`scripts/screenshots.js`, `e2e/*.spec.js`)
- **Current behavior**: Both `scripts/screenshots.js` (README screenshots) and `e2e/*.spec.js` rely on Playwright's `_electron.launch()`.
- **Caveat**: Playwright cannot launch a Tauri binary, and `tauri-driver` does not support macOS WKWebView.
- **1:1 Fix**: Run `scripts/screenshots.js` and fast UI smoke tests against `app/renderer/` served over localhost with a thin HTTP bridge to `fistbump-core` (works on macOS/Win/Linux), and add `tauri-driver` E2E tests on Linux/Windows CI runners.

#### 12. Minor Cross-WebView CSS/Form Differences
- Native form controls (`<select>`, `<input type="search">` clear buttons, `<details>` markers) and custom scrollbars (`::-webkit-scrollbar`) render slightly differently in WKWebView/WebKitGTK compared to Chromium. Audit every view on all three OSes during Phase 2.

---

## 5. Testing & CI/CD Migration

### Unit Tests (`app/tests/*.test.js`)
- Renderer-focused tests (`dom-helpers.test.js`, `i18n.test.js`, `places.test.js`, `renderer-wiring.test.js`) remain unchanged under `node --test`.
- Main-process tests (`backend.test.js`, `ipc.test.js`, `secrets.test.js`, `updates.test.js`) should be ported to Rust unit/integration tests (`cargo test` in `src-tauri/`).

### End-to-End Tests (`e2e/*.spec.js`, `playwright.config.js`)
- Playwright's `_electron.launch()` cannot launch a Tauri binary.
- **Recommended split**:
  1. **UI / Renderer E2E (Playwright + Chromium)**: Serve `app/renderer/` over a local HTTP server with a lightweight `window.api` mock (or HTTP bridge to a test `fistbump-core` instance) so existing fast UI smoke tests still run on macOS/Linux/Windows.
  2. **Full App E2E (`tauri-driver`)**: Run `tauri-driver` + WebdriverIO on **Linux and Windows** CI runners (note: `tauri-driver` does **not** support macOS WKWebView).

### CI/CD (`.github/workflows/build.yml`)
- Replace `electron-builder` with `tauri-apps/tauri-action` on `macos-latest` (`aarch64-apple-darwin`, `x86_64-apple-darwin`), `windows-latest` (`x86_64-pc-windows-msvc`), and `ubuntu-22.04` (`x86_64-unknown-linux-gnu`).
- Keep `npm run build:go` and `npm run fetch:llama` as pre-build steps.
- On macOS, retain ad-hoc signing (`codesign --force --deep --sign -`) on the built `.app` bundle before DMG creation if not using an Apple Developer ID certificate, and preserve `build/background.tiff` / `build/badgeIcon.icns` DMG styling via `tauri.conf.json` (`bundle.macOS.dmg`).

---

## 6. Phased Execution Roadmap (1:1 Parity)

Each phase leaves the Electron app runnable in parallel until Phase 5 cutover, so you can diff behavior side-by-side at every step.

### Phase 1: Tauri v2 Scaffold, Data Continuity & Go Backend Supervisor
**Goal**: Boot the Tauri shell against `app/renderer/` and supervise `fistbump-core` with zero data migration.
- [x] Add `@tauri-apps/cli` (v2) to `package.json` (add temporary `dev:tauri` / `build:tauri` scripts while keeping Electron scripts intact during migration).
- [x] Initialize `src-tauri/` (`Cargo.toml`, `build.rs`, `tauri.conf.json`, `capabilities/default.json`) with `frontendDist: "../app/renderer"`, `withGlobalTauri: true`, `dragDropEnabled: false`, and `zoomHotkeysEnabled: false`.
- [x] Configure `src-tauri/capabilities/default.json` with `core:default`, `core:webview:allow-set-webview-zoom`, `dialog:default`, `opener:default`, and `clipboard-manager:default`.
- [x] Resolve the user data directory in Rust as `FISTBUMP_USER_DATA` or `<appData>/fistbump` (lowercase) so existing SQLite databases (`fistbump.db`), caches, and downloaded GGUF models load untouched.
- [x] Implement `src-tauri/src/backend.rs`: binary lookup (`bin/<os>-<arch>/` in dev, `resource_dir/bin/` in release), 32-byte `FISTBUMP_TOKEN` env var, `PORT=` stdout parser (10s timeout), `/v1/health` probe, 200-line stderr ring buffer (40-line tail), crash auto-restart (max 3 in 60s + `backend:restarted` event), and graceful stdin-close shutdown (5s timeout).
- [x] **Phase 1 Gate**: Run `npm install`, `cargo test` (in `src-tauri/`), and `npm run dev:tauri` to verify the window launches, spawns `fistbump-core` on `<appData>/fistbump`, survives a simulated crash, and cleanly terminates `fistbump-core` on quit.

### Phase 2: IPC Bridge, Core Commands & Keychain Secrets
**Goal**: Full feature parity for all ~70+ IPC channels without rewriting `app/renderer/js/api.js` or view scripts.
- [x] Create `app/renderer/js/tauri-bridge.js` implementing `window.api` via `window.__TAURI__` (with `if (window.api) return;` guard so Electron and Playwright mocks keep working during migration) and include it before `js/prefs.js` / `js/api.js` in `index.html` and `error.html`.
- [x] Wire `app.onOpenSettings` in `tauri-bridge.js` (fixing the pre-existing `preload.js` gap for `Cmd/Ctrl+,`).
- [x] Port `ipc-handlers.js` to `src-tauri/src/routes.rs` and `src-tauri/src/commands.rs`: validated REST proxy for all `ROUTES` (keeping `port` and `FISTBUMP_TOKEN` hidden from the renderer), `CancellationToken`-backed `connectors:search` and `connectors:suggest`, `resume:importFile` (open dialog + 5 MB cap) and `resume:importData`, `resume:export` / `tailored:export` (`Content-Disposition` filename parsing + save dialog + `last_saved` allowlist + application `exported_pdf_path` update), `resume:previewPdf` / `tailored:previewPdf`, `shell:showItem`, `storage:*`, and `app:info`.
- [x] Implement `src-tauri/src/secrets.rs` using the `keyring` crate (`com.fistbump.app` / `ai-remote-key`) and call `restore_remote_key` on initial startup and every `backend:restarted` event.
- [x] **Phase 2 Gate**: Run `cargo test` and `npm run dev:tauri`, then walk through Master Resume CRUD + drag-and-drop file import (`.pdf`/`.txt`/`.md`), Greenhouse search + cancel, Job paste/save/archive/trash, Tailoring + AI test, Application Tracker, Export + Reveal in folder, and Settings.

### Phase 3: Cross-WebView UI Parity (PDF, Drag Regions, Zoom, Spellcheck & i18n)
**Goal**: Eliminate every visual and behavioral divergence between bundled Chromium and WKWebView / WebView2 / WebKitGTK.
- [x] **PDF Preview**: Bundle `pdfjs-dist` locally in `app/renderer/vendor/pdfjs/` (via `scripts/sync-pdfjs.js` on `postinstall`) and update `app/renderer/js/pdf-preview.js` + `styles/views.css` to render PDF pages to `<canvas>` inside `.pdf-body` (with `<iframe>` fallback) so previews are identical and toolbar-free on macOS, Windows, and Linux.
- [x] **Drag Regions & Expanded PDF**: Add `data-tauri-drag-region` to `.sidebar-top` and `.toolbar` in `index.html` plus all view-created `.toolbar.drag` headers (`home.js`, `diff-view.js`, `tracker.js`, `resume-builder.js`), and update `setExpanded(on)` in `pdf-preview.js` to remove/restore `data-tauri-drag-region` on `.drag` elements while expanded so the Close button is never swallowed.
- [x] **Zoom & Spellcheck**: Block `Cmd/Ctrl + +/-/0` in `tauri-bridge.js`, keep `window.api.setZoom` working across `0.9 | 1 | 1.15 | 1.3`, and add `spellcheck="false"` to `<body>` in `index.html`.
- [x] **About String & Locales**: Return `{ version, tauri, platform, arch, dataDir }` from `app:info` and update the About line in `app/renderer/views/settings/settings.js` plus the matching translation keys in `js/locales/{de,fr,es}.js`.
- [x] **Cross-WebView CSS Normalization**: Add `color-scheme: dark` on `:root` and `-webkit-appearance: none; appearance: none;` on `input[type="search"]` in `styles/base.css`.
- [x] **Phase 3 Gate**: Run `npm install` (to install and sync `pdfjs-dist`), then `npm run dev:tauri` and verify Master Resume & Tailored PDF preview (including Expand/Close), Settings Text Size zoom, About version string + language switching, and search inputs.

### Phase 4: Native Shell Polish, Lifecycle, Menus, Error Screen & Auto-Updater
**Goal**: Match every OS-native detail—vibrancy, window chrome, Dock behavior, menus, error recovery, and in-app updates.
- [x] **Single Instance & Window State**: Wire `tauri-plugin-single-instance` (focus/restore existing window on second launch) and `tauri-plugin-window-state` (remember size, position, maximized state across connected displays; show window only after ready so it never flashes white).
- [x] **Per-OS Window Chrome & Vibrancy (`lib.rs`)**:
  - *macOS*: Enable `transparent(true)`, `titleBarStyle: Overlay`, `hiddenTitle: true`, `trafficLightPosition: (16, 19)`, `Effect::Sidebar` vibrancy (`FollowsWindowActiveState`), and forced `Theme::Dark`.
  - *Windows*: Enable `decorations(false)` + dark `Effect::Mica` on Windows 11 with a clean `#0b2457` fallback on Windows 10, and add `platform-win32`-only HTML min/max/close buttons in `.win-controls`.
  - *Linux*: Keep opaque, natively decorated window with `#0b2457` background and `Theme::Dark`.
- [x] **macOS Dock Lifecycle**: Intercept `RunEvent::ExitRequested` (`prevent_exit()` unless explicit Quit) and `RunEvent::Reopen` so closing the window on macOS keeps the app and `fistbump-core` alive in the Dock, matching Electron.
- [x] **Navigation & New-Window Guards**: Block all WebView navigation/popups outside `tauri://localhost` / `http://tauri.localhost` and route `https:` URLs to the system browser via `tauri-plugin-opener`.
- [x] **Menus & Context Menu**: Port `app/main/menus.js` using `tauri::menu` (`src-tauri/src/menus.rs`: macOS/Win/Linux app menu, dev-only Reload/DevTools, `Cmd/Ctrl+,` -> `app:open-settings`, GitHub Help link) and preserve the `https:` link right-click menu (`Open in Browser` / `Copy Link`) in `tauri-bridge.js`.
- [x] **Startup Error Screen**: Port `app/main/error-screen.js` in `lib.rs` to open a secondary `WebviewWindow` (`error.html`, `720x600`) when `fistbump-core` fails to start or exceeds 3 crashes/min, wired to `error:details`, `error:copyDetails`, `app:show-data-folder`, and `error:quit`.
- [x] **Auto-Updater (`tauri-plugin-updater` + `src-tauri/src/updates.rs`)**: Add `tauri-plugin-updater` + `tauri-plugin-process`, add `updater:default` + `process:default` capabilities, and bridge `app:check-updates`, `app:download-update`, `app:cancel-update`, `app:install-update` (stopping `fistbump-core` before `update.install()` + `app.restart()`), and `update:progress` events. (Embed `plugins.updater.pubkey` + `endpoints` once you generate your `minisign` keypair in Phase 5.)
- [x] **Phase 4 Gate**: Run `cargo test` and `npm run dev:tauri` to verify macOS dark overlay title bar + Sidebar vibrancy + traffic lights at `(16, 19)`, single-instance lock, macOS close-and-Dock-reopen, `Cmd/Ctrl+,`, right-click link menu, and broken-backend error screen.

### Phase 5: Packaging, CI/CD, Testing, Screenshots & Electron Cutover
**Goal**: Ship release installers with identical naming/UX, migrate tests and automation, and remove Electron.
- [x] **Resource Bundling & Installers**: Added `scripts/stage-resources.js` to stage `bin/<os>-<arch>/` (`fistbump-core`, `llama-server`, and all shared libraries) into `src-tauri/resources/bin/`, wired `bundle.resources`, `bundle.createUpdaterArtifacts: true`, `build/background.tiff` for macOS DMG, and `build/installerSidebar.bmp` + `installMode: "currentUser"` for Windows NSIS in `src-tauri/tauri.conf.json`.
- [x] **CI/CD (`.github/workflows/{app,build,e2e}.yml`)**:
  - Updated `app.yml` and `build.yml` to run Go tests, `npm test`, and `npm run test:rust` (`cargo test`).
  - Replaced `electron-builder` in `build.yml` with `tauri-apps/tauri-action@v0` on `macos-latest` (`aarch64-apple-darwin` + `x86_64-apple-darwin`), `windows-latest` (`x86_64-pc-windows-msvc`), and `ubuntu-22.04` (`x86_64-unknown-linux-gnu`), publishing signed updater artifacts (`.sig`) and `latest.json` alongside installers.
  - Updated `e2e.yml` to run Playwright Chromium smoke tests against `fistbump-core`.
- [x] **Tests & Screenshots**:
  - Updated `app/tests/renderer-wiring.test.js` to validate against `app/renderer/js/tauri-bridge.js` and removed obsolete Electron main-process JS tests (now covered by `cargo test`).
  - Added `e2e/harness.js` and updated `e2e/fixtures.js`, `e2e/smoke.spec.js`, and `scripts/screenshots.js` to run against Playwright Chromium + the real `fistbump-core` binary.
- [x] **Docs**: Updated `README.md` (Tauri v2 stack, Rust prerequisite, Ubuntu 22.04+ Linux baseline, `src-tauri/` layout).
- [x] **Cutover & Cleanup**: Switched `npm run dev` / `npm run dist*` to Tauri in `package.json`, and removed `electron`, `electron-builder`, `electron-builder.yml`, `app/main/`, `app/preload/`, and `scripts/adhoc-sign.js`.
- [x] **Phase 5 Gate**: Run `npm install`, `npm test`, `npm run test:rust`, and `npm run dev` locally, and populate `plugins.updater.pubkey` in `src-tauri/tauri.conf.json` (set `TAURI_SIGNING_PRIVATE_KEY` and optional `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` in GitHub Actions secrets before tagging a release).

# Release checklist

Run this on **every platform** before the release is published. The draft release is built by CI, so the installers you test are the ones people will download.

Fill in the numbers on the size line and put the checklist in the release notes.

## Before you start

- [ ] The version in `package.json`, `src-tauri/tauri.conf.json`, `src-tauri/Cargo.toml`, and both lockfiles match the release version.
- [ ] The backend reports the same version. The build passes it with `-ldflags "-X main.version=<version>"`, so `GET /v1/health` answers with the release version. The default in `backend/cmd/fistbump-core/main.go` stays `"dev"`; only the build sets it.
- [ ] The matching version tag is pushed after the release PR merges.
- [ ] CI has finished and the draft release exists on the [releases page](https://github.com/tlmcguire/fistbump/releases), with a Windows `.exe`, a macOS `.dmg` for Apple silicon, a macOS `.dmg` for Intel and a Linux `.AppImage`.

## On each platform

Do this on Windows 10 and 11, macOS 12 or later (on both an Apple silicon and an Intel Mac, using the matching `.dmg`) and Linux (Ubuntu 22.04 or later, Fedora 36 or later).

Use a **fresh machine or a new user account**, so nothing is left over from an earlier build.

### Install

- [ ] The app installs from the draft release without errors.
- [ ] The unsigned-app warning appears as the user guide describes, and the steps in it work:
  - [ ] Windows: **More info**, then **Run anyway**.
  - [ ] macOS: right-click the app, **Open**, then **Open** in the dialog.
- [ ] The app opens and the window appears.

### The core workflow, with the rules only

Leave **Who writes suggestions** on **Built-in rules only** in **Settings**, then work through the whole flow once.

- [ ] Build the master resume: fill in the details, add a job and add a school, then save it.
- [ ] Save a job: find a posting with **Find jobs**, or paste one, and click **Save job**.
- [ ] See the skill gap: the posting shows a fit percentage and the missing skills.
- [ ] Tailor and review: **Tailor**, **Generate suggestions**, then accept, reject and edit at least one suggestion each. Nothing changes in the resume until a suggestion is accepted.
- [ ] Build the tailored resume and look at both the **Changes** and **PDF preview** tabs.
- [ ] Export a PDF: **Export PDF** saves a file that opens and shows the tailored resume.
- [ ] Track the application: **Track** adds it to the Tracker, and changing the status saves.

### The core workflow, with a local model

- [ ] In **Settings**, under **Local model**, download the small model (about 1.1 GB, needs 4 GB of RAM).
- [ ] Choose **Local model only**, or leave it on **Automatic**.
- [ ] Work through the core workflow again: build a resume, save a job, read the skill gap, tailor, review, export a PDF and track the application.
- [ ] The suggestions are written by the model, and the engine chip says so. The model starts on the first request and stops after a few minutes of no use.
- [ ] The larger model (about 2.0 GB, needs 8 GB of RAM) downloads too, if the machine has the memory for it.

### The backend recovers from a crash

- [ ] With the app open and a job saved, find `fistbump-core` in the process list and end it.
- [ ] The app restarts the backend, the views reload, and the app keeps working: a saved job still opens, and a new suggestion can still be made.
- [ ] No error screen appears.

### Storage

- [ ] **Settings**, then **Storage**, shows the sizes: the database, the models and the search cache.
- [ ] The data folder path is shown and the button opens it.
- [ ] **Preview** lists what a cleanup would remove. **Clean up** then removes exactly what the preview listed, and the sizes go down by that much.

### Size

Record the numbers. NFR-5 asks for an install under **250 MB**, not counting model weights.

- [ ] Installer size: ______
- [ ] Installed size: ______
- [ ] Share of the Tauri host: ______
- [ ] Share of `fistbump-core`: ______
- [ ] Share of `llama-server`: ______

Record the same numbers for each platform.

### Uninstall

- [ ] The app uninstalls: Windows through **Installed apps**, macOS by moving the app to the **Trash**, Linux by deleting the `.AppImage`.
- [ ] The data folder is gone after the uninstall: `~/Library/Application Support/fistbump` on macOS, `%APPDATA%\fistbump` on Windows, `~/.config/fistbump` on Linux.

## 0.6.0 upgrade checks

- [ ] Upgrade from 0.5.x with a saved remote key. Re-enter the key once, restart, and verify it restores for that provider. Switch to a different origin with a blank key and verify a restart does not restore the previous provider’s key.
- [ ] Open a Tracker CSV containing a formula-like title in a spreadsheet and confirm it displays as text.
- [ ] Import a private HTTPS calendar feed and open an exported event in the OS calendar.
- [ ] Run the community-data verifier and relay tests. Deploy the Worker separately to ship relay changes.

## After every platform has passed

- [ ] Every box above is ticked on every platform.
- [ ] The size numbers are in the release notes.
- [ ] The release notes link to [docs/user-guide.md](../docs/user-guide.md).
- [ ] Open the draft release on the [releases page](https://github.com/tlmcguire/fistbump/releases) and publish it.
- [ ] Download one installer from the published release and check it installs, so the published files are the ones that were tested.

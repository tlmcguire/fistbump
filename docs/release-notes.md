![Fistbump](https://raw.githubusercontent.com/tlmcguire/fistbump/main/docs/images/github-banner.png)

# Fistbump v0.3.0

Fistbump is a local-first job application assistant: your resume, the postings, the skills they ask
for that you do not have yet, a tailored resume you can export as a PDF, and a tracker for the
applications you sent. Your data stays on your computer unless you choose a remote AI provider.

## What's new in 0.3.0

Fistbump has moved from Electron to a native **Tauri v2** Rust shell. Everything works the same way
and your existing resume, jobs, applications and local models carry over, while the app itself is
more than half the memory and under half the download size.

- **Less than half the memory.** Idle RAM on macOS (Apple silicon) dropped from **423.4 MB** to
  **198.1 MB** (-53%), leaving more room for the local AI model.
- **62% smaller on disk, 63% smaller download.** The macOS Apple silicon `.app` bundle dropped from
  **429 MB** to **161 MB**, and the `.dmg` installer dropped from **180 MB** to **66 MB**. The
  250 MB NFR-5 install-size target is now met with room to spare.
- **Consistent PDF previews on every OS.** Resume and tailored PDF previews now render locally with
  PDF.js onto a canvas, removing OS PDF viewer quirks and toolbars on macOS, Windows and Linux.
- **Native look and polish.** macOS uses a dark overlay title bar with Sidebar vibrancy and stays
  open in the Dock when you close the window; Windows uses frameless Mica with a clean Windows 10
  fallback; date pickers in Master resume work cleanly on macOS and Linux WebKit; and the Tailor
  loading animation keeps full transparency.
- **Signed in-place updates.** Updates from 0.3.0 onward are cryptographically verified with
  minisign and installed in place.

## In 0.2.4

You can now tailor a resume without asking for suggestions first, and suggestions keep your bullets.

- **Edit it myself.** A new button in Tailor copies your resume for the chosen job and opens it straight in
  the **Edit resume** tab, with no round of suggestions first. Start from the master resume or a saved
  version with the "Based on" picker. Each copy appears as a **Your edit** chip beside the rounds, and its
  small x deletes it, as it does for a round.
- **Bullets stay bullets.** A suggestion could drop the `- ` markers of the lines it rewrote, or delete a whole
  line. Each rewritten line now keeps the marker its original had, and a suggestion that changes the number
  of lines is set aside in favor of the built-in rules.
- **No stray backslashes.** The Changes view and the suggestion cards showed `\-` where a line began with a
  dash. They now show the line as you wrote it.

## In 0.2.3

Tailoring is now yours to finish, and the skill match shows what a job would like as well as what it needs.

- **Edit the tailored resume yourself.** A new **Edit resume** tab in Tailor holds the whole resume as text.
  Change anything, save, and the Changes view and the PDF follow what you wrote. Your edit never touches
  the resume you started from, so the comparison still shows what changed.
- **Required and nice-to-have skills, side by side.** Search results, saved jobs and the Tailor sidebar
  list the skills a job requires and the ones it would like, each colored by where you stand. The long
  per-skill breakdown folds away under Details. Tailor also says how many nice-to-have skills are on
  your resume.
- **"Is a plus" now means nice to have.** A line such as "Kafka is a plus" or "Terraform preferred" counts
  as nice to have even when it sits under Requirements, so it no longer drags down your required match.
- **Suggestions follow the resume's format.** The model is told to write plain text, keep bullets as they
  are and add no headings, and a suggestion that comes back with Markdown symbols is dropped in favor of
  the built-in rules.
- **A calmer Tailor view.** Switching to a different job, or opening one from the recommended list, no
  longer flashes the previous page or fades several parts in on top of each other.
- **Updates show their progress where you click.** Choosing **Update and restart** in the popup keeps you
  where you are and shows the download in the popup, with Cancel, instead of jumping to Settings.
- **Industry search is easier to find.** The empty search page says an industry alone is enough and has a
  **Browse by industry** button. The loading message now says that boards read in the last 6 hours load
  from this computer and the rest come from Greenhouse.

## In 0.2.0

The biggest release yet. Fistbump now tells you whether a job is worth it, not just whether you fit it.

### Is this job worth it?

- **Pay, benefits and cost of living, for every job.** Open any posting, saved or not, and a new panel
  answers three questions. Is the pay competitive for this role, level and place? Are the benefits
  good? Can you live well there on this pay? Each answer comes with a plain verdict and the numbers
  behind it. The figures are estimates from averages built into the app, for US locations, and nothing
  is sent anywhere to get them.
- **Your real take-home pay.** Federal, state and local income tax, Social Security and Medicare come
  off first, then rent and a modest monthly budget for the city, so the money left over is what you
  would actually keep. A table shows the pay per year, month, paycheck, week, day and hour, before
  and after tax.
- **Remote jobs use where you live.** Set your home location in your profile and remote postings are
  measured against your own city.
- **Benefits are read, never guessed.** Health, dental and vision coverage, retirement matching, paid
  time off, parental leave, equity and more are counted only when the posting offers them. When a
  posting mentions benefits but does not spell them out, it says so and rates nothing.
- **Good, fair or bad at a glance.** Every search result shows a short line of signals: your fit,
  then pay, benefits and cost of living.

### A smarter search

- **New sorts:** best overall, best fit, highest pay, best benefits and best cost of living.
- **New filters** for pay against the market, benefits and cost of living, with a switch to turn the
  ratings off.
- **See the filters a search used.** Each one shows as a chip under the results, and its x takes it off
  and searches again.
- **Choose what shows under each result:** fit, missing skills, pay range, each rating, the benefits
  listed, the money left each month, a description preview, the date, other locations and the
  company link.
- **Tidier filters.** The filters now sit in a few even rows instead of a long panel.
- **Shorter lists of places.** A posting open in 40 states reads "Remote in 40 places", not a paragraph.
- **More boards and six new industries:** robotics and drones, space, crypto and Web3, HR and workforce
  tech, fashion and beauty, and sports, fitness and wellness. Boards that no longer exist were removed.

### Jobs that fit you, on Home

- **Matches on Home.** With target positions in your profile, Home suggests open roles that fit them,
  your preferred work mode and your location, best fit to your resume first.
- **Home adds no load.** Matches reuse what your last search loaded, so showing Home never sends a
  request to every board.

### Fixes and polish

- **A saved job is never offered for saving again.** Search results and links show a Saved tag and a
  **View saved job** button that opens it in Saved jobs.
- **No more mystery failures when Greenhouse is busy.** Searches now pace themselves and explain any
  delay. See "Searching politely" below.
- **Results no longer come back empty after a slow listing.** Reading the descriptions now has its own
  time limit.
- **Clearer high-contrast mode.** With Increase contrast on, the filters end in a clear edge.
- **Roomier Settings button.** Its highlight no longer hugs the icon and the word.

### Searching politely

Fistbump finds jobs by asking Greenhouse, the job board many companies use. Greenhouse only answers so
many questions at a time, so this release makes Fistbump a good guest:

- **It asks a few things at a time, not everything at once.** Even with several searches running, only a
  handful of requests are out at any moment.
- **It goes fast when you have been away, and eases off when you are busy.** Your first search after a
  break runs at full speed. If you search again and again, Fistbump paces itself instead of being turned away.
- **It remembers what it has already seen.** Job lists from the last few hours are reused, and Home's
  suggestions only use what your searches already found, so they never add to the load.
- **It never asks twice for the same thing.** If two searches need the same company's jobs at the same
  moment, one request serves both.
- **If Greenhouse asks it to slow down, it listens.** Fistbump waits a little, shows what it already has,
  and tells you plainly to try again shortly. On Home, **Search again** counts down until a new try
  will work.

## Earlier in 0.1.x

- **The Tailor tab shows the full skill match.** Beside your suggestions it now lists every skill the job
  asks for, required and preferred, colored as on your resume, likely, missing or not mentioned, with the
  same breakdown and evidence as a saved job.
- **Close works on the whole button.** On an expanded PDF the top of the Close button sat under the window's
  drag area and did not respond; the full button is clickable now.
- **Research, projects and activities are imported.** Entries under headings such as Research or
  Extracurricular now come in as experience, named by their section when they list no organization,
  instead of being left as warnings. Remove any that are not a job.
- **Spacing in the import review.** The Add education and Add experience rows no longer touch the card above.
- **Updates install themselves.** **Update and restart** downloads the new version, puts it in place of
  the old one and opens Fistbump again; your data is kept. When Fistbump opens it tells you about a new
  version, with **Not now** and a way to skip that version. On a Mac, keep the app in Applications so it
  can replace itself. Updating from 0.1.9 or earlier still goes the old way once.
- **A searching magnifier.** The search icon sweeps as if looking around.
- **The skill match shows every skill a job asks for.** Each one appears in a colored row: on your
  resume, likely (a related skill is on your resume, shown with the one that suggests it), missing,
  or a soft skill your resume does not name. The breakdown with your evidence follows below.
- **EKS counts as Kubernetes.** EKS, GKE and AKS (Kubernetes on AWS, Google Cloud and Azure) are known
  skills, likely for anyone who lists Kubernetes, and EKS on a resume counts as AWS.
- **Clearer links for a job.** Open original posting opens the job on the company's own site when
  there is one. Company website opens the company's home page, now known for most companies, so you
  can see what it does.
- **Animated icons.** Each icon acts out what it stands for when you point at it, focus it or open
  its view: the wand casts, the Settings sliders slide, the tracker ticks itself off. Reduce motion
  turns this off.
- **Updates download from Settings.** When a newer version is available, **Download update** saves
  the installer for your computer into your Downloads folder, with progress and a Cancel button, and
  **Install** opens it: the installer on Windows, the disk image on macOS, the file's folder on
  Linux. Fistbump does not install the update by itself. Before 0.1.10 the button only opened the
  releases page, so this first update from 0.1.7 or earlier is still a download from that page.
- **The update dot no longer covers the Settings label.** When a newer version is available, the
  dot now sits after the word "Settings" instead of over its last letter.
- **No title bar or menu bar on Windows.** The window no longer has a title bar, and Alt does not
  bring the menu bar back. The minimize, maximize and close buttons stay, at the top right, and the
  window is moved by dragging the toolbar. Menu shortcuts such as Ctrl+, for Settings still work.
- **Better job search matching.** A city now matches as a whole name and is not confused with a
  city of the same name elsewhere: "Columbia, SC" no longer finds British Columbia or Columbia, MD.
  A posting in your city ranks above one that only shares your state.
- **A fit that tells gaps from skills you likely have.** A required skill you do not list but that
  sits next to one you do (PostgreSQL for MySQL, Excel for Tableau) shows as "Likely", earns half
  credit and is not counted as missing. Each posting shows Likely and Gaps chips, and the Strong
  and Good fit thresholds are more lenient.
- **Activities are no longer imported as jobs.** Projects, research, volunteering and activities
  on a resume are reported as text to add by hand instead of becoming work experience.
- **More boards and two new industries.** The Greenhouse list grows from 630 to 947 boards, with
  Marketing and advertising and Automotive and mobility added as industries.
- **Tailoring saves its suggestions again.** On a database that an earlier test build had moved
  ahead, a round written by the local model or a remote provider could not be saved, and the app
  showed "An unexpected error occurred". The app now repairs the database when it opens; your data
  is kept.
- **Clearer error messages.** An unexpected error now says to try again and restart the app, and
  a round whose suggestions could not be saved says so, with the steps to take.
- **The local AI starts on macOS.** In the earlier macOS builds the bundled `llama-server` could not
  load its libraries, so using the local model failed with "llama-server exited while starting".
  The builds now include them. If you use the local model on a Mac, update to this version.
- **A missing library now fails the build.** After the llama server is fetched, the build checks
  that every library the macOS binary needs is in the bundle, and runs the server when it can, so
  this cannot reach a release again.
- Earlier fixes since 0.1.0: the macOS app is ad-hoc signed so the DMG opens.

![Tailoring a resume](https://raw.githubusercontent.com/tlmcguire/fistbump/main/docs/images/tailor.png)

## What you can do

- **Keep one master resume.** Enter it by hand or import a file, see it as a live PDF, and save
  named versions.
- **Collect jobs.** Search Greenhouse boards, open a link, or paste a posting. Postings are parsed
  into company, title, location, pay and the skills they ask for.
- **See the skill gap.** Required and preferred skills your resume does not show yet, with a match
  score. Matching is lenient: a skill under another name counts, and soft skills you do not name
  are marked "Not mentioned" rather than missing. The search can run without your resume.
- **Tailor with review.** Rounds of suggestions, written by built-in rules, a local model or a
  remote provider. Nothing changes until you accept, reject or edit each one, and the finished
  resume can be edited by hand. A failed round says
  what went wrong and what to try. Rounds can be deleted with the x on their chip.
- **Export.** Build the tailored resume and save a PDF or Markdown.
- **Track applications.** Stages from Saved to Offered, dates, next steps and notes, with follow-ups
  on the Home view.
- **Stay in control of storage.** Settings shows what the app stores, previews a cleanup before
  anything is removed, and old untracked jobs and the Trash are cleared on their own.
- **Recover from a crash.** If the backend stops, the app restarts it. If it cannot start, an error
  screen shows what happened with a Copy details button.
- Four languages: English, Spanish, French and German.

## Installers

| Platform | File | Notes |
|----------|------|-------|
| Windows 10 and 11 | `FistBump-0.3.0-Windows-x64.exe` | NSIS, x64, unsigned |
| macOS 12 and later, Apple silicon | `FistBump-0.3.0-macOS-arm64.dmg` | unsigned |
| macOS 12 and later, Intel | `FistBump-0.3.0-macOS-x64.dmg` | unsigned |
| Ubuntu 22.04 and later, Fedora 36 and later | `FistBump-0.3.0-Linux-x86_64.AppImage` | x64, unsigned (`libwebkit2gtk-4.1`) |

The builds are not signed or notarized, so the first launch is stopped by the system: on macOS
right-click the app and choose Open, on Windows choose "More info" and then "Run anyway". The
user guide ([user-guide.md](user-guide.md)) walks through it.

## Install size against NFR-5

NFR-5 asks for an install under 250 MB, without model weights. Moving from Electron to Tauri v2
closed the gap without dropping any features. Measured on macOS Apple silicon:

| Metric | Electron (0.2.x) | Tauri v2 (0.3.0) | Change |
|---|---|---|---|
| Installer (`.dmg`) | 180 MB | **66 MB** | **-63%** |
| Installed app bundle (`.app`) | 429 MB | **161 MB** | **-62%** |
| Shell overhead (excl. ~126 MB `bin/`) | ~303 MB | **~35 MB** | **-88%** |
| Idle RAM (RSS, full process tree) | 423.4 MB | **198.1 MB** | **-53%** |

**NFR-5 is now met** (161 MB installed vs. the 250 MB limit, with `fistbump-core` and `llama-server`
still bundled inside the app).

## Upgrading and data

Because 0.3.0 switches from Electron to Tauri, download this installer once from the releases page
and install it over the earlier version (future Tauri-to-Tauri updates install in place).

Your SQLite database, master resume, saved jobs, applications and downloaded local models stay in
the same data folder and carry over untouched. If you previously saved an optional remote AI API
key in Settings, re-enter it once after upgrading (the new OS keychain store cannot read Electron's
older encrypted key file).

## Known limits

- Greenhouse only: the job board support covers companies that post on a Greenhouse board.
- Skill matching is strongest for technical roles. It compares the posting's skills with the
  resume's, so roles with a lot of soft skills get less help. It is deliberately lenient: a skill
  you have under another name counts (REST covers API), and a soft skill your resume does not name
  is listed as "Not mentioned" rather than missing. The search can also be run without the resume
  at all, by unticking **Match with my resume** in the filters.
- The builds are unsigned, so the first launch needs the extra click described above.
- The local model needs about 4 GB of free memory for the default one, and the model file itself
  is about 1 GB on disk, outside the install size above.

Full documentation: the [user guide](https://github.com/tlmcguire/fistbump/blob/main/docs/user-guide.md).

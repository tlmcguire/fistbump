# Fistbump user guide

Fistbump is a job application assistant that runs on your own computer. You keep one master resume, and for each job you apply to, Fistbump tells you which skills the posting asks for that your resume does not mention yet, suggests rewrites of the parts that matter, and lets you build a tailored resume you export as a PDF.

Everything stays on your computer. Nothing is uploaded unless you turn on a remote AI provider yourself.

## What Fistbump does

- **One master resume.** Fill in the form, or import a `.pdf`, `.docx`, `.odt`, `.md`, or `.txt` resume you already have.
- **Find a job.** Search Greenhouse, Lever, and Ashby boards (plus custom watchlist boards), paste a job link into the search bar, or paste a posting from any site.
- **See the skill gap.** For every posting, a percentage of the required skills your resume covers, the ones you are missing, and pay/cost-of-living analysis.
- **Tailor the resume.** Fistbump writes suggested changes for that job. You accept, reject, or edit each one before anything changes.
- **Export PDF or DOCX.** Save the tailored resume in 5 PDF styles or as a `.docx` file.
- **Track the application.** Keep every application in one list, compare offers side by side, export CSV or `.ics` reminders, and bulk-manage entries.
- **Make it yours.** Pick from 23 style and seasonal themes (including automatic monthly themes) with full accessibility support.

## Install

The installers are on the releases page: [github.com/tlmcguire/fistbump/releases](https://github.com/tlmcguire/fistbump/releases).

The builds are not signed or notarized, so your computer will warn you the first time. That warning is expected for this release.

### Windows

1. Download the `.exe` installer from the releases page.
2. Open it. Windows SmartScreen shows a blue window saying the app is from an unknown publisher.
3. Click **More info**, then **Run anyway**.
4. Follow the installer. The app installs for your user account, under `AppData\Local\Programs\FistBump`.

### macOS

There are two files. Download the **arm64** one if you have an Apple silicon Mac (M1 or newer, 2020 or later). Download the **x64** one if you have an Intel Mac. You can check by clicking the Apple menu, then **About This Mac**.

1. Open the `.dmg` file.
2. Drag **FistBump** into your **Applications** folder.
3. Open it from **Applications**. The first launch is blocked: macOS says the app cannot be opened because it is from an unidentified developer.
4. Close the message. In **Finder**, right-click (or Control-click) **FistBump** in Applications and choose **Open**.
5. Click **Open** in the dialog that appears. The app opens. You only do this the first time.

### Linux

1. Download the `.AppImage` file from the releases page.
2. Make it executable: right-click it, choose **Properties**, then tick **Allow executing file as program**.
3. Double-click it to run it. If your system opens it with a text editor instead, choose **Run as a Program** from the right-click menu.

If your Linux is missing the libraries the app needs, the message will say which one. Installing `libwebkit2gtk-4.1` (Ubuntu 22.04+, Fedora 36+) usually fixes it.

## First run

Fistbump opens on **Home**. Everything is reached from the bar at the bottom of the window: **Home**, **Jobs**, **Resume**, the search box, and the gear for **Settings**. Press **⌘K** (or **Ctrl+K**) to jump to the search box from anywhere.

Home shows what needs you today, your next seven days (follow-ups, interviews and closing dates), every job by stage, and new postings that match your target roles. Start with your resume.

### 1. Build your master resume

1. Click **Resume** in the bottom bar. The **Ready to tailor?** panel lists what is still missing; click **Add** beside an item to go straight to its field.
2. Fill in your details: name, email, phone, where you live, the kind of work you are looking for.
3. Add each job you have had, with what you did and what came of it.
4. Add your education. Under **Degree layout**, choose **One line per group** (the default) or **Separate lines**. Only degrees from the same known institution with compatible graduation dates and GPAs can share a line.
5. Click **Save as version** if you want a copy to come back to. You can also start from a resume you already have: click **Import file** and choose a `.pdf`, `.docx`, `.odt`, `.md`, or `.txt` file, or click **Paste text** and paste the text of a resume.

Your master resume is the starting point for every tailored version. Education follows work experience by default. If you previously changed the order, choose **Restore default order** under **Custom sections & section order**.

### 2. Find a job, or paste one

Everything starts in the search box at the bottom of the window:

- **Type a search** to look through Greenhouse, Lever, and Ashby job boards (plus any company boards on your watchlist in **Settings**). The results open with the full search form, where you can add a location and filters.
- **Paste a job link** from Greenhouse, Lever, or Ashby to open and analyze it.
- **Paste or drop the text of a posting** from any site, and it opens in **Paste a job**, ready to preview and save.

In the search results, The **Filters** button opens company, ATS source, industry, work mode, date posted, pay, fit with your resume, and skills you are missing. Untick **Match with my resume** to search without your resume.
Saved jobs are listed under **Jobs**. The list shows up to 25 jobs per page and puts jobs that need your attention first. The tabs filter by stage (Saved, Tailored, Applied, Interview, Offer, and Closed for archived, trashed and rejected jobs). Select several jobs to move them to a stage, archive them or move them to the Trash.

Use **Filters → Location & timing → Distance radius** to find postings within a chosen number of miles. The **Location** field sets the center. Leave it blank to use the location in your resume profile. Enter a U.S. city and state, such as Charleston, SC. Distance is straight-line between Census place points. Postings with no recognized location are excluded while the filter is on.

After a successful search, use the small bookmark inside the keyword field to save it. Select any entry in **Saved searches** under the search bar to run it again. The bar shows five saved searches by default. Use **Show all** to see the rest. **History** shows recent searches.

**Paste a job** can fetch a direct Greenhouse, Lever, or Ashby job link. You can also paste your own copy of the posting text alongside the link. For other sites, paste the posting text and review or enter the job title, company, location, work mode, employment type, deadline, pay, and company website. Preview before saving or sharing. Use **Back to Search** or **Esc** to leave and return to your draft later.

### 3. Read the skill gap

Open a saved job from **Jobs** and its page shows where it stands, the one next step, and how well your resume covers it: a percentage with a word such as **Strong fit**, **Partial fit** or **Weak fit**, and the list of required skills you are missing. Below that it lists the technical and soft skills the posting asks for, marking each as matched or missing.

This analysis is done with plain rules on your computer. It never uses AI, so it is instant and it never invents anything.

The comparison is deliberately forgiving, because a resume is not a list of every skill you have. A skill you have under another name still counts: a resume that shows REST also covers API, and a resume that shows Google Analytics covers Analytics. A soft skill your resume does not name ("attention to detail" is the usual one) is listed as **Not mentioned** instead of missing, because resumes rarely list them and they are no evidence against you. The percentage counts the technical skills only.

### 4. Tailor the resume

1. Open the saved job and click **Tailor resume**.
2. Pick the base resume if you want one other than your master resume, and click **Generate suggestions**.
3. Your resume appears as a page with each suggestion in place: added words highlighted, removed words struck through, and a note in the margin saying why. **Accept** it, **Reject** it, or **Edit** it to write your own wording (keys **A**, **R**, **E**; **J** and **K** move between changes). Nothing changes in your resume until you accept a suggestion.
4. When you have decided on all of them, click **Build tailored resume**.
5. The **Changes** tab shows the differences against your base resume, and the **PDF preview** tab shows the PDF exactly as it will be saved.

### 5. Export the PDF

Click **Export PDF**. The file is saved to your Documents folder, and a link appears to show you where it went. If the PDF cannot be made, Fistbump offers to save Markdown instead.

### 6. Track the application

On the job's page, change its **Stage** (Applied, Interviewing, and so on), set a follow-up date, and keep notes; it then appears in Home's next seven days. The full **Tracker** table, with CSV export and offer comparison, is linked from the bottom of **Jobs** and from each job's page. Every application is a row with the company, the position, a status, the date you applied, and the next step. Change a status from the dropdown and it saves straight away. You can archive an application to hide it while keeping it, or move a job to the Trash.

## AI suggestions

### By default: built-in rules only

Out of the box, suggestions are written by Fistbump's own rules, on your computer, with no model downloaded and nothing sent anywhere. The rules reword and reorder what is already true in your resume. They never add a claim you did not make, and they never write a skill you have not used.

In **Settings**, under **AI suggestions**, **Who writes suggestions** shows which engine is in use. Choose **Built-in rules only** to keep it that way.

### Downloading a local model

You can download a small model that runs on your own computer. In **Settings**, under **AI suggestions** and **Local model**, pick one and download it. The download comes from Hugging Face, so it needs a connection once.

| Model | Download size | Memory it needs |
|-------|---------------|-----------------|
| Qwen2.5 1.5B Instruct (Q4_K_M) | about 1.1 GB | 4 GB of RAM |
| Llama 3.2 3B Instruct (Q4_K_M) | about 2.0 GB | 8 GB of RAM |

The larger model usually writes better suggestions, but it needs a computer with more memory. Fistbump marks a model as too big for your computer if you do not have the memory it needs.

The model is not downloaded with the app, so it does not count towards the installed size. It is stored in the data folder and can be removed from **Settings**.

Once a model is installed, choose **Local model only**, or leave it on **Automatic** to use the model when it is there and the rules when it is not. The model starts the first time you ask for suggestions and stops again after a few minutes of no use.

### Adding a remote provider

This is optional and off until you set it up. In **Settings**, under **Remote provider (optional)**, enter the address of an OpenAI-compatible API, the model name, and the key, then click **Save provider**.

What it sends: when you ask for suggestions, Fistbump sends the text of your resume and the text of the job posting to that address, so the provider can write the suggestions. Nothing else is sent, and nothing is sent until you ask for suggestions.

The app saves the key in your system keychain, bound to the provider’s scheme, host, and port. When upgrading from 0.5.x, re-enter your remote key once under **Settings → Remote provider**. Fistbump never reads it back for display. If your computer cannot store the key safely, it is kept for the session only and you enter it again after a restart. Ollama or another provider running on your own computer needs no key at all.

Fistbump contacts public job boards for searches and background refreshes of previously searched boards. Model downloads, remote AI requests, private calendar lookups, bug reports, and community submissions also make network requests when you use those features. The [network call inventory](periodic-external-calls.md) covers scheduled checks.

## Where your data lives

Fistbump keeps everything in one folder:

| Platform | Folder |
|----------|--------|
| macOS | `~/Library/Application Support/fistbump` |
| Windows | `%APPDATA%\fistbump` |
| Linux | `~/.config/fistbump` |

In the app, **Settings**, then **Storage**, shows the folder path and a button to open it. The same section shows how much space the database, the downloaded models, the job search cache and the whole of Fistbump use. Under **Clean up** you can choose what to remove, look at what a preview says would go, and then clear it.

### Backing up

1. Quit Fistbump, so nothing is written while you copy.
2. Copy the whole data folder somewhere safe, such as an external drive or a cloud folder.

Copy the folder back into the same place to restore your resume, saved jobs, applications, and settings. Remote AI keys and private calendar addresses live separately in the system keychain.

### Uninstalling completely

1. Delete the app:
   - Windows: **Settings**, **Apps**, **Installed apps**, **Fistbump**, **Uninstall**.
   - macOS: drag **FistBump** from **Applications** to the **Trash**.
   - Linux: delete the `.AppImage` file.
2. Delete the data folder listed above, if you want the computer to keep nothing of Fistbump.

## Themes & accessibility

In **Settings**, under **Appearance & accessibility**, you can choose from 11 style themes, 12 monthly seasonal themes with custom offline SVG artwork, or **Auto (Current Month)** so the app automatically matches the current month. The rest mixes with any theme:

- **Day or night**: every theme comes in Night, Darker (the panels turn to dark paper with light ink, for dark rooms) and Day. **Match system** follows your computer's light and dark appearance as it changes.
- **Accent color**: the theme's own, or one of six for every theme.
- **Paper**: cream, white, or tinted with the accent.
- **Ink marks**: the hand-drawn underlines, circles and stars, or off.
- **Navigation bar**: at the bottom of the window or at the top.
- **Motion**: Full, Gentle (short fades only) or None.

**Text size**, **Increase contrast** and **Underline links** apply across every theme. The **View** menu has Day or Night, the navigation bar and text size too.

## The menu bar

On a Mac the menu bar follows the usual layout. **File** starts a new search (⌘N), pastes a job posting (⇧⌘N), imports a resume (⌘O) and exports your applications as a spreadsheet (⇧⌘E). **Edit** has Find (⌘F) for the search box on the page you are on. **View** has day or night, the navigation bar, text size and full screen. **Go** moves Back and Forward (⌘[ and ⌘]), to each place (⌘1 to ⌘6) and to the search box (⌘K). **Help** opens this guide, the keyboard shortcuts, and **Report an Issue**. On Windows and Linux the same menus are in the window, with Settings and Quit under File.

## Calendar

**Calendar** in the bar (⌘6) shows your plans by week or month: interviews, calls and follow-ups, plus each tracker follow-up date on its day. Click an empty hour, or **Plan an interview** (also on a job's page, under More actions), to add one: the job, the day and time, how long, where, and notes. If it runs into another plan, or into something on your own calendar, the form says so before you save, and the plan is marked **Overlaps** after.

Everything stays in Fistbump until you choose otherwise, in **Settings → Calendar**:

- **Add plans to**: the calendar app on this computer (Apple Calendar or Outlook, through an .ics file), Google Calendar, Outlook.com, or ask each time. Open any plan to add it there, or turn on **Add new plans to my calendar**.
- **Check for clashes with my calendar**: paste your calendar's private iCal address (Google Calendar: your calendar's settings, "Secret address in iCal format"; Outlook: Publish calendar; iCloud: Public calendar). Fistbump downloads the feed and uses its event times to detect clashes. It stores the address in your system keychain and checks again about every 15 minutes while the calendar is open. Feed URLs must use HTTPS or webcal, and redirects must stay on the same origin. **Forget my calendars** removes it.
- **Export .ics** on the calendar saves every plan from today on as one file to import anywhere.

## Report an issue

**Help → Report an Issue…** (or **Settings → About**) sends a problem, an idea or a question to the people who make Fistbump. You do not need an account. As you type, reports that look the same are shown; **That's mine** adds your note to one instead of filing another. **What will be sent** shows exactly what goes: no name, email, resume or jobs, and email addresses and phone numbers are removed. If the service cannot be reached, you can copy the report or open it on GitHub.

Search by company to include the expanded public Greenhouse catalog. Broad Greenhouse searches use the curated boards and your watchlist, plus shared boards if enabled.

## Shared job boards and listings

Two switches in **Settings → Job search**, both off until you turn them on:

- **Use boards other people found** adds job boards other Fistbump users shared, and that were checked, to your searches. The list is fetched about once a day.
- **Share the boards I add** sends the name of each board you add to your watchlist (like `lever:netflix`), nothing else. Each one is checked and reviewed before it reaches everyone.

To contribute a posting, open **Paste a job**, preview it, and select **Share with the Fistbump community**. You can also save it to Your jobs. Sharing is optional for each posting.

## Known limits

- **Built-in job search covers Greenhouse, Lever, and Ashby.** You can also add custom Greenhouse, Lever, or Ashby boards in **Settings → Company watchlist**. For employers using a different applicant tracking system, copy the posting text and use **Paste a job**.
- **Skill matching is strongest for technical roles.** The matching looks for skills and keywords in the posting's requirements. For roles that are described mostly in prose, the percentage and the missing list are less precise.
- **The builds are unsigned.** macOS and Windows both warn you the first time you open the app. Updates install themselves: when Fistbump opens it looks for a newer version and, if there is one, asks whether to **Update and restart**. Fistbump then downloads the installer for your computer, closes, puts the new version in place of the old one and opens again; your data is kept. **Not now** asks again next time, and **Don't remind me about this version** skips that version until a newer one comes out. **Settings**, then **Updates**, has the same **Update and restart** button and a switch to stop the question at startup. On a Mac the app must be in Applications (not run from the disk image) to replace itself; otherwise **Open installer** opens the downloaded disk image.

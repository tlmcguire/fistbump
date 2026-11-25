![Fistbump](https://raw.githubusercontent.com/tlmcguire/fistbump/main/docs/images/github-banner.png)

# Fistbump v0.4.0

Fistbump is a local-first job application assistant. Your data stays on your computer unless you choose a remote AI provider.

## What's new

- **Five PDF styles.** Classic, Modern, Minimal, Compact and Executive, with live previews in Master resume and Tailor.
- **Import DOCX and Markdown.** Resumes can come in as `.docx`, `.md`, `.pdf` or `.txt`. Hidden text in DOCX files is stripped and reported.
- **Better resume parsing.** Improved section headings, bullets, degrees, multi-line titles and date ranges.
- **Undated entries.** Mark projects, volunteer work or undated experience as "Date range not applicable".
- **Sharper tailoring suggestions.** Suggestions cite specific job requirements and skip no-op rewrites.
- **Sourced pay and cost data.** Every estimate cites its source, and the dataset refreshes in the app every 30 days.
- **Paste a link in Search.** Paste a Greenhouse job URL into the search bar to open and analyze it.

![Tailoring a resume](https://raw.githubusercontent.com/tlmcguire/fistbump/main/docs/images/tailor.png)

## Installers

| Platform | File |
|----------|------|
| Windows 10 and 11 | `FistBump-0.4.0-Windows-x64.exe` |
| macOS 12 and later, Apple silicon | `FistBump-0.4.0-macOS-arm64.dmg` |
| macOS 12 and later, Intel | `FistBump-0.4.0-macOS-x64.dmg` |
| Ubuntu 22.04 and later, Fedora 36 and later | `FistBump-0.4.0-Linux-x86_64.AppImage` |

The builds are unsigned. On macOS, right-click the app and choose Open. On Windows, choose "More info", then "Run anyway". The [user guide](https://github.com/tlmcguire/fistbump/blob/main/docs/user-guide.md) has the details.

## Known limits

- Greenhouse boards only.
- Skill matching is strongest for technical roles.
- The local model needs about 4 GB of free memory. Its file is about 1 GB on disk, outside the install size.

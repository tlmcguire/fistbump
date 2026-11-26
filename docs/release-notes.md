# Fistbump v0.5.1

A focused follow-up to v0.5.0 restoring color cues in Search and Listings, adding a Search source toggle for ATS badges, and letting the community crowdsource listings from sites like LinkedIn, Indeed, and Workday directly into Search.

## What's new

- **Green, yellow, and red colors restored in Search and Listings.** Fit scores, pay/benefits/cost-of-living badges, listing verdict pills, and skill match chips (`on your resume`, `likely`, `missing`) now display clear green, yellow, and red states across Night, Day, Darker, High Contrast, and all 23 themes. The saved Job page also shows the color-coded Benefits rating alongside pay and cost of living.
- **Toggle Source / ATS badges in Search.** Under **Filters → Show on each result card**, toggle **Source / ATS badge** on or off to control whether the ATS or community source label appears beneath each search result.
- **Add / submit listings from LinkedIn, Indeed, Workday, and more.** Click **Add listing** in Tracker or Your jobs (or **Submit listing** in Search, Settings, or the bottom bar) to open a guided submission form:
  - Paste a **Greenhouse, Lever, or Ashby** link and Fistbump fetches the listing through its built-in ATS connectors automatically — no need to copy-paste content.
  - Paste a link from **LinkedIn, Indeed, Workday, Glassdoor, or any other site** along with its description to save it to your jobs and/or share a scrubbed copy with the Fistbump community (no account needed).
- **Community listings in Search.** Search crowdsourced listings directly from **Filters → Source → Community listings**, or tick **Include community listings** to blend them alongside ATS results. Community results are flagged with their source badge (for example, `Community · LinkedIn`).

## Fixes

- Consolidated the Tailor proofing desk header into a single strip, removing repeated round and engine lines (`79bf303`).
- Pasting a non-ATS job link into the search bar or omnibox now hands off smoothly to the paste form with the URL pre-filled instead of failing with a fetch error.

## Installers

| Platform | File |
|----------|------|
| Windows 10 and 11 | `FistBump-0.5.1-Windows-x64.exe` |
| macOS 12 and later, Apple silicon | `FistBump-0.5.1-macOS-arm64.dmg` |
| macOS 12 and later, Intel | `FistBump-0.5.1-macOS-x64.dmg` |
| Ubuntu 22.04 and later, Fedora 36 and later | `FistBump-0.5.1-Linux-x86_64.AppImage` |

The builds are unsigned. On macOS, right-click the app and choose Open. On Windows, choose "More info", then "Run anyway".

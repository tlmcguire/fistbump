![Fistbump](https://raw.githubusercontent.com/tlmcguire/fistbump/main/docs/images/github-banner.png)

# Fistbump v0.6.0

- **More reliable resume imports.** Education follows work experience by default. Previews and exports consolidate only degrees from the same known institution with compatible dates and GPAs. You can also choose separate lines.
- **Faster search and posting workflows.** Filter by distance from your resume location or another U.S. city, run saved searches from the search bar, and browse saved jobs 25 at a time. Paste a job can fetch supported listings from a link and offers editable details for other sites before you save or optionally share.
- **Safer credentials and exports.** Remote AI keys are bound to their provider. The bundled local model server requires a per-launch key. Tracker CSV exports open formula-like fields as text, and calendar exports handle embedded line breaks safely.
- **Safer community publishing.** Expired listings are hidden, catalog fetches enforce origin and size limits, and the relay retries failed GitHub writes before removing submissions from its queue.

## Job-board coverage

The bundled catalog grew by **5,216 unique boards** since v0.5.1, a **389% increase**.

| Job board provider | v0.5.1 | v0.6.0 |
| --- | ---: | ---: |
| Greenhouse | 981 | 5,524 |
| Lever | 144 | 405 |
| Ashby | 216 | 628 |
| **Total** | **1,341** | **6,557** |

The public dataset contributes **4,300 boards** in about **364 KiB** of bundled data. Open-Apply discovery runs daily. StoreSignal and Common Crawl discovery runs monthly. Each run can verify up to **180 candidates** for review, and actual additions vary. While Fistbump is open, it refreshes eligible boards from past searches daily. Open job counts vary as listings are added and closed.

**Upgrading from 0.5.x?** Re-enter your remote AI key once in **Settings → Remote provider** so Fistbump can associate it with the correct provider. This applies to providers that use a key.

[User guide](https://github.com/tlmcguire/fistbump/blob/v0.6.0/docs/user-guide.md) · [Security policy](https://github.com/tlmcguire/fistbump/blob/v0.6.0/docs/security-policy.md) · [Previous releases](https://github.com/tlmcguire/fistbump/releases)

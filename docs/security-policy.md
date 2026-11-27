# Fistbump Security Policy

## Desktop trust boundaries

- The Go API binds to IPv4 loopback and requires a random per-launch bearer. The Tauri bridge disables HTTP proxies and redirects for that connection. Bundled `llama-server` uses a separate per-process key and disables its built-in web UI.
- The desktop webviews allow the packaged app origins. External HTTPS links open in the system browser. The content security policy limits renderer network access, and PDF.js runs with evaluation disabled.
- The OS keychain binds a remote AI key to the provider origin. Older credentials without an origin require one-time re-entry. Private calendar addresses also stay in the keychain. Calendar fetches stream at most 4 MiB and follow up to three same-origin HTTPS redirects.
- Application CSV exports escape formula-like fields as text. Calendar exports escape all newline forms, and native calendar handoff uses a private temporary file.

## Community data and automation

## Core Principles

1. **Human-in-the-loop publishing**: No community submission, public dataset refresh, or automated pruning run pushes directly to `main`. Every automated change opens or updates a Pull Request that must pass CI security checks and maintainer review.
2. **Defense in depth**: Validation runs at four independent boundaries: the Cloudflare Worker relay at ingestion time, the GitHub Actions fetch/prune scripts, the PR CI security gate, and the desktop Go backend at load time.
3. **Zero trust in external redirects**: Automated workflows never follow arbitrary cross-domain redirects, preventing domain-takeover or parked-domain hijacking if a third-party site is later sold.
4. **No dateless listings**: Every community listing carries a mandatory expiration timeframe so outdated postings cannot linger indefinitely.

---

## Layer 1: Edge Ingestion (`relay/src/index.js`)

All community submissions enter through the Cloudflare Worker relay (`POST /v1/boards` and `POST /v1/listings`).

### Rate Limiting & PII / Injection Scrubbing
- **Per-IP rate limiting**: Uses a secret-salted SHA-256 hash scoped to the hour. Cloudflare KV provides best-effort counters with a 3,700-second TTL. The relay caps POST bodies at 192 KiB. Limits: `30/hr` for boards, `20/hr` for listings, `6/hr` for bug reports.
- **Text sanitization (`scrub`)**: Every string field strips ASCII control characters, zero-width spaces, bidirectional override characters (`U+202A–U+202E`), invisible format characters (`U+2060–U+206F`, `U+FEFF`), and Unicode Tag characters (`U+E0000–U+E007F`, commonly used for hidden prompt injection). It also redacts email addresses, phone numbers, and local home directory paths (`/Users/...`, `/home/...`, `C:\Users\...`).

### Board Validation (`POST /v1/boards`)
1. **Strict token format**: Only `greenhouse`, `lever`, and `ashby` are accepted. Tokens must match `^[a-z0-9][a-z0-9_-]{0,63}$` and cannot be reserved slugs (`embed`, `jobs`, `apply`, `api`, `assets`, `login`, `www`, etc.).
2. **Typosquat rejection (`isLikelyTyposquat`)**: Any token of length ≥ 5 at Levenshtein edit distance `1` from an already-known board on the same ATS (e.g., `knownc0` vs. `knownco`) is rejected as `not_found`.
3. **Live active-posting check (`checkBoard`)**: The relay queries the official ATS API (`boards-api.greenhouse.io`, `api.lever.co`, or `api.ashbyhq.com`) with `redirect: "error"` and requires **at least one active open job posting** (`jobs.length > 0` or `data.length > 0`). Empty, placeholder, or fake boards that return `200 OK` with zero open jobs are rejected.

### Listing Validation & ATS Board Auto-Discovery (`POST /v1/listings`)
1. **Strict URL validation (`safeHttpsUrl`)**: `listing_url` and `company_url` must use `https:`. URLs with embedded credentials (`user:pass@`), raw IPv4/IPv6 addresses, `localhost`/`.local`/`.internal` hosts, punycode (`xn--`) homograph domains, or known URL shorteners (`bit.ly`, `tinyurl.com`, `t.co`, `lnkd.in`, etc.) are dropped.
2. **Exact domain suffix matching (`siteFromUrl`)**: Source classification (Workday, SmartRecruiters, LinkedIn, Indeed, Greenhouse, Lever, Ashby, etc.) uses strict hostname suffix matching (`host === domain || host.endsWith("." + domain)`) rather than substring matching, preventing spoofed hosts like `workday.phish.example` from impersonating a trusted ATS.
3. **Mandatory timeframe enforcement**:
   - The relay extracts explicit closing dates from `closes_at` or `raw_text` (e.g., *"Application deadline: ..."*, *"Apply by ..."*, *"Closes on ..."*). If `closes_at` has already elapsed, the submission is rejected with `400 listing_expired`.
   - Every accepted listing receives both `added` (`YYYY-MM-DD`) and a computed `expires_at` (`YYYY-MM-DD`):
     - Long-running / evergreen / rolling listings, identified from the title or listing content: at most `added + 120 days`.
     - Standard listings (e.g., LinkedIn, Indeed, Workday): at most `added + 45 days`.
     - An explicit `closes_at` may shorten, but never extend, the applicable timeframe. A caller-supplied `evergreen` boolean is ignored.
4. **Automatic ATS board discovery (`maybeQueueDiscoveredBoard`)**: If a user submits a listing whose `listing_url` or `company_url` points to a supported ATS (`boards.greenhouse.io`, `job-boards.greenhouse.io`, `boards.eu.greenhouse.io`, `job-boards.eu.greenhouse.io`, `jobs.lever.co`, `jobs.eu.lever.co`, or `jobs.ashbyhq.com`), the relay extracts `{ ats, token }` (including Greenhouse `/embed/job_board?for=<token>` links), runs the exact same typosquat and active-posting checks, and queues the newly discovered board into `data/community-boards.json` alongside the listing.

---

## Layer 2: Automated Fetchers & Domain-Takeover Defense

- **Public ATS dataset updater (`scripts/update-ats-boards.js`)**:
  - Contacts only the configured Hugging Face, Common Crawl, and GitHub raw hosts for StoreSignal, CDX, and Open-Apply inputs.
  - Uses `redirect: "manual"` and inspects every redirect hop (max 3 hops), requiring HTTPS and the original allowlisted origin on every hop.
  - Rejects `text/html` responses (catching parked or hijacked domains returning HTML instead of JSON/NDJSON), enforces a `5 MiB` response byte cap, strips invisible characters, blocks reserved slugs, and verifies at most 180 new candidates against official ATS APIs per run. Only successful probes enter the proposed catalog.
- **Cost & pay reference updater (`scripts/update-cost-data.js`)**:
  - Uses `redirect: "manual"` (max 3 hops) and enforces HTTPS-only plus same `baseDomain` on every redirect hop. If a reference domain is sold and redirects to a different domain, verification fails immediately.
- **Listing pruner (`scripts/prune-community-listings.js`)**:
  - Only probes allowlisted official ATS hosts (`boards.greenhouse.io`, `job-boards.greenhouse.io`, `boards.eu.greenhouse.io`, `job-boards.eu.greenhouse.io`, `jobs.lever.co`, `jobs.eu.lever.co`, `jobs.ashbyhq.com`) using `redirect: "manual"` and HTTPS/allowlist verification. It never probes LinkedIn, Indeed, Workday, or arbitrary third-party domains.

---

## Layer 3: Pull Request Security Gate (`.github/workflows/verify-community-pr.yml`)

Every Pull Request touching `data/community-boards.json`, `data/community-listings.json`, or `backend/internal/connectors/public_boards.json` triggers a `pull_request_target` workflow defined by the base revision. It runs the validator from that trusted revision against the checked-out PR data, without executing PR code, with minimal permissions (`contents: read`, `persist-credentials: false`, `timeout-minutes: 5`).

The validator reads bounded regular JSON files and rejects symlinks in catalog paths. It checks for the following:
- Unsupported ATS name, invalid/reserved token, or duplicate token.
- Edit-distance-1 typosquat in `community-boards.json` against the curated Greenhouse, Lever, or Ashby catalogs.
- Invisible, bidirectional override, or Unicode Tag characters in any text field.
- Non-HTTPS, credentialed, IP-literal, local, punycode (`xn--`), or shortened URLs.
- Missing or invalid calendar dates, an `expires_at` beyond the applicable standard (`45`-day) or content-derived evergreen (`120`-day) timeframe, or an already-elapsed explicit/extracted close date. A close date can shorten but not extend a timeframe.

---

## Layer 4: Listing Expiration & Pruning Policy

The weekly `Prune Outdated Community Listings` workflow (`.github/workflows/prune-community-listings.yml`) runs `scripts/prune-community-listings.js` to keep `data/community-listings.json` fresh without prematurely dropping long-running roles:

| Rule | Condition | Action |
|---|---|---|
| **1. No-escape timeframe rule** | Missing or invalid `added` date (or future date > 2 days) | **Pruned immediately** (`missing_timeframe` / `invalid_date`) |
| **2. Explicit close date** | `closes_at` (or deadline extracted from `raw_text`) `< today` | **Pruned immediately** (`close_date_elapsed`) |
| **3. Live ATS check** | Allowlisted Greenhouse/Lever/Ashby URL returns `404` or `410` | **Pruned immediately** (`ats_posting_closed`) |
| **4. Live ATS protection** | Allowlisted ATS URL returns `200 OK` and not past `closes_at` | **Kept** past the standard TTL only after a successful live check (up to the 180-day ceiling) |
| **5. Standard TTL** | Standard listing (LinkedIn, Indeed, Workday, etc.) past its content-policy timeframe (default **45 days**) | **Pruned** (`standard_ttl_elapsed`) |
| **6. Long-running / evergreen TTL** | Title or listing content matches rolling/evergreen/open-until-filled wording; a submitted boolean alone is ignored | Extended **120-day** TTL (`evergreen_ttl_elapsed`) |
| **7. Hard ceiling** | Any listing older than **180 days** from `added` | **Pruned unconditionally** (`hard_max_age_exceeded`) |

---

Listings past their expiration or closing date are hidden from users even if a live ATS check keeps the record for review. The relay retains queued submissions when GitHub rejects a write or PR creation.

## Layer 5: Desktop Client Validation (`fistbump-core`)

When the desktop app refreshes cached datasets in the background (`backend/internal/connectors/public_boards.go` and `backend/internal/cost/updater.go`):
- The updater uses the feed bundled with the app. Downloaded and cached data cannot change the next refresh URL. Only HTTPS URLs without credentials are accepted.
- Redirects are restricted to HTTPS and the exact same host (max 3 hops).
- Response bodies are size-capped (`4 MiB` for public boards, `256 KiB` for cost sources).
- Every loaded board token is re-validated against `^[a-z0-9][a-z0-9_-]{0,63}$` before entering memory or `<dataDir>/public-ats-boards.json`.

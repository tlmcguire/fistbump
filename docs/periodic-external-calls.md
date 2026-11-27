# Periodic External Calls & Scheduled Jobs

This document catalogs every scheduled or periodic outbound network call made by Fistbump across GitHub Actions, the Cloudflare Worker community relay, and the desktop application (`fistbump-core`).

## Summary Table

| Component | Job / Workflow | Schedule | External Hosts Contacted | Output / Effect |
|---|---|---|---|---|
| GitHub Actions | `update-cost-data.yml` | Monthly (1st, 06:00 UTC) | `bea.gov`, `huduser.gov`, `bls.gov`, `irs.gov`, `taxfoundation.org` | Refreshes `backend/internal/cost/sources.json` timestamps/metadata |
| GitHub Actions | `update-ats-boards.yml` | Monthly (1st, 07:00 UTC) | `datasets-server.huggingface.co`, `index.commoncrawl.org` | Opens/updates PR (`chore/refresh-public-ats-boards`) updating `public_boards.json` |
| GitHub Actions | `prune-community-listings.yml` | Weekly (Mon, 07:00 UTC) | Allowlisted ATS hosts only (Greenhouse, Lever, Ashby) | Opens/updates PR (`chore/prune-community-listings`) pruning `community-listings.json` |
| Cloudflare Worker | `relay/wrangler.toml` cron | Daily (06:17 UTC) | `api.github.com` | Opens/updates PRs (`community-boards`, `community-listings`) from KV queue |
| Cloudflare Worker | Edge KV cache refresh | Every 6 hours (on demand) | `api.github.com` | Refreshes cached `community-boards.json` and `community-listings.json` |
| Desktop App (Go) | `cost.StartUpdater` | Checks every 24h (refreshes every 30d) | Configured HTTPS `feed_url` (GitHub raw) | Updates `<dataDir>/cost-sources.json` |
| Desktop App (Go) | `connectors.StartBoardUpdater` | Checks every 24h (refreshes every 30d) | Configured HTTPS `feed_url` (GitHub raw) | Updates `<dataDir>/public-ats-boards.json` |

---

## 1. GitHub Actions Workflows

### 1.1 Refresh Cost & Pay Reference Sources
- **Workflow**: `.github/workflows/update-cost-data.yml`
- **Script**: `scripts/update-cost-data.js`
- **Schedule**: `0 6 1 * *` (1st of every month at 06:00 UTC) + manual `workflow_dispatch`.
- **External sites**: Verifies HTTP reachability of the 7 reference sources in `backend/internal/cost/sources.json` (BEA Regional Price Parities, HUD Fair Market Rents, BLS OES/CEX/EBS, IRS Tax Brackets, Tax Foundation State Tax Rates).
- **Safety controls**: Uses `redirect: "manual"` (max 3 hops) requiring `https:` and the exact same `baseDomain` on every redirect hop. Any cross-domain redirect or non-HTTPS hop fails verification.

### 1.2 Refresh Public ATS Board Datasets
- **Workflow**: `.github/workflows/update-ats-boards.yml`
- **Script**: `scripts/update-ats-boards.js`
- **Schedule**: `0 7 1 * *` (1st of every month at 07:00 UTC) + manual `workflow_dispatch`.
- **External sites**:
  - `datasets-server.huggingface.co`: queries `edwarddgao/open-apply-jobs` and `storesignalhq/ats-job-boards`.
  - `index.commoncrawl.org`: queries `collinfo.json` and the latest CDX index for `boards.greenhouse.io/*`, `job-boards.greenhouse.io/*`, `jobs.lever.co/*`, and `jobs.ashbyhq.com/*`.
- **How it works**: Extracts and normalizes Greenhouse, Lever, and Ashby board tokens, deduplicates against the curated catalog, writes `backend/internal/connectors/public_boards.json`, runs `scripts/verify-community-data.js`, and opens/updates a Pull Request (`chore/refresh-public-ats-boards`) for maintainer review.
- **Safety controls**: Strict `ALLOWED_HOSTS` allowlist, `redirect: "manual"` (max 3 hops), rejects `text/html` responses, `5 MiB` per-response cap, reserved-slug blocklist, and `5,000` max new boards per run.
- **Offline test**: `node scripts/update-ats-boards.js --offline`

### 1.3 Prune Outdated Community Listings
- **Workflow**: `.github/workflows/prune-community-listings.yml`
- **Script**: `scripts/prune-community-listings.js`
- **Schedule**: `0 7 * * 1` (every Monday at 07:00 UTC) + manual `workflow_dispatch`.
- **External sites**: Only probes `listing_url` values hosted on the official allowlisted ATS domains (`boards.greenhouse.io`, `job-boards.greenhouse.io`, `boards.eu.greenhouse.io`, `job-boards.eu.greenhouse.io`, `jobs.lever.co`, `jobs.eu.lever.co`, `jobs.ashbyhq.com`). Never visits LinkedIn, Indeed, Workday, or arbitrary third-party domains.
- **How it works**: Removes listings with missing/invalid timeframes, elapsed close dates (`closes_at` or extracted deadline text), `404`/`410` ATS responses, or elapsed TTLs (45 days standard, 120 days evergreen/rolling, 180-day hard ceiling). Runs `scripts/verify-community-data.js` and opens/updates a Pull Request (`chore/prune-community-listings`) if any rows changed.
- **Offline / dry-run test**: `node scripts/prune-community-listings.js --offline --dry-run`

---

## 2. Cloudflare Worker Relay (`relay/`)

### 2.1 Daily Community Pull Request Batch
- **Config**: `relay/wrangler.toml` (`crons = ["17 6 * * *"]`, daily at 06:17 UTC)
- **Handler**: `scheduled()` -> `publishBoards(env)` and `publishListings(env)` in `relay/src/index.js`.
- **External site**: `api.github.com` (authenticated via GitHub App installation token or repository token).
- **How it works**: Drains queued `board:*` and `listing:*` keys from Cloudflare KV, commits them to branches `community-boards` and `community-listings`, and opens (or updates) Pull Requests targeting `main`.

### 2.2 Edge Cache Refresh & On-Demand ATS Verification
- **Cache refresh**: `GET /v1/boards` and `GET /v1/listings` cache `data/community-boards.json` and `data/community-listings.json` in KV for **6 hours** (`expirationTtl: 6 * 3600`), fetching from `api.github.com` only on cache miss.
- **On-demand ATS check (non-periodic, user-triggered)**: When a user submits a board (`POST /v1/boards`) or a listing hosted on a supported ATS (`POST /v1/listings`), the relay performs a single `redirect: "error"` check against `boards-api.greenhouse.io`, `api.lever.co`, or `api.ashbyhq.com` to confirm the board exists and has at least one active open job posting.

---

## 3. Desktop Application (`fistbump-core`)

### 3.1 30-Day Cost & Pay Reference Updater
- **Code**: `backend/internal/cost/updater.go` (`cost.StartUpdater`, started in `backend/cmd/fistbump-core/main.go`).
- **Schedule**: Checks at startup and every **24 hours** whether `refresh_days` (default `30`) have elapsed since `updated_at`.
- **How it works**: If a `feed_url` is configured and 30 days have elapsed, fetches the JSON feed (10s timeout, `256 KiB` cap), validates it, activates it in memory, and caches it to `<dataDir>/cost-sources.json`. Offline launches gracefully keep the cached or bundled file.

### 3.2 30-Day Public ATS Board Updater
- **Code**: `backend/internal/connectors/public_boards.go` (`connectors.StartBoardUpdater`, started in `backend/cmd/fistbump-core/main.go`).
- **Schedule**: Checks at startup and every **24 hours** whether `refresh_days` (default `30`) have elapsed since `updated_at`.
- **How it works**: If a `feed_url` is configured and 30 days have elapsed, fetches the JSON feed over HTTPS only (15s timeout, `4 MiB` cap, same-host HTTPS redirects only, max 3 hops), validates `version == 1` and every board token, activates it in memory, and caches it to `<dataDir>/public-ats-boards.json`.

### 3.3 Local-Only Periodic Tasks (Zero External Calls)
- **Hourly cache purge** (`backend/cmd/fistbump-core/main.go`): Runs every 1 hour to delete expired rows (>24h) from the local SQLite search and logo caches. Makes no network requests.
- **Startup job retention & trash cleanup** (`backend/cmd/fistbump-core/main.go`): Runs once at startup on the local SQLite database. Makes no network requests.

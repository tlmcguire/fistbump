# Fistbump community relay

A small Cloudflare Worker between the app and GitHub. It handles three kinds of community submissions:

- **Report an issue.** People without a GitHub account describe a problem in the app. The relay files it as an issue
  labelled `from-the-app`, or, when it reads like an open report, adds it to that one as "someone else ran into this
  too" (with a 👍), so the same problem is not filed ten times. Before sending, the app shows up to three open reports
  that look similar and lets the person say "that's mine".
- **Shared job boards.** People who turn on sharing send the Greenhouse, Lever and Ashby boards they add to their
  watchlist. The relay asks each board whether it is real, queues the new ones, and once a day opens (or updates)
  a single pull request that adds them to `data/community-boards.json`. A maintainer reviews and merges it; people
  who use shared boards then get them on their next search.

- **Shared job listings.** The relay queues public listing text and links for a maintainer-reviewed PR. Published listings are visible through their expiration date.

The relay scrubs common email addresses, phone numbers, and home folder names from submitted text. Rate limits use a secret-salted IP hash scoped to the hour. The publisher keeps queued entries until GitHub confirms the write and any required PR creation.

## Set up

1. Create a GitHub App on your account with **Issues: read and write**, **Contents: read and write** and
   **Pull requests: read and write**, install it on the repository, and note its App ID and installation ID.
   Generate a private key and convert it to PKCS#8:
   `openssl pkcs8 -topk8 -nocrypt -in app.pem -out app-pkcs8.pem`
   (A fine-grained personal access token with the same permissions works too: set `GITHUB_TOKEN` instead.)
2. Create the issue labels `from-the-app`, `bug`, `idea` and `question` in the repository.
3. `npx wrangler kv namespace create RELAY` and put the id in `wrangler.toml`.
4. Set the secrets:
   ```
   npx wrangler secret put GITHUB_APP_ID
   npx wrangler secret put GITHUB_APP_PRIVATE_KEY
   npx wrangler secret put GITHUB_INSTALLATION_ID
   npx wrangler secret put RATE_SALT
   ```
5. `npx wrangler deploy`, then point the app at it: set `FISTBUMP_RELAY_URL` when building
   (or change `DEFAULT_RELAY` in `src-tauri/src/relay.rs`).

## Endpoints

| | |
|---|---|
| `GET /v1/reports/similar?q=` | up to 3 open app reports that look like the words |
| `POST /v1/reports` | `{kind: bug\|idea\|question, title, details, steps?, app: {version, os, view}, same_as?}` → `{status: created\|added, number, url}` |
| `POST /v1/boards` | `{boards: ["greenhouse:stripe", "lever:netflix"]}` → `{queued, known, not_found}` |
| `GET /v1/boards` | the shared list (`data/community-boards.json`) |

| `POST /v1/listings` | Submits a listing for review |
| `GET /v1/listings` | Returns published listings that are still current |

`npm test` runs the relay tests with mocked GitHub and ATS APIs.

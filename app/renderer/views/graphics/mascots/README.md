# Pico, the Fistbump mascot

Thirty poses of Pico, plus `face-expressions.png`, a sheet of its nine faces for reference.
Each pose is a 400px transparent PNG, cropped and scaled from the 1254px originals so every pose keeps the same scale.
The app draws them at about 128px.

The originals are raster images, so a pose's colours are close to the app's palette (navy `#0B2457`, off-white `#F2F5FB`, gold `#CEA615`) rather than exact.

## Where they are used

| Pose | Where |
|---|---|
| `02-welcome` | Home, beside the greeting |
| `03-fistbump` | Home, once the master resume has nothing missing |
| `05-searching-wide` | Find jobs, before a search |
| `09-search-stopped` | Find jobs, after a search is canceled |
| `08-no-results` | Find jobs, when a search finds nothing |
| `10-cant-connect` | Find jobs, when a search fails |
| `07-results-found` | Find jobs, in the listing pane beside a list of results |
| `06-open-link` | Open a link, before a link is opened |
| `29-something-went-wrong` | A listing that could not be loaded |
| `28-cleaning-up` | Settings, after clearing history, resetting the search form or clearing the cache |

## Not used yet

| Poses | Meant for |
|---|---|
| `01-neutral` | A default companion |
| `04-offer-win` | An offer or other big win |
| `25-thinking-loading` | Work that takes a while (a search is too quick for it, so it keeps a spinner) |
| `11-reading-resume` · `12-cant-read-resume` · `13-hidden-text` | Importing a resume and its warnings |
| `14-writing` · `15-saving-version` | The resume editor and saved versions |
| `16-skill-gap` · `17-tailoring-in-progress` · `18-reviewing-changes` · `19-changes-accepted` · `20-pdf-exported` | Tailoring, review and PDF export |
| `21-application-sent` · `22-waiting` · `23-interview-day` · `24-on-to-the-next` | The application tracker |
| `26-private-local` · `27-update-available` | Privacy notes, an update notice |
| `30-mega-error-faceplant` | A serious error |

## Home set

`home/` holds four more poses for the Home view, cropped and scaled the same way: `01-morning-hello` for a welcome or return visit, `02-cozy-break` for a quiet moment, `03-youve-got-this` for a nudge or a finished step, and `04-little-victory` for a small win.

`home/animations/` holds six looping SVGs: `idle`, `cozy`, `victory`, `searching`, `loading` and `thinking`. Each embeds its own artwork, scaled down to 480px, so it works as a plain `<img src>`. They show a still pose when reduced motion is on. None is used yet.

## Introduction

`intro/` holds the "Meet Pico" introduction: `meet-pico.png` (a 1200px still), `meet-pico.svg` and `meet-pico-animated.svg` (the same layout, the second with an entrance and idle motion), and `pico-wave-cutout.png`, the waving artwork on its own. None is used yet.

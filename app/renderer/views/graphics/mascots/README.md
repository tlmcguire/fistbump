# Pico, the Fistbump mascot

Thirty poses of Pico, plus `face-expressions.png`, a sheet of its nine faces for reference.
Each pose is a 400px transparent PNG, cropped and scaled from the 1254px originals so every pose keeps the same scale.
The app draws them at about 128px.

The originals are raster images, so a pose's colours are close to the app's palette (navy `#0B2457`, off-white `#F2F5FB`, gold `#CEA615`) rather than exact.

## Where they are used

| Pose | Where |
|---|---|
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
| `01-neutral` · `02-welcome` · `03-fistbump` | A default companion, a welcome, the signature fistbump (Home now uses the Home set) |
| `04-offer-win` | An offer or other big win |
| `25-thinking-loading` | Work that takes a while (a search is too quick for it, so it keeps a spinner) |
| `11-reading-resume` · `12-cant-read-resume` · `13-hidden-text` | Importing a resume and its warnings |
| `14-writing` · `15-saving-version` | The resume editor and saved versions |
| `16-skill-gap` · `17-tailoring-in-progress` · `18-reviewing-changes` · `19-changes-accepted` · `20-pdf-exported` | Tailoring, review and PDF export |
| `21-application-sent` · `22-waiting` · `23-interview-day` · `24-on-to-the-next` | The application tracker |
| `26-private-local` · `27-update-available` | Privacy notes, an update notice |
| `30-mega-error-faceplant` | A serious error |

## Home set

`home/` holds four more poses for the Home view, cropped and scaled the same way, and `home/animations/` six looping SVGs. Each SVG embeds its own artwork, scaled down to 480px, so it works as a plain `<img src>`.

| Asset | Where |
|---|---|
| `02-cozy-break` · `04-little-victory` | Not used (Home's greeting shows a monthly or fun-screen pose) |
| `03-youve-got-this` | Home, beside "Finish your resume" when one or two things are left |
| `01-morning-hello` | Not used (Home shows the month's pose instead) |
| `animations/*.svg` | Not used. The app shows still poses only for now. |

## Away, monthly and fun-screen sets

These three folders hold 400px poses scaled from the full 1254px canvas without cropping, since their props reach the edges. Home draws them a little larger so Pico stays the same size as in the cropped poses.

| Folder | Where |
|---|---|
| `monthly/` (`01-january` to `12-december`) | Home, beside the greeting, half the time: the pose for the current month |
| `fun-screen/` (pixel alien, playful error, tiny bird game, pixel cat, pixel heart, achievement) | Home, beside the greeting, the other half of the time: one picked at random each time Home is shown |
| `away/` (`01-one-week` to `05-six-months-plus`) | Not used. Home still says "Welcome back" after a week or more away, beside a monthly or fun-screen pose. |

## Introduction

`intro/` holds the "Meet Pico" introduction: `meet-pico.png` (a 1200px still), `meet-pico.svg` and `meet-pico-animated.svg` (the same layout, the second with an entrance and idle motion), and `pico-wave-cutout.png`, the waving artwork on its own. Home shows `meet-pico.png` above the welcome to someone with no master resume yet. The other three files are not used.

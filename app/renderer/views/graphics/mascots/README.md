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

`home/` holds four more poses for the Home view, cropped and scaled the same way, and `home/animations/` six looping SVGs. Each SVG embeds its own artwork, scaled down to 480px, so it works as a plain `<img src>`, and holds still when the system asks for reduced motion.

| Asset | Where |
|---|---|
| `animations/idle.svg` (still: `01-morning-hello`) | Home, beside the greeting |
| `animations/cozy.svg` (still: `02-cozy-break`) | Home, beside the greeting from 6pm |
| `animations/victory.svg` (still: `04-little-victory`) | Home, beside the greeting once the master resume has nothing missing |
| `03-youve-got-this` | Home, beside "Finish your resume" when one or two things are left |
| `animations/searching.svg` · `loading.svg` · `thinking.svg` | Not used yet |

Home shows the still pose in place of an animation when Reduce motion is on in Settings.

## Introduction

`intro/` holds the "Meet Pico" introduction: `meet-pico.png` (a 1200px still), `meet-pico.svg` and `meet-pico-animated.svg` (the same layout, the second with an entrance and idle motion), and `pico-wave-cutout.png`, the waving artwork on its own. Home shows `meet-pico-animated.svg` above the welcome to someone with no master resume yet, or `meet-pico.png` when motion is reduced. `meet-pico.svg` and `pico-wave-cutout.png` are not used.
